package v1

import (
	"container/list"
	"sync"
	"sync/atomic"
	"time"
	"yamaclient/lib"
	"yamaclient/lib/api"
	"yamaclient/lib/channel"
	"yamaclient/lib/settings"

	"github.com/go-chi/chi/v5"
	"gitlab.com/gomidi/midi/v2/drivers"
)

type Handler struct {
	api.NodeHandler
	*chi.Mux
	init   atomic.Bool
	mx     sync.RWMutex
	queue  *list.List
	config settings.HttpConfig

	qsubscribe chan struct{} // 1-slot level signal, never replaced; doesnt return element (subs compete to pop)
	channels   *channelCache
}

func NewHandler(driver *drivers.Out, config settings.HttpConfig) *Handler {
	cacheSize := config.SongCache
	chs := make([]*channel.Channel, 0, cacheSize)
	for range cacheSize {
		chs = append(chs, channel.New(driver))
	}
	ttl := time.Second * time.Duration(config.Limits.CacheExpirySeconds)
	return &Handler{
		Mux:        chi.NewRouter(),
		queue:      list.New(),
		config:     config,
		channels:   newChannelCache(chs, ttl),
		qsubscribe: make(chan struct{}, 1),
	}
}

// fires once per batch of pushes since the last receive; a push made while
// nobody is waiting stays pending instead of being missed
func (h *Handler) AwaitPush() <-chan struct{} {
	return h.qsubscribe
}

// caller must hold h.mx
func (h *Handler) newAPICCR() *api.QueueChangeResponse {
	return &api.QueueChangeResponse{Length: uint(h.queue.Len()), Capacity: h.config.QueueSize}
}
func (h *Handler) Peek(max uint) *api.Queue {
	h.mx.RLock()
	defer h.mx.RUnlock()
	slen := min(uint(h.queue.Len()), max, h.config.QueueSize)
	q := &api.Queue{
		QueueChangeResponse: *h.newAPICCR(),
		Songs:               make([]api.Song, 0, slen),
	}
	if slen == 0 {
		return q
	}

	cur := h.queue.Front()
	for cur != nil && uint(len(q.Songs)) < max {
		q.Songs = append(q.Songs, cur.Value.(api.Song))
		cur = cur.Next()
	}
	return q
}
func (h *Handler) Push(songs ...api.Song) (*api.QueueChangeResponse, error) {
	h.mx.Lock()
	defer h.mx.Unlock()
	if uint(h.queue.Len()+len(songs)) > h.config.QueueSize {
		return nil, api.ErrQueueFull
	}
	for _, s := range songs {
		if lib.ToSourceType(s.Type) == lib.None { //bad input
			continue
		}
		h.queue.PushBack(s)
	}

	//release .AwaitPush()
	select {
	case h.qsubscribe <- struct{}{}:
	default: // already pending
	}

	return h.newAPICCR(), nil
}
func (h *Handler) RemoveIndices(idxs ...uint) *api.QueueChangeResponse {
	h.mx.Lock()
	defer h.mx.Unlock()
	return h.removeIndices(idxs)
}

// like RemoveIndices, but negative idxs count from the back. resolved under the
// same lock as the removal so a concurrent push/pop can't shift them
func (h *Handler) RemoveAt(idxs ...int) (*api.QueueChangeResponse, error) {
	h.mx.Lock()
	defer h.mx.Unlock()
	queueLen := h.queue.Len()
	resolved := make([]uint, 0, min(queueLen, len(idxs)))
	for _, idx := range idxs {
		if len(resolved) >= queueLen {
			break
		}
		if idx < 0 {
			idx += queueLen
		}
		if idx < 0 || idx >= queueLen {
			return nil, api.ErrBadIndex
		}
		resolved = append(resolved, uint(idx))
	}
	return h.removeIndices(resolved), nil
}

// caller must hold h.mx
func (h *Handler) removeIndices(idxs []uint) *api.QueueChangeResponse {
	if len(idxs) == 0 {
		return h.newAPICCR()
	}
	imap := make(map[uint]int)
	var maxQi uint = 0
	for _, idx := range idxs {
		imap[idx] = 1
		maxQi = max(maxQi, idx) //break early
	}

	var qi uint = 0
	cur := h.queue.Front()
	for cur != nil {
		next := cur.Next()
		if _, ok := imap[qi]; ok {
			h.queue.Remove(cur)
		}
		if qi == maxQi {
			break
		}
		cur = next
		qi++
	}
	return h.newAPICCR()
}
func (h *Handler) RemoveSongs(songs ...api.Song) *api.QueueChangeResponse {
	smap := make(map[api.Song]int)
	maxDel := len(songs)
	for _, song := range songs {
		smap[song]++
	}

	h.mx.Lock()
	defer h.mx.Unlock()

	cur := h.queue.Front()
	del := 0
	for cur != nil && del < maxDel {
		next := cur.Next()
		v := cur.Value.(api.Song)
		if count := smap[v]; count > 0 {
			h.queue.Remove(cur)
			smap[v]--
			del++
		}
		cur = next
	}
	return h.newAPICCR()

}
func (h *Handler) Clear() *api.QueueChangeResponse {
	h.mx.Lock()
	defer h.mx.Unlock()
	h.queue.Init()
	return h.newAPICCR()
}
