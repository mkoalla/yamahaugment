package api

import (
	"errors"
	"fmt"
	"time"
	"yamaclient/lib"
)

var (
	ErrBadRequest = errors.New("")
	ErrQueueFull  = fmt.Errorf("queue will not fit request%w", ErrBadRequest)
	ErrBadIndex   = fmt.Errorf("queue index out of range%w", ErrBadRequest)
	ErrQueueEmpty = errors.New("queue is empty")
)

type SourceType = lib.SourceType
type PlayerStatus = lib.PlayerStatus

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

type Active interface{} // placeholder for types not crossing boundary (see player.go)

type PlayerPatchable struct {
	Elapsed *time.Duration `json:"elapsed"`
	Delta   *time.Duration `json:"delta"`
	Status  *string        `json:"status"` //player status; only (omit)/Paused/Playing/Stopped
}
type PlayerSongMetadata struct {
	Start   time.Time     `json:"start"`
	Elapsed time.Duration `json:"elapsed"`
	End     time.Duration `json:"remaining"`
}
type Player struct {
	Current  *Song              `json:"current-song"`
	Status   string             `json:"status"`
	Metadata PlayerSongMetadata `json:"metadata"`
	Active   Active             `json:"-"`
}
