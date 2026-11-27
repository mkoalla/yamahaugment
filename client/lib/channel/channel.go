package channel

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"yamaclient/lib"

	"gitlab.com/gomidi/midi/v2/drivers"
	"gitlab.com/gomidi/midi/v2/smf"
)

type SourceType uint8

const (
	None SourceType = iota
	ContentFile
	ContentDirect
)

func (s SourceType) String() string {
	switch s {
	case None:
		return "None"
	case ContentFile:
		return "ContentFile"
	case ContentDirect:
		return "ContentDirect"
	default:
		return "Unknown"
	}
}

var (
	ErrLoadPlaying          = errors.New("attempt to load new content while existing")
	ErrUnknownContentSource = errors.New("unknown content source")
	ErrNoContentLoaded      = errors.New("no content loaded")
	ErrNotPlaying           = errors.New("no content playing")
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
	//
	c.reader.StopPlaying()
	c.playing = false
}

// does not lock
func (c *Channel) setReader(data *Content) error {
	var rd *smf.TracksReader
	switch data.Source {
	case None:
		return ErrNoContentLoaded
	case ContentFile:
		rd = smf.ReadTracks((data.Data).(string), data.Tracks...)
	case ContentDirect:
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
	return func() error {
		if !c.epoch.CompareAndSwap(epoch, epoch+1) {
			return ErrHandleExpired
		}
		err := rd.Play(*out)
		lib.Drain(done, err)
		return err
	}
}

func (c *Channel) GetPlayer() (func() error, error) {
	if c.IsPlaying() {
		return nil, nil
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
