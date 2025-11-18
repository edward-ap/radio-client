// Package metadata selects and orchestrates different strategies that fetch
// track information (ICY, JSON APIs, etc.) for the currently playing stream.
package metadata

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

// Info describes metadata updates reported by strategies.
type Info struct {
	Title       string
	Description string
	Station     string
}

const (
	// MetadataTypeICY indicates ICY metadata (inline on the streaming socket).
	MetadataTypeICY = "ICY"
	// MetadataTypeJSON indicates metadata received via a JSON status endpoint.
	MetadataTypeJSON = "JSON"
)

// StrategyHint allows callers to provide or receive the chosen metadata strategy.
type StrategyHint struct {
	Type string
	URL  string
}

// Provider watches metadata for a stream and emits updates through onUpdate.
type Provider interface {
	Watch(ctx context.Context, streamURL string, hint StrategyHint, onUpdate func(Info), onStrategy func(StrategyHint))
}

// Logger is a small logging interface used by strategies for non-fatal errors.
type Logger interface {
	Printf(format string, args ...any)
}

// NewProvider builds the root dispatcher that tries strategies in order.
func NewProvider(client *http.Client, log Logger) Provider {
	if client == nil {
		client = http.DefaultClient
	}
	return &dispatcher{
		client:  client,
		logger:  log,
		direct:  newDirectStrategy(client, log),
		status:  newStatusJSONStrategy(client, log),
		sibling: newSiblingStrategy(client, log),
	}
}

var errNoICY = errors.New("icy metadata unavailable")

// dispatcher implements Provider by chaining direct ICY, sibling discovery, and
// JSON status strategies until one produces data.
type dispatcher struct {
	client  *http.Client
	logger  Logger
	direct  *directStrategy
	status  *statusJSONStrategy
	sibling *siblingStrategy
}

func (d *dispatcher) Watch(ctx context.Context, streamURL string, hint StrategyHint, onUpdate func(Info), onStrategy func(StrategyHint)) {
	if ctx == nil || streamURL == "" || onUpdate == nil {
		return
	}
	hType := strings.ToUpper(strings.TrimSpace(hint.Type))
	switch hType {
	case MetadataTypeJSON:
		target := hint.URL
		go d.runStatus(ctx, streamURL, target, onUpdate, onStrategy)
		return
	case MetadataTypeICY:
		target := hint.URL
		if target == "" {
			target = streamURL
		}
		go d.runDirect(ctx, target, onUpdate, onStrategy)
		return
	}
	go d.autoWatch(ctx, streamURL, onUpdate, onStrategy)
}

// autoWatch tries strategies in the following order:
//   1. Direct ICY metadata on the stream URL.
//   2. Sibling discovery (common patterns on aggregator hosts).
//   3. JSON status endpoints.
func (d *dispatcher) autoWatch(ctx context.Context, streamURL string, onUpdate func(Info), onStrategy func(StrategyHint)) {
	if err := d.direct.Watch(ctx, streamURL, func() {
		if onStrategy != nil {
			onStrategy(StrategyHint{Type: MetadataTypeICY, URL: streamURL})
		}
	}, onUpdate); err == nil || !errors.Is(err, errNoICY) {
		return
	}
	if target, info, err := d.sibling.Discover(ctx, streamURL); err == nil {
		if onStrategy != nil {
			onStrategy(StrategyHint{Type: MetadataTypeICY, URL: target})
		}
		if info.Title != "" || info.Station != "" {
			onUpdate(info)
		}
		d.direct.Watch(ctx, target, nil, onUpdate)
		return
	}
	_ = d.status.Watch(ctx, streamURL, "", func(api string) {
		if onStrategy != nil {
			onStrategy(StrategyHint{Type: MetadataTypeJSON, URL: api})
		}
	}, onUpdate)
}

func (d *dispatcher) runDirect(ctx context.Context, url string, onUpdate func(Info), onStrategy func(StrategyHint)) {
	// direct strategy invocation intentionally ignores returned error; autoWatch
	// handles fallback ordering while hint-driven runs assume the caller knows best.
	_ = d.direct.Watch(ctx, url, func() {
		if onStrategy != nil {
			onStrategy(StrategyHint{Type: MetadataTypeICY, URL: url})
		}
	}, onUpdate)
}

func (d *dispatcher) runStatus(ctx context.Context, streamURL, apiURL string, onUpdate func(Info), onStrategy func(StrategyHint)) {
	// status strategy always resolves the actual endpoint (apiURL may be empty).
	_ = d.status.Watch(ctx, streamURL, apiURL, func(actual string) {
		if onStrategy != nil {
			onStrategy(StrategyHint{Type: MetadataTypeJSON, URL: actual})
		}
	}, onUpdate)
}
