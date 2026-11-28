package v1

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
	"yamaclient/lib"
	"yamaclient/lib/api"
	"yamaclient/lib/channel"
)

type Active struct {
	api.Active
	Channel *channel.Channel `json:"-"`
	Content *channel.Content `json:"-"`
}

var (
	ErrErrFlagSet         = errors.New("error flag set")
	ErrStoppedFlag        = errors.New("stop flag set") //harmless
	ErrFailedClearRunning = errors.New("cannot clear while running")
	ErrFailedRace         = errors.New("race against another call")
)

type WorkerEvent uint8

const (
	None      WorkerEvent = iota
	Interrupt             // send awaits worker response
	Stop
	Play // send can restart dead worker
	Pause
	Next // send can restart dead worker
)

type WorkerPayload struct {
	Event   WorkerEvent
	Payload any
}

type queueWorker struct {
	mx      sync.Mutex
	stopped atomic.Bool
	errFlag atomic.Bool
	err     error

	handler *Handler
	state   *api.Player

	event     chan *WorkerPayload // actions to free from Awaiting
	subscribe chan struct{}
}

func (q *queueWorker) clear() error {
	if !q.stopped.Load() {
		return ErrFailedClearRunning
	}
	<-q.subscribe // redundancy if mid-close

	q.mx.Lock()
	defer q.mx.Unlock()
	if !q.errFlag.CompareAndSwap(true, false) {
		return fmt.Errorf("worker clear(): %w", ErrFailedRace)
	}

	//reset props
	q.err = nil
	// close(q.subscribe) //worker will close
	// close(q.event)
	q.state = &api.Player{
		Active: Active{},
	}
	return nil
}

// Play & Next can restart the worker
//
// Interrupt holds the lock & worker loop until returnfunc() releases TWICE
// pre-cycle modification currently unused by anything else,but here in case
// does not return errors (currently)
func (q *queueWorker) sendEvent(e WorkerEvent, data ...any) any {
	q.mx.Lock()
	defer q.mx.Unlock()

	var err error
	switch e {
	case Stop:
		if !q.stopped.CompareAndSwap(false, true) {
			return nil // q.event may be closed
		}
		q.event <- &WorkerPayload{Event: e, Payload: data}
		return nil
	case Interrupt:
		ch := make(chan api.Player)
		q.event <- &WorkerPayload{e, ch}
		//
		ctr := atomic.Uint32{}
		var val uint32 = 0
		// returns nil once worker is able to resume
		// Pre-Cycle | call()->Cycle | Post-Cycle | call() | continue
		return func() *api.Player {
			if val >= 2 || !ctr.CompareAndSwap(val, val+1) {
				return nil
			}
			val++
			c := <-ch
			return &c // worker is holding lock here
		}
	case Play, Next:
		if q.stopped.Load() {
			err = q.err
			_ = q.clear() // errs not harmful currently
			err = errors.Join(err, q.run())
		}
		fallthrough
	default:
		if q.stopped.Load() {
			return nil
		}
		q.event <- &WorkerPayload{Event: e, Payload: data}
	}

	if err != nil { //not fatal
		log.Printf("sendEvent %d: %v", e, err)
	}
	return nil
}

// worker will handle release inline, nobody else should call release() anyway
// func (q *queueWorker) release() {
// 	q.mx.Lock()
// 	defer q.mx.Unlock()

//		old := q.subscribe
//		q.subscribe := make(chan struct{})
//		close(old)
//	}

func (q *queueWorker) _step(p *WorkerPayload) (ok bool, err error) {
	if q.stopped.Load() {
		p.Event = Stop
	}

	var interruptCh chan api.Player
	var queuePushed bool

	//Pre-Cycle
	switch p.Event {
	case Stop:
		return false, nil
	case Interrupt:
		interruptCh = p.Payload.(chan api.Player)
	case Play:
	case Next:
	case Pause:
	case None: // signal that song queue added, try pop
		queuePushed = true
	}

	//Pre-Cycle Stage
	q.mx.Lock()
	defer q.mx.Unlock()
	if interruptCh != nil { //must run while owning lock
		interruptCh <- *q.state
	}

	//Cycle Stage
	// TODO ensure player performs action requested (r/w to/from handler and state.active)
	// non-Next commands can directly use Active (unless the song is over)
	// may need an event case for when song is completed. this might warrant
	// redesigining the midi Do() player again to reveal more of its states
	// by post-cycle, q.state should
	// fully reflect changes

	//Post-Cycle Stage
	if interruptCh != nil { //must run while owning lock
		interruptCh <- *q.state
	}
	fmt.Println(queuePushed, p.Event, p.Payload) // tmp prevent queuePushed blocking compilation
	return true, nil
}
func (q *queueWorker) _digestErr(ok bool, err error) {
	if err == nil {
		return
	}

	t := "WARN"
	if !ok {
		t = "ERR"
	}
	log.Printf("queueworker %s: %v", t, err)
}
func (q *queueWorker) _runner() {
	q.mx.Lock()
	q.state.Status = lib.Paused.String()
	q.mx.Unlock()
	//
	var stepErr error
	var none WorkerPayload
	for {
		var p = &none
		select {
		case p = <-q.event: // pointer to a copy
		case <-q.handler.AwaitPush():
		}
		//
		ok, err := q._step(p)
		q._digestErr(ok, err)
		if !ok {
			stepErr = err // (true, err) = big error
			break
		}

	}
	// cleanup
	q.mx.Lock()
	defer q.mx.Unlock()

	q.stopped.Store(true)
	if stepErr != nil {
		q.err = stepErr
		q.errFlag.Store(true)
	}

	close(q.event)
	close(q.subscribe)

}

// instantaneus
func (q *queueWorker) run() error {
	q.mx.Lock()
	defer q.mx.Unlock()
	if !q.stopped.CompareAndSwap(true, false) {
		return fmt.Errorf("worker run(): %w", ErrFailedRace)
	}
	if q.errFlag.Load() {
		return ErrErrFlagSet
	}
	//
	q.state.Status = lib.Stopped.String()
	q.event = make(chan *WorkerPayload)
	q.subscribe = make(chan struct{})
	go q._runner()
	return nil
}

// points to a readable copy of the playerstate
// bypasses lock when owned by worker, or stopped (release == nil)
func (q *queueWorker) UpdateState() (*api.Player, error) {
	var state *api.Player
	release := q.sendEvent(Interrupt).(func() *api.Player)
	//Pre cycle opns, try modify state
	// nop

	//Post cycle opns
	if release == nil { // stopped, should capture manually
		q.mx.Lock()
		s := *q.state //copy
		q.mx.Unlock()
		state = &s
	} else { // drain interrupt steps
		s := release()
		for s != nil {
			state = s
			s = release()
		}
	}

	return state, nil
}
func newWorker(h *Handler) *queueWorker {
	//
	closedSub := make(chan struct{})
	close(closedSub)
	closedEvent := make(chan *WorkerPayload)
	close(closedEvent)
	//
	q := queueWorker{
		handler: h,
		state: &api.Player{
			Active: Active{},
		},
		event:     closedEvent,
		subscribe: closedSub,
	}
	q.stopped.Store(true)
	return &q
}

var worker atomic.Pointer[queueWorker]

func InitPlayerWorker(h *Handler) {
	w := newWorker(h)
	if !worker.CompareAndSwap(nil, w) {
		panic("player worker already initialized")
	} else if err := w.run(); err != nil {
		panic(err)
	}
}

// TODO: ensure h.mx is actually used in these & queue.go

// GET /player
func HTTPGetPlayer(h *Handler) httpFunc {
	return func(w wrt, r *req) {
		worker := worker.Load() // *queueWorker
		var err error

		state, err := worker.UpdateState()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		err = json.NewEncoder(w).Encode(state)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
}

// PATCH /player
func HTTPSetPlayer(h *Handler) httpFunc {
	return func(w wrt, r *req) {
		worker := worker.Load() // *queueWorker

		//parse input json
		var options api.PlayerPatchable
		if err := json.NewDecoder(r.Body).Decode(&options); err != nil {
			var mbe *http.MaxBytesError
			if errors.As(err, &mbe) {
				w.WriteHeader(http.StatusRequestEntityTooLarge)
			} else {
				w.WriteHeader(http.StatusBadRequest)
			}
			return
		}

		// validate fields
		var elapsed *time.Duration // pointer so nil=no change
		var status lib.PlayerStatus
		if options.Elapsed != nil {
			elapsed = options.Elapsed
		}
		if options.Status != nil {
			status = lib.ToPlayerStatus(*options.Status)
			if status == lib.Unknown {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}
		var event WorkerEvent
		switch status {
		case lib.Unknown: // default when options.Status == nil
			event = None
		case lib.Paused:
			event = Pause
		case lib.Playing:
			event = Play
		case lib.Stopped:
			event = Stop
		default:
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		//fire
		worker.sendEvent(event, elapsed)
	}
}

// POST /player/next

// need to check for races, and migrate to a worker consumer
// 200 OK
// 204 NO CONTENT -> queue is empty
// state's current song should persist presence & play-state when 204ing
func HTTPNextPlayer(h *Handler) httpFunc {
	return func(w wrt, r *req) {
		worker := worker.Load() // *queueWorker
		worker.sendEvent(Next)
	}
}

/*
// simple demo
func(w wrt, r *req) {
	h.channels.ForEach(chanStop)
	for h.queue.Len() > 0 {
		song := h.Peek(1).Songs[0]
		h.RemoveIndices(0)
		// ! ^ race here

		ch, err := h.channels.Load(song)
		if err != nil {
			log.Printf("parsing %v: %v", song, err)
			continue
		}
		play, err := ch.GetPlayer() // no errcheck since channels r autogenerated
		if errors.Is(err, channel.ErrAlreadyPlaying) {
			_ = ch.Await()
			play, err = ch.GetPlayer()
		}
		if err != nil {
			log.Printf("starting %v: %v", song, err)
			continue
		}
		go play() //nolint
		return
	}
	w.WriteHeader(http.StatusNoContent) //empty/ied
}
*/
