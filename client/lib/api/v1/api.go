package v1

import (
	"net/http"

	"github.com/go-chi/chi/v5"
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
			r.Post("/remove", HTTPRemoveQueue(h))
			r.Post("/remove-at", HTTPRemoveAtQueue(h))
		})

		r.With(
			EnforceBodyLimitsKB(h.config.Limits.BodyPreferKB),
		).Route("/player", func(r chi.Router) {
			r.Get("/", HTTPGetPlayer(h))
			r.Patch("/", HTTPSetPlayer(h))
			r.Post("/next", HTTPSkipPlayer(h))
		})
	})
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
