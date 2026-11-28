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

	channels *channelCache
}

func NewHandler(driver *drivers.Out, config settings.HttpConfig) *Handler {
	cacheSize := config.SongCache
	chs := make([]*channel.Channel, 0, cacheSize)
	for range cacheSize {
		chs = append(chs, channel.New(driver))
	}
	ttl := time.Second * time.Duration(config.Limits.CacheExpirySeconds)
	return &Handler{
		Mux:      chi.NewRouter(),
		queue:    list.New(),
		config:   config,
		channels: newChannelCache(chs, ttl),
	}
}
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
	return h.newAPICCR(), nil
}
func (h *Handler) RemoveIndices(idxs ...uint) *api.QueueChangeResponse {
	if len(idxs) == 0 {
		return h.newAPICCR()
	}
	imap := make(map[uint]int)
	var maxQi uint = 0
	for _, idx := range idxs {
		imap[idx] = 1
		maxQi = max(maxQi, idx) //break early
	}

	h.mx.Lock()
	defer h.mx.Unlock()

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
