package channel

import (
	"cmp"
	"errors"
	"math"
	"slices"
	"time"

	"gitlab.com/gomidi/midi/v2/smf"
)

var ErrContentTooLong = errors.New("content longer than ~71min")

// playable events in time order, packed so a song costs ~12B/event instead of
// smf's ~40B (32B struct + an allocation per message). built once at load;
// the smf.TracksReader is dropped afterwards
type events struct {
	at   []uint32 // µs from start
	end  []uint32 // message i is data[end[i-1]:end[i]]
	data []byte
}

func newEvents(rd *smf.TracksReader) (*events, error) {
	type ev struct {
		at  int64
		msg []byte
	}
	var evs []ev // transient; points into rd's messages
	size := 0
	rd.Do(func(te smf.TrackEvent) {
		if te.Message.IsPlayable() {
			evs = append(evs, ev{te.AbsMicroSeconds, te.Message})
			size += len(te.Message)
		}
	})
	if err := rd.Error(); err != nil {
		return nil, err
	}
	// Do walks track by track; stable keeps same-time order within a track
	slices.SortStableFunc(evs, func(a, b ev) int { return cmp.Compare(a.at, b.at) })
	if len(evs) > 0 && evs[len(evs)-1].at > math.MaxUint32 {
		return nil, ErrContentTooLong
	}

	e := &events{
		at:   make([]uint32, len(evs)),
		end:  make([]uint32, len(evs)),
		data: make([]byte, 0, size),
	}
	for i, ev := range evs {
		e.data = append(e.data, ev.msg...)
		e.at[i], e.end[i] = uint32(ev.at), uint32(len(e.data))
	}
	return e, nil
}

func (e *events) len() int { return len(e.at) }

func (e *events) time(i int) time.Duration {
	return time.Duration(e.at[i]) * time.Microsecond
}

func (e *events) msg(i int) []byte {
	var from uint32
	if i > 0 {
		from = e.end[i-1]
	}
	return e.data[from:e.end[i]]
}

// time of the last playable event
func (e *events) duration() time.Duration {
	if e.len() == 0 {
		return 0
	}
	return e.time(e.len() - 1)
}
