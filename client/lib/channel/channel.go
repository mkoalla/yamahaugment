package channel

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	compiled_drv "yamaclient/drivers"
	"yamaclient/lib"

	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
	"gitlab.com/gomidi/midi/v2/smf"
)

type SourceType = lib.SourceType

var (
	ErrLoadPlaying          = errors.New("attempt to load new content while existing")
	ErrUnknownContentSource = errors.New("unknown content source")
	ErrNoContentLoaded      = errors.New("no content loaded")
	ErrNotPlaying           = errors.New("no content playing")
	ErrAlreadyPlaying       = errors.New("content already playing")
	ErrNoDriver             = errors.New("no driver provided")
	ErrHandleExpired        = errors.New("this Play() handle is expired")
)

type Content struct {
	Name   string
	Source SourceType
	Data   any
	Tracks []int
}

type Channel struct {
	driver  *drivers.Out
	content Content
	events  *events
	mx      sync.RWMutex
	playing bool
	loaded  bool
	epoch   atomic.Uint64
	done    chan error
	stop    chan struct{} // closed by Stop(); one per GetPlayer() handle
	exited  chan struct{} // closed when the running handle returns
}

func New(d *drivers.Out) *Channel {
	return &Channel{
		driver: d,
		done:   make(chan error),
	}
}

func (c *Channel) Await() error {
	c.mx.RLock()
	if !c.playing {
		c.mx.RUnlock()
		return ErrNotPlaying
	}
	src := c.done
	c.mx.RUnlock()
	return <-src
}

func (c *Channel) IsPlaying() bool {
	c.mx.RLock()
	defer c.mx.RUnlock()
	return c.playing
}

// blocks until a running handle has returned, so nothing is sent after Stop()
func (c *Channel) Stop() {
	c.mx.Lock()
	if !c.loaded {
		c.mx.Unlock()
		return
	}
	// don't rely on reader.StopPlaying(): smf sets isPlaying late
	if c.stop != nil {
		close(c.stop)
		c.stop = nil
	}
	c.playing = false
	exited := c.exited
	c.mx.Unlock()
	if exited != nil {
		<-exited
	}
}

// time of the last playable event; 0 if not loaded
func (c *Channel) Duration() time.Duration {
	c.mx.RLock()
	defer c.mx.RUnlock()
	if !c.loaded {
		return 0
	}
	return c.events.duration()
}

// stop note echo
func (c *Channel) HaltNotesNow() error {
	c.mx.RLock()
	defer c.mx.RUnlock()
	if c.driver == nil {
		return ErrNoDriver
	}
	out := *c.driver
	if err := out.Open(); err != nil {
		return err
	}
	var errs []error
	for ch := range uint8(16) { // all driver channels
		errs = append(errs,
			out.Send(midi.ControlChange(ch, midi.AllSoundOff, 0)),
			out.Send(midi.ControlChange(ch, midi.AllNotesOff, 0)),
		)
	}
	return errors.Join(errs...)
}

// does not lock
func (c *Channel) setReader(data *Content) error {
	var rd *smf.TracksReader
	switch data.Source {
	case lib.None:
		return ErrNoContentLoaded
	case lib.ContentFile:
		rd = smf.ReadTracks(fmt.Sprintf("data/%s", data.Data), data.Tracks...)
	case lib.ContentDirect:
		rd = smf.ReadTracksFrom((data.Data).(io.Reader), data.Tracks...)
	default:
		return fmt.Errorf("%w: %d", ErrUnknownContentSource, data.Source)
	}
	if rd.Error() != nil {
		return rd.Error()
	}
	evs, err := newEvents(rd)
	if err != nil {
		return err
	}
	c.events = evs
	c.loaded = true
	return nil
}

// lazy=true waits until Play() to load
func (c *Channel) Load(data Content, lazy bool) error {
	if c.IsPlaying() {
		return ErrLoadPlaying
	}
	c.mx.Lock()
	defer c.mx.Unlock()
	if !lazy {
		if err := c.setReader(&data); err != nil {
			return err
		}
	} else {
		c.loaded = false
	}
	c.content = data //set only after it could error
	return nil
}

// caller should have locked
func (c *Channel) getPlayerHandle(offset time.Duration) func() error {
	evs := c.events
	out := c.driver
	epoch := c.epoch.Add(1) // newer handles expire this one
	done := c.done
	stop := make(chan struct{})
	c.stop = stop
	var used atomic.Bool
	return func() error {
		if !used.CompareAndSwap(false, true) {
			return ErrHandleExpired
		}
		c.mx.Lock()
		if c.epoch.Load() != epoch {
			c.mx.Unlock()
			return ErrHandleExpired
		}
		select {
		case <-stop: // stopped before it started
			c.mx.Unlock()
			return nil
		default:
		}
		exited := make(chan struct{})
		c.exited = exited
		c.mx.Unlock()

		err := play(evs, *out, stop, offset)

		c.mx.Lock()
		if c.stop == stop { // ran to the end without Stop()
			c.stop = nil
			c.playing = false
		}
		c.mx.Unlock()
		close(exited)
		lib.Drain(done, err)
		return err
	}
}

// note on/off and poly aftertouch; skipped when seeking past them
func isNoteEvent(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	switch data[0] & 0xF0 {
	case 0x80, 0x90, 0xA0:
		return true
	}
	return false
}

// mirrors smf.TracksReader.Play, but properly cancellable. starts at offset:
// earlier non-note events (program, CC, pitch bend...) are sent at once so the
// device state matches the seek point; earlier notes are dropped
func play(evs *events, out drivers.Out, stop <-chan struct{}, offset time.Duration) error {
	if err := out.Open(); err != nil {
		return err
	}

	// const -> compiler can remove at build time
	if compiled_drv.LockThread {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
	}

	timer := time.NewTimer(0)
	defer timer.Stop()
	start := time.Now().Add(-offset)
	for i := range evs.len() {
		at, data := evs.time(i), evs.msg(i)
		if at < offset {
			if isNoteEvent(data) {
				continue
			}
			if err := out.Send(data); err != nil {
				return err
			}
			continue
		}
		timer.Reset(time.Until(start.Add(at)))
		select {
		case <-stop:
			return nil
		case <-timer.C:
		}
		if err := out.Send(data); err != nil {
			return err
		}
	}
	return nil
}

func (c *Channel) GetPlayer() (func() error, error) {
	return c.GetPlayerAt(0)
}

// handle starts playback offset into the content
func (c *Channel) GetPlayerAt(offset time.Duration) (func() error, error) {
	if c.IsPlaying() {
		return nil, ErrAlreadyPlaying
	}
	c.mx.Lock()
	unlock := sync.OnceFunc(c.mx.Unlock)
	defer unlock()

	if c.driver == nil {
		return nil, ErrNoDriver
	}

	if !c.loaded {
		if err := c.setReader(&c.content); err != nil {
			return nil, err
		}
	}

	c.playing = true
	handle := c.getPlayerHandle(offset)
	unlock()
	return handle, nil
}
