package api

import (
	"errors"
	"fmt"
)

var (
	ErrBadRequest = errors.New("")
	ErrQueueFull  = fmt.Errorf("queue will not fit request%w", ErrBadRequest)
)

const (
	UintMax = ^uint(0)
)

const (
	None SourceType = iota
	ContentFile
	ContentDirect
)

type SourceType uint8

func (s SourceType) String() string {
	switch s {
	case None:
		return "None"
	case ContentFile:
		return "ContentFile"
	case ContentDirect:
		return "ContentDirect"
	default:
		return "Unknown"
	}
}
func ToSourceType(s string) SourceType {
	switch s {
	case "ContentFile":
		return ContentFile
	case "ContentDirect":
		return ContentDirect
	default:
		return None
	}
}

type NodeHandlerQueue interface {
	Peek(max uint) *Queue
	Push(songs ...Song) (*QueueChangeResponse, error)
	RemoveIndices(idxs ...uint) *QueueChangeResponse
	RemoveSongs(songs ...Song) *QueueChangeResponse
	Clear() *QueueChangeResponse
}
type NodeHandler interface {
	NodeHandlerQueue
}

type Song struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type QueueChangeResponse struct {
	Length   uint `json:"length"`
	Capacity uint `json:"capacity"`
}

// Length/Capacity always based off queue size (not peeked)
type Queue struct {
	QueueChangeResponse
	Songs []Song `json:"songs"`
}
