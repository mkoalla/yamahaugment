package v1

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math"
	"net/http"
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

type WorkerEvent uint8

const (
	None WorkerEvent = iota // no transition (seek-only)
	Stop
	Play
	Pause
	Next
)

type command struct {
	event   WorkerEvent
	elapsed *time.Duration // optional absolute seek, applied before event
	delta   *time.Duration // optional relative seek, resolved against the live position
	reply   chan error     // buffered(1)
}

type songDone struct {
	gen uint64
	err error
}

// a single goroutine (run) owns all player state below the snapshot,
// so none of it is locked. everyone else talks to it through cmds
type queueWorker struct {
	handler  *Handler
	ctx      context.Context
	cmds     chan command
	done     chan songDone              // play goroutines report here
	snapshot atomic.Pointer[api.Player] // republished after every step

	status   lib.PlayerStatus
	cur      *api.Song
	ch       *channel.Channel
	offset   time.Duration // position when not playing / at last (re)start
	started  time.Time     // wall time of last (re)start
	duration time.Duration
	gen      uint64 // bumped on every start/halt; stale songDone is ignored
}

func newWorker(h *Handler, ctx context.Context) *queueWorker {
	w := &queueWorker{
		handler: h,
		ctx:     ctx,
		cmds:    make(chan command),
		done:    make(chan songDone),
		status:  lib.Awaiting, // first push autoplays
	}
	w.publish()
	return w
}

func InitPlayerWorker(h *Handler) {
	if h.worker != nil {
		panic("player worker already initialized")
	}
	h.worker = newWorker(h, context.Background())
	go h.worker.run()
}

func (w *queueWorker) run() {
	for {
		var reply chan error
		var err error
		select {
		case c := <-w.cmds:
			reply = c.reply
			err = w.apply(c)
		case d := <-w.done:
			if d.gen == w.gen {
				w.songEnded(d.err)
			}
		case <-w.handler.AwaitPush():
			if w.status == lib.Awaiting {
				w.advance()
			}
		case <-w.ctx.Done():
			w.halt()
			return
		}
		w.publish() // before replying, so the caller reads its own result
		if reply != nil {
			reply <- err
		}
	}
}

func (w *queueWorker) apply(c command) error {
	if c.elapsed != nil || c.delta != nil {
		to := w.elapsed()
		if c.elapsed != nil {
			to = *c.elapsed
		}
		if c.delta != nil {
			to = addSat(to, *c.delta)
		}
		w.seek(to)
	}
	switch c.event {
	case Play:
		switch w.status {
		case lib.Paused, lib.Stopped:
			if w.cur != nil {
				w.start(w.offset)
				return nil
			}
			w.advance()
		case lib.Awaiting:
			w.advance()
		}
	case Pause:
		if w.status == lib.Playing {
			w.offset = w.elapsed()
			w.halt()
			w.status = lib.Paused
		}
	case Stop: // keeps the song, rewinds
		w.halt()
		w.offset = 0
		w.status = lib.Stopped
	case Next:
		return w.next()
	}
	return nil
}

func (w *queueWorker) elapsed() time.Duration {
	if w.status != lib.Playing {
		return w.offset
	}
	return min(w.offset+time.Since(w.started), w.duration)
}

// delta clamp
func addSat(a, b time.Duration) time.Duration {
	sum := a + b
	if b > 0 && sum < a {
		return math.MaxInt64
	}
	if b < 0 && sum > a {
		return math.MinInt64
	}
	return sum
}

// clamps to [0, duration]
func (w *queueWorker) seek(to time.Duration) {
	if w.cur == nil {
		return
	}
	to = min(max(to, 0), w.duration)
	if w.status == lib.Playing {
		w.halt()
		w.start(to)
		return
	}
	w.offset = to
}

// empty queue leaves the current song and state untouched
func (w *queueWorker) next() error {
	song, ok := w.handler.PopFront()
	if !ok {
		return api.ErrQueueEmpty
	}
	w.halt()
	if err := w.load(song); err != nil {
		log.Printf("player: loading %v: %v", song, err)
		w.advance()
		return nil
	}
	w.start(0)
	return nil
}

// plays the next loadable song, or goes Awaiting
func (w *queueWorker) advance() {
	w.halt()
	for {
		song, ok := w.handler.PopFront()
		if !ok {
			w.cur, w.ch = nil, nil
			w.offset, w.duration = 0, 0
			w.status = lib.Awaiting
			return
		}
		if err := w.load(song); err != nil {
			log.Printf("player: loading %v: %v", song, err)
			continue
		}
		w.start(0)
		return
	}
}

func (w *queueWorker) load(song api.Song) error {
	ch, err := w.handler.channels.Load(song)
	if err != nil {
		return err
	}
	w.cur, w.ch = &song, ch
	w.offset, w.duration = 0, ch.Duration()
	return nil
}

func (w *queueWorker) start(offset time.Duration) {
	play, err := w.ch.GetPlayerAt(offset)
	if err != nil {
		log.Printf("player: starting %v: %v", w.cur, err)
		w.advance()
		return
	}
	w.gen++
	gen := w.gen
	w.offset, w.started = offset, time.Now()
	w.status = lib.Playing
	go func() {
		err := play()
		select {
		case w.done <- songDone{gen, err}:
		case <-w.ctx.Done():
		}
	}()
}

// silences the current channel; Stop() blocks until its handle returned
func (w *queueWorker) halt() {
	w.gen++
	if w.ch == nil {
		return
	}
	w.ch.Stop()
	_ = w.ch.HaltNotesNow()
}

func (w *queueWorker) songEnded(err error) {
	if err != nil {
		log.Printf("player: playing %v: %v", w.cur, err)
	}
	w.advance()
}

func (w *queueWorker) publish() {
	p := &api.Player{Status: w.status.String()}
	if w.cur != nil {
		song := *w.cur
		p.Current = &song
	}
	p.Metadata.Elapsed = w.offset
	p.Metadata.End = w.duration - w.offset
	if w.status == lib.Playing {
		p.Metadata.Start = w.started.Add(-w.offset)
	}
	w.snapshot.Store(p)
}

// never blocks on the worker; times are filled in at read time while playing
func (w *queueWorker) State() api.Player {
	s := *w.snapshot.Load()
	if s.Status == lib.Playing.String() {
		total := s.Metadata.Elapsed + s.Metadata.End
		el := min(time.Since(s.Metadata.Start), total)
		s.Metadata.Elapsed, s.Metadata.End = el, total-el
	}
	return s
}

// c.reply is set here
func (w *queueWorker) send(ctx context.Context, c command) error {
	c.reply = make(chan error, 1)
	select {
	case w.cmds <- c:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-c.reply:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func writeState(w wrt, q *queueWorker) {
	if err := json.NewEncoder(w).Encode(q.State()); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// GET /player
func HTTPGetPlayer(h *Handler) httpFunc {
	return func(w wrt, r *req) {
		writeState(w, h.worker)
	}
}

// PATCH /player
func HTTPSetPlayer(h *Handler) httpFunc {
	return func(w wrt, r *req) {
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
		var status lib.PlayerStatus
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
		if err := h.worker.send(r.Context(), command{event: event, elapsed: options.Elapsed, delta: options.Delta}); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writeState(w, h.worker)
	}
}

// POST /player/next
// 200 OK
// 204 NO CONTENT -> queue is empty
// state's current song should persist presence & play-state when 204ing
func HTTPNextPlayer(h *Handler) httpFunc {
	return func(w wrt, r *req) {
		err := h.worker.send(r.Context(), command{event: Next})
		if errors.Is(err, api.ErrQueueEmpty) {
			w.WriteHeader(http.StatusNoContent)
			return
		} else if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writeState(w, h.worker)
	}
}
