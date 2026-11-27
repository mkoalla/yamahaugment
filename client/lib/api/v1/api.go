package v1

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"yamaclient/lib/api"
	"yamaclient/lib/channel"

	"github.com/go-chi/chi/v5"
	"gitlab.com/gomidi/midi/v2/drivers"
)

type req = http.Request
type wrt = http.ResponseWriter
type httpFunc = func(wrt, *req)

func (h *Handler) InitRoutes() {
	if !h.init.CompareAndSwap(false, true) {
		return
	}
	h.Route("/api/v1", func(r chi.Router) {

		r.With(
			EnforceBodyLimitsKB(h.config.Limits.BodyPreferKB),
		).Route("/queue", func(r chi.Router) {
			r.Get("/", HTTPGetQueue(h))
			r.Post("/add", HTTPAddQueue(h))
		})
	})
}

func (h *Handler) BuildChannels(driver *drivers.Out, count uint) {
	for range count {
		h.channels = append(h.channels, channel.New(driver))
	}
}

func EnforceBodyLimitsKB(limit uint) func(http.Handler) http.Handler {
	lim := int64(limit) * 1000
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, lim)
			next.ServeHTTP(w, r)
		})
	}
}

func HTTPGetQueue(h *Handler) httpFunc {
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
			return
		}
	}
}
func HTTPAddQueue(h *Handler) httpFunc {
	return func(w wrt, r *req) {
		var songs []api.Song
		var err error
		if err = json.NewDecoder(r.Body).Decode(&songs); err != nil {
			var e *http.MaxBytesError
			if errors.As(err, &e) {
				w.WriteHeader(http.StatusRequestEntityTooLarge)
			} else {
				w.WriteHeader(http.StatusBadRequest) //not 500 bc sender's payload
			}
			return
		}

		resp, err := h.Push(songs...)
		if err != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		err = json.NewEncoder(w).Encode(resp)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

	}
}
