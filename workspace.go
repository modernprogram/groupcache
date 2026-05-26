package groupcache

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"sync"
	"time"
)

// Workspace holds the "global" state for groupcache.
type Workspace struct {
	httpPoolMade bool
	portPicker   func() PeerPicker

	mu     sync.RWMutex
	groups map[string]*Group

	bufferPool sync.Pool

	defaultTransport *http.Transport
}

// DefaultWorkspace is the default workspace, useful for tests.
// If your application does not need to recreate groupcache resources,
// you can use this default workspace as well.
var DefaultWorkspace = NewWorkspace()

// NewWorkspace creates new workspace.
func NewWorkspace() *Workspace {
	return &Workspace{
		groups: make(map[string]*Group),
		bufferPool: sync.Pool{
			New: func() any { return new(bytes.Buffer) },
		},
		defaultTransport: newTransport(),
	}
}

func newTransport() *http.Transport {
	t := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext(ctx, network, addr)
		},
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return t
}
