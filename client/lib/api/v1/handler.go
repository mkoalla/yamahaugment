package v1

import (
	"container/list"
	"sync"
	"sync/atomic"
	"yamaclient/lib/api"
	"yamaclient/lib/channel"
	"yamaclient/lib/settings"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	api.NodeHandler
	*chi.Mux
	init     atomic.Bool
	mx       sync.RWMutex
	queue    *list.List
	config   settings.HttpConfig
	channels []*channel.Channel
}

func NewRouter(config settings.HttpConfig) *Handler {
	return &Handler{
		Mux:      chi.NewRouter(),
		queue:    list.New(),
		config:   config,
		channels: make([]*channel.Channel, 0, config.SongCache),
	}
}
func (h *Handler) newAPICCR() *api.QueueChangeResponse {
	return &api.QueueChangeResponse{Length: uint(h.queue.Len()), Capacity: h.config.QueueSize}
}
func (h *Handler) Peek(max uint) *api.Queue {
	h.mx.RLock()
	defer h.mx.RUnlock()
	len := min(uint(h.queue.Len()), max, h.config.QueueSize)
	q := &api.Queue{
		QueueChangeResponse: *h.newAPICCR(),
		Songs:               make([]api.Song, 0, len),
	}
	if len == 0 {
		return q
	}

	cur := h.queue.Front()
	for cur != nil {
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
		if _, ok := imap[qi]; ok {
			h.queue.Remove(cur)
		}
		if qi == maxQi {
			break
		}
		cur = cur.Next()
		qi++
	}
	return h.newAPICCR()
}
func (h *Handler) RemoveSongs(songs ...api.Song) *api.QueueChangeResponse {
	smap := make(map[api.Song]int)
	for _, song := range songs {
		smap[song] = 1
	}

	h.mx.Lock()
	defer h.mx.Unlock()

	cur := h.queue.Front()
	for cur != nil {
		v := cur.Value.(*api.Song)
		if _, ok := smap[*v]; ok {
			h.queue.Remove(cur)
		}
		cur = cur.Next()
	}
	return h.newAPICCR()

}
func (h *Handler) Clear() *api.QueueChangeResponse {
	h.mx.Lock()
	defer h.mx.Unlock()
	h.queue.Init()
	return h.newAPICCR()
}
