package metadata

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"html"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// directStrategy connects directly to the stream URL and reads ICY metadata
// blocks interleaved in the response body.
type directStrategy struct {
	client *http.Client
	logger Logger
}

func newDirectStrategy(client *http.Client, log Logger) *directStrategy {
	return &directStrategy{client: client, logger: log}
}

// Watch probes the provided stream for ICY metadata. It runs synchronously and
// stops when the context is cancelled or an error occurs.
func (s *directStrategy) Watch(ctx context.Context, streamURL string, onReady func(), onUpdate func(Info)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Icy-MetaData", "1")
	req.Header.Set("User-Agent", defaultUA)

	cli := s.client
	if cli == nil {
		cli = &http.Client{
			Transport: &http.Transport{
				ForceAttemptHTTP2: false,
				Proxy:             http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   7 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout: 7 * time.Second,
				TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS10},
			},
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Timeout: 0,
		}
	}

	resp, err := cli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		loc := resp.Header.Get("Location")
		if loc == "" {
			return fmt.Errorf("redirect without location")
		}
		return s.Watch(ctx, loc, onReady, onUpdate)
	}

	metaIntStr := resp.Header.Get("icy-metaint")
	if metaIntStr == "" {
		return errNoICY
	}

	metaInt, err := strconv.Atoi(metaIntStr)
	if err != nil || metaInt <= 0 {
		return errNoICY
	}

	station := strings.TrimSpace(resp.Header.Get("icy-name"))
	if station != "" {
		onUpdate(Info{Station: html.UnescapeString(station)})
	}
	if onReady != nil {
		onReady()
		onReady = nil
	}

	reader := bufio.NewReader(resp.Body)
	buf := make([]byte, metaInt)
	for {
		if _, err := ioReadFull(ctx, reader, buf); err != nil {
			return err
		}

		length, err := reader.ReadByte()
		if err != nil {
			return err
		}

		metaLen := int(length) * 16
		if metaLen == 0 {
			continue
		}

		metaBuf := make([]byte, metaLen)
		if _, err := ioReadFull(ctx, reader, metaBuf); err != nil {
			return err
		}

		text := extractStreamTitle(string(metaBuf))
		if text != "" {
			onUpdate(Info{Title: fixMojibake(text), Station: html.UnescapeString(station)})
		}
	}
}
