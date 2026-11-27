package v1

import (
	"container/list"
	"sync"
	"sync/atomic"
	"yamaclient/lib/channel"
	"yamaclient/lib/settings"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	APINodeHandler
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
func (h *Handler) newAPICCR() *APIQueueChangeResponse {
	return &APIQueueChangeResponse{uint(h.queue.Len()), h.config.QueueSize}
}
func (h *Handler) Peek(max uint) *APIQueue {
	h.mx.RLock()
	defer h.mx.RUnlock()
	len := min(uint(h.queue.Len()), max, h.config.QueueSize)
	q := &APIQueue{
		*h.newAPICCR(),
		make([]APISong, 0, len),
	}
	if len == 0 {
		return q
	}

	cur := h.queue.Front()
	for cur != nil {
		q.Songs = append(q.Songs, cur.Value.(APISong))
		cur = cur.Next()
	}
	return q
}
func (h *Handler) Push(song APISong) (*APIQueueChangeResponse, error) {
	h.mx.Lock()
	defer h.mx.Unlock()
	if uint(h.queue.Len()) >= h.config.QueueSize {
		return nil, ErrAPIQueueFull
	}
	h.queue.PushBack(&song)
	return h.newAPICCR(), nil
}
func (h *Handler) RemoveIndices(idxs ...uint) *APIQueueChangeResponse {
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
func (h *Handler) RemoveSongs(songs ...APISong) *APIQueueChangeResponse {
	smap := make(map[APISong]int)
	for _, song := range songs {
		smap[song] = 1
	}

	h.mx.Lock()
	defer h.mx.Unlock()

	cur := h.queue.Front()
	for cur != nil {
		v := cur.Value.(*APISong)
		if _, ok := smap[*v]; ok {
			h.queue.Remove(cur)
		}
		cur = cur.Next()
	}
	return h.newAPICCR()

}
func (h *Handler) Clear() *APIQueueChangeResponse {
	h.mx.Lock()
	defer h.mx.Unlock()
	h.queue.Init()
	return h.newAPICCR()
}
