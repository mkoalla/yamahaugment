package v1

import (
	"errors"
	"fmt"
)

var (
	ErrBadRequest   = errors.New("")
	ErrAPIQueueFull = fmt.Errorf("queue is full%w", ErrBadRequest)
)

const (
	UintMax = ^uint(0)
)

type APINodeHandlerQueue interface {
	Peek(max uint) *APIQueue
	Push(song APISong) (*APIQueueChangeResponse, error)
	RemoveIndices(idxs ...uint) *APIQueueChangeResponse
	RemoveSongs(songs ...APISong) *APIQueueChangeResponse
	Clear() *APIQueueChangeResponse
}
type APINodeHandler interface {
	APINodeHandlerQueue
}

type APISong struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type APIQueueChangeResponse struct {
	Length   uint `json:"length"`
	Capacity uint `json:"capacity"`
}

// Length/Capacity always based off queue size (not peeked)
type APIQueue struct {
	APIQueueChangeResponse
	Songs []APISong `json:"songs"`
}
