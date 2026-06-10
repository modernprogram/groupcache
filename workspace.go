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
var DefaultWorkspace = NewWorkspace(DefaultResponseHeaderTimeout)

// NewWorkspace creates new workspace.
// responseHeaderTimeout is the time to wait for the response header
// in a cross peer request.
// responseHeaderTimeout defaults to 15s if set to 0 or negative.
// Set responseHeaderTimeout to about the longest time a key can take
// to be generated (on cache miss).
// Be careful not to set responseHeaderTimeout too low, otherwise it will
// cause cross peer requests to fail when the key owner peer main cache
// does not have the key (cache miss) and takes long to generate the key.
func NewWorkspace(responseHeaderTimeout time.Duration) *Workspace {
	return &Workspace{
		groups: make(map[string]*Group),
		bufferPool: sync.Pool{
			New: func() any { return new(bytes.Buffer) },
		},
		defaultTransport: newTransport(responseHeaderTimeout),
	}
}

const DefaultResponseHeaderTimeout = 15 * time.Second

// newTransport creates new http.Transport with default settings.
func newTransport(responseHeaderTimeout time.Duration) *http.Transport {
	if responseHeaderTimeout <= 0 {
		responseHeaderTimeout = DefaultResponseHeaderTimeout
	}

	// Intentionally diverges from http.DefaultTransport defaults:
	// tuned for groupcache peer traffic and explicit HTTP/1 behavior.
	t := &http.Transport{
		Proxy:             http.ProxyFromEnvironment,
		ForceAttemptHTTP2: false,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{
				Timeout: 7 * time.Second,
				KeepAliveConfig: net.KeepAliveConfig{
					Enable:   true,
					Idle:     15 * time.Second,
					Interval: 15 * time.Second,
					Count:    3,
				},
			}).DialContext(ctx, network, addr)
		},
		MaxIdleConns:          1000,
		MaxIdleConnsPerHost:   30,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: responseHeaderTimeout,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return t
}
