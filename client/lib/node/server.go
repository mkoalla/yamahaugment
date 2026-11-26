package node

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"yamaclient/lib"
	"yamaclient/lib/settings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
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
	if n.available.CompareAndSwap(true, false) {
		return ErrNodeUnstarted //not started
	}
	return n.server.Shutdown(ctx)
}
func New(config settings.NodeConfig) EdgeNode {
	if config.Port == 0 {
		return nil
	}
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("welcome")) //nolint
	})

	return &Node{
		server: &http.Server{Addr: fmt.Sprintf(":%d", config.Port), Handler: r},
		res:    make(chan error),
	}
}
