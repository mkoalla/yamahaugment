package channel

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

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
	reader  *smf.TracksReader
	mx      sync.RWMutex
	playing bool
	loaded  bool
	epoch   atomic.Uint64
	done    chan error
	stop    chan struct{} // closed by Stop(); one per GetPlayer() handle
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

func (c *Channel) Stop() {
	c.mx.Lock()
	defer c.mx.Unlock()
	if !c.loaded {
		return
	}
	// don't rely on reader.StopPlaying(): smf sets isPlaying late
	if c.stop != nil {
		close(c.stop)
		c.stop = nil
	}
	c.playing = false
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
	c.reader = rd
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

func (c *Channel) ForEach(fn func(smf.TrackEvent)) error {
	c.mx.RLock()
	defer c.mx.RUnlock()
	//
	if c.reader == nil {
		return ErrNoContentLoaded
	}
	return c.reader.Do(fn).Error()
}
func (c *Channel) Error() error {
	c.mx.Lock()
	defer c.mx.Unlock()
	if !c.loaded {
		return nil
	}
	return c.reader.Error()
}

// caller should have locked
func (c *Channel) getPlayerHandle() func() error {
	rd := c.reader
	out := c.driver
	epoch := c.epoch.Load()
	done := c.done
	stop := make(chan struct{})
	c.stop = stop
	return func() error {
		if !c.epoch.CompareAndSwap(epoch, epoch+1) {
			return ErrHandleExpired
		}
		err := play(rd, *out, stop)
		lib.Drain(done, err)
		return err
	}
}

type playEvent struct {
	at   time.Duration
	data []byte
}

// mirrors smf.TracksReader.Play, but properly cancellable
func play(rd *smf.TracksReader, out drivers.Out, stop <-chan struct{}) error {
	if err := rd.Error(); err != nil {
		return err
	}
	if err := out.Open(); err != nil {
		return err
	}
	var evs []playEvent
	rd.Do(func(te smf.TrackEvent) {
		if te.Message.IsPlayable() {
			evs = append(evs, playEvent{time.Duration(te.AbsMicroSeconds) * time.Microsecond, te.Message})
		}
	})
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].at < evs[j].at })

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	timer := time.NewTimer(0)
	defer timer.Stop()
	start := time.Now()
	for _, ev := range evs {
		timer.Reset(time.Until(start.Add(ev.at)))
		select {
		case <-stop:
			return nil
		case <-timer.C:
		}
		if err := out.Send(ev.data); err != nil {
			return err
		}
	}
	return nil
}

func (c *Channel) GetPlayer() (func() error, error) {
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
	handle := c.getPlayerHandle()
	unlock()
	return handle, nil
}
