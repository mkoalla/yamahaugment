package v1

import (
	"encoding/json"
	"net/http"
	"strconv"
	"yamaclient/lib/channel"

	"github.com/go-chi/chi/v5"
	"gitlab.com/gomidi/midi/v2/drivers"
)

type req = http.Request
type wrt = http.ResponseWriter

func (h *Handler) InitRoutes() {
	if !h.init.CompareAndSwap(false, true) {
		return
	}
	h.Route("/api/v1", func(r chi.Router) {

		r.Get("/queue", HTTPGetQueue(h))
	})
}

func (h *Handler) BuildChannels(driver *drivers.Out, count uint) {
	for range count {
		h.channels = append(h.channels, channel.New(driver))
	}
}

func HTTPGetQueue(h *Handler) func(wrt, *req) {
	return func(w wrt, r *req) {
		queryMaxVal := r.URL.Query().Get("max")
		var maxVal uint
		if queryMaxVal == "" {
			maxVal = h.config.QueueSize
		} else {
			val, _ := strconv.Atoi(queryMaxVal)
			maxVal = uint(max(0, val))
		}

		err := json.NewEncoder(w).Encode(h.Peek(maxVal))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
}
