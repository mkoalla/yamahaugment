package node

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"yamaclient/lib"
	"yamaclient/lib/settings"

	api "yamaclient/lib/api/v1"

	"github.com/go-chi/chi/v5/middleware"
	"gitlab.com/gomidi/midi/v2/drivers"
)

var (
	ErrNodeUnstarted = errors.New("node hasn't been started") //harmless
)

type EdgeNode interface {
	Start()
	Await() error
	Available() bool
	Stop(context.Context) error
}

type Node struct {
	EdgeNode
	server    *http.Server
	res       chan error
	available atomic.Bool
}

func (n *Node) Await() error {
	return <-n.res
}
func (n *Node) Available() bool {
	return n.available.Load()
}
func (n *Node) Start() {
	if !n.available.CompareAndSwap(false, true) {
		return
	}
	lib.Drain(n.res, n.server.ListenAndServe())
}
func (n *Node) Stop(ctx context.Context) error {
	if !n.available.CompareAndSwap(true, false) {
		return ErrNodeUnstarted //not started
	}
	return n.server.Shutdown(ctx)
}
func New(driver *drivers.Out, config settings.HttpConfig) EdgeNode {
	if config.Port == 0 {
		return nil
	}

	r := api.NewRouter(config)
	r.Use(middleware.Logger)
	r.BuildChannels(driver, config.SongCache)
	r.InitRoutes()

	return &Node{
		server: &http.Server{
			Addr:    fmt.Sprintf(":%d", config.Port),
			Handler: r,
		},
		res: make(chan error),
	}
}
