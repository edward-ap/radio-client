package metadata

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// siblingStrategy scans for adjacent mount points (e.g. replacing "-320k" with
// "-128k") that may expose ICY metadata when the primary stream does not.
type siblingStrategy struct {
	client *http.Client
	logger Logger
}

var siblingCache sync.Map // family key -> sibling URL

// newSiblingStrategy constructs a new siblingStrategy with optional HTTP client
// and logger overrides.
func newSiblingStrategy(client *http.Client, log Logger) *siblingStrategy {
	return &siblingStrategy{client: client, logger: log}
}

// Discover attempts to find a lossy sibling stream that exposes ICY metadata.
// Returns the sibling URL and an optional initial Info sample.
func (s *siblingStrategy) Discover(ctx context.Context, streamURL string) (string, Info, error) {
	key := stationCacheKey(streamURL)
	if key != "" {
		if cached, ok := siblingCache.Load(key); ok {
			if target, _ := cached.(string); target != "" {
				return target, Info{}, nil
			}
		}
	}
	target, info, err := s.scanCandidates(ctx, streamURL)
	if err == nil && key != "" {
		siblingCache.Store(key, target)
	}
	return target, info, err
}

// scanCandidates builds and probes candidate mount points until one responds
// with ICY metadata.
func (s *siblingStrategy) scanCandidates(ctx context.Context, streamURL string) (string, Info, error) {
	u, err := url.Parse(streamURL)
	if err != nil || u == nil {
		return "", Info{}, errors.New("invalid url")
	}
	candidates := buildSiblingCandidates(u.Path)
	if len(candidates) == 0 {
		return "", Info{}, errors.New("no candidates")
	}
	for i, suffix := range candidates {
		select {
		case <-ctx.Done():
			return "", Info{}, ctx.Err()
		default:
		}
		cand := &url.URL{Scheme: u.Scheme, Host: u.Host, Path: suffix}
		candURL := cand.String()
		title, station, ok := s.probeCandidate(ctx, candURL)
		if ok {
			return cand.String(), Info{Title: title, Station: station}, nil
		}
		if i < len(candidates)-1 {
			time.Sleep(80 * time.Millisecond)
		}
	}
	return "", Info{}, errors.New("no sibling metadata")
}

// probeCandidate performs a lightweight HEAD-ish ICY request and only downloads
// the first metadata block to confirm viability.
func (s *siblingStrategy) probeCandidate(ctx context.Context, candidate string) (string, string, bool) {
	client := s.client
	if client == nil {
		client = &http.Client{
			Transport: &http.Transport{
				ForceAttemptHTTP2: false,
				Proxy:             http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   2 * time.Second,
					KeepAlive: 15 * time.Second,
				}).DialContext,
				TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS10},
			},
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Timeout: 0,
		}
	}
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, candidate, nil)
	if err != nil {
		return "", "", false
	}
	req.Header.Set("Icy-MetaData", "1")
	req.Header.Set("User-Agent", defaultUA)
	resp, err := client.Do(req)
	if err != nil {
		return "", "", false
	}
	defer resp.Body.Close()
	metaIntStr := resp.Header.Get("icy-metaint")
	if metaIntStr == "" {
		return "", "", false
	}
	metaInt, err := strconv.Atoi(metaIntStr)
	if err != nil || metaInt <= 0 {
		return "", "", false
	}
	title, err := firstMetaBlock(bufio.NewReader(resp.Body), metaInt)
	if err != nil || strings.TrimSpace(title) == "" {
		return "", "", false
	}
	station := strings.TrimSpace(resp.Header.Get("icy-name"))
	return fixMojibake(title), fixMojibake(station), true
}

// buildSiblingCandidates generates prioritized sibling mount paths by swapping
// bitrate or codec suffixes (e.g. "-320k" -> "-128k").
func buildSiblingCandidates(originalPath string) []string {
	if originalPath == "" {
		return nil
	}
	orig := originalPath
	if !strings.HasPrefix(orig, "/") {
		orig = "/" + orig
	}
	suffix := detectSiblingSuffix(orig)
	if suffix == "" {
		return nil
	}
	priorities := []string{
		"320", "320k",
		"256", "256k",
		"192", "192k",
		"128", "128k",
		"stream", "live", "aac", "aacp", "mp3",
	}
	result := make([]string, 0, len(priorities))
	for _, label := range priorities {
		result = append(result, strings.Replace(orig, suffix, label, 1))
	}
	return result
}

// detectSiblingSuffix extracts the trailing token (bitrate/codec) to swap.
func detectSiblingSuffix(path string) string {
	if path == "" {
		return ""
	}
	lastSep := strings.LastIndexAny(path, "-_.")
	if lastSep >= 0 && lastSep+1 < len(path) {
		return path[lastSep+1:]
	}
	return strings.Trim(path, "/")
}
