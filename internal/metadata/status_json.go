package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// statusJSONStrategy polls Icecast-style /status-json.xsl endpoints on the same
// host as the stream, parsing track info from the response.
type statusJSONStrategy struct {
	client *http.Client
	logger Logger
}

func newStatusJSONStrategy(client *http.Client, log Logger) *statusJSONStrategy {
	return &statusJSONStrategy{client: client, logger: log}
}

// Watch sends the initial update after the first successful poll and then
// continues to refresh every 10 seconds until the context is cancelled.
func (s *statusJSONStrategy) Watch(ctx context.Context, streamURL string, apiURL string, onReady func(string), onUpdate func(Info)) error {
	if strings.TrimSpace(apiURL) == "" {
		var err error
		apiURL, err = buildStatusURL(streamURL)
		if err != nil {
			return err
		}
	}
	title, station, ok := s.pollOnce(ctx, apiURL)
	if !ok {
		return errors.New("status-json unavailable")
	}
	if onReady != nil {
		onReady(apiURL)
	}
	onUpdate(Info{Title: title, Station: station})
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			title, station, ok := s.pollOnce(ctx, apiURL)
			if ok {
				onUpdate(Info{Title: title, Station: station})
			}
		}
	}
}

// pollOnce performs a single request, returning (title, station, true) if a
// valid source entry exists.
func (s *statusJSONStrategy) pollOnce(ctx context.Context, apiURL string) (string, string, bool) {
	client := s.client
	if client == nil {
		client = http.DefaultClient
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", "", false
	}
	req.Header.Set("User-Agent", defaultUA)
	resp, err := client.Do(req)
	if err != nil {
		return "", "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", false
	}
	var st iceStats
	dec := json.NewDecoder(io.LimitReader(resp.Body, 1<<20))
	if err := dec.Decode(&st); err != nil {
		return "", "", false
	}
	sources := extractSources(st.IceStats.Source)
	for _, src := range sources {
		if strings.TrimSpace(src.Title) != "" {
			station := src.Server
			if strings.TrimSpace(station) == "" {
				station = src.IcyName
			}
			return src.Title, station, true
		}
	}
	return "", "", false
}

// buildStatusURL converts a stream URL ("/live/rock") into its sibling JSON
// endpoint ("/live/status-json.xsl").
func buildStatusURL(streamURL string) (string, error) {
	u, err := url.Parse(streamURL)
	if err != nil {
		return "", err
	}
	u.Path = path.Join("/", path.Dir(u.Path), "status-json.xsl")
	return u.String(), nil
}

type iceStats struct {
	IceStats struct {
		Source any `json:"source"`
	} `json:"icestats"`
}

type iceSource struct {
	Title   string `json:"title"`
	Server  string `json:"server_name"`
	IcyName string `json:"icy-name"`
}

// extractSources normalizes Icecast's `source` field which may be a single
// object or an array depending on mount counts.
func extractSources(v any) []iceSource {
	out := []iceSource{}
	switch val := v.(type) {
	case map[string]any:
		b, _ := json.Marshal(val)
		var s iceSource
		if json.Unmarshal(b, &s) == nil {
			out = append(out, s)
		}
	case []any:
		for _, it := range val {
			b, _ := json.Marshal(it)
			var s iceSource
			if json.Unmarshal(b, &s) == nil {
				out = append(out, s)
			}
		}
	}
	return out
}
