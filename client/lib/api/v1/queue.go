package v1

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"yamaclient/lib/api"
)

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

func HTTPRemoveQueue(h *Handler) httpFunc {
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

		err = json.NewEncoder(w).Encode(h.RemoveSongs(songs...))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

	}
}

func HTTPRemoveAtQueue(h *Handler) httpFunc {
	return func(w wrt, r *req) {
		var intSongs []int
		var err error
		if err = json.NewDecoder(r.Body).Decode(&intSongs); err != nil {
			var e *http.MaxBytesError
			if errors.As(err, &e) {
				w.WriteHeader(http.StatusRequestEntityTooLarge)
			} else {
				w.WriteHeader(http.StatusBadRequest) //not 500 bc sender's payload
			}
			return
		}
		queueLen := h.queue.Len()
		songCap := min(queueLen, len(intSongs))
		songs := make([]uint, 0, songCap)
		for _, i := range intSongs {
			if len(songs) >= songCap {
				break
			}

			idx := i
			if idx < 0 {
				idx += queueLen
			}
			if idx < 0 || idx > queueLen {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			songs = append(songs, uint(idx))
		}
		err = json.NewEncoder(w).Encode(h.RemoveIndices(songs...))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

	}
}
