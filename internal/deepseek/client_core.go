package deepseek

import (
	"context"
	"net/http"
	"sync"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	trans "ds2api/internal/deepseek/transport"
	"ds2api/internal/devcapture"
	"ds2api/internal/util"
)

// intFrom is a package-internal alias for the shared util version.
var intFrom = util.IntFrom

type Client struct {
	Store      *config.Store
	Auth       *auth.Resolver
	capture    *devcapture.Store
	regular    trans.Doer
	stream     trans.Doer
	fallback   *http.Client
	fallbackS  *http.Client
	maxRetries int

	proxyClientsMu sync.RWMutex
	proxyClients   map[string]requestClients
}

func NewClient(store *config.Store, resolver *auth.Resolver) *Client {
	return &Client{
		Store:      store,
		Auth:       resolver,
		capture:    devcapture.Global(),
		regular:    trans.New(60 * time.Second),
		stream:     trans.New(0),
		fallback:   &http.Client{Timeout: 60 * time.Second},
		fallbackS:  &http.Client{Timeout: 0},
		maxRetries: 3,
		proxyClients: map[string]requestClients{},
	}
}

// PreloadPow 保留兼容接口，纯 Go 实现无需预加载。
func (c *Client) PreloadPow(_ context.Context) error {
	return nil
}
