package metadata

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type testLogger struct{}

func (testLogger) Printf(string, ...any) {}

func TestStatusJSONSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "status-json.xsl") {
			io.WriteString(w, `{"icestats":{"source":{"title":"Foo - Bar","server_name":"Station"}}}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	strat := newStatusJSONStrategy(srv.Client(), testLogger{})
	got := make(chan Info, 1)
	go func() {
		_ = strat.Watch(ctx, srv.URL+"/stream", "", func(string) {}, func(info Info) {
			got <- info
			cancel()
		})
	}()

	select {
	case info := <-got:
		if info.Title != "Foo - Bar" || info.Station != "Station" {
			t.Fatalf("unexpected info: %+v", info)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for metadata")
	}
}

func TestStatusJSONNotFound(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	strat := newStatusJSONStrategy(srv.Client(), testLogger{})
	if err := strat.Watch(ctx, srv.URL+"/stream", "", func(string) {}, func(Info) {}); err == nil {
		t.Fatal("expected error for missing status json")
	}
}

func TestStatusJSONInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("{broken"))
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	strat := newStatusJSONStrategy(srv.Client(), testLogger{})
	if err := strat.Watch(ctx, srv.URL+"/stream", "", func(string) {}, func(Info) {}); err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestStrategyFallbackToSibling(t *testing.T) {
	streamTitle := "Test Title"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rock-flac":
			// no icy headers -> direct should fail
			w.Write([]byte("flac stream without icy"))
		case "/rock-320":
			w.Header().Set("icy-metaint", "1")
			w.Header().Set("icy-name", "Rock Paradise")
			w.Write(buildICYBody(streamTitle))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	prov := NewProvider(srv.Client(), testLogger{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	strategyResolved := make(chan StrategyHint, 1)
	updates := make(chan Info, 1)
	prov.Watch(ctx, srv.URL+"/rock-flac", StrategyHint{}, func(info Info) {
		updates <- info
	}, func(h StrategyHint) {
		strategyResolved <- h
		cancel()
	})

	select {
	case hint := <-strategyResolved:
		if hint.Type != MetadataTypeICY || !strings.Contains(hint.URL, "rock-320") {
			t.Fatalf("unexpected hint: %+v", hint)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("timeout waiting for strategy hint")
	}

	select {
	case info := <-updates:
		if strings.TrimSpace(info.Title) != streamTitle {
			t.Fatalf("unexpected title %q", info.Title)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("timeout waiting for metadata update")
	}
}

func TestStrategyFallbackJSON(t *testing.T) {
	var statusHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rock-flac":
			w.Write([]byte("no icy"))
		case "/status-json.xsl":
			statusHits.Add(1)
			io.WriteString(w, `{"icestats":{"source":{"title":"A - B","server_name":"Station"}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	prov := NewProvider(srv.Client(), testLogger{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	strategyResolved := make(chan StrategyHint, 1)
	go prov.Watch(ctx, srv.URL+"/rock-flac", StrategyHint{}, func(Info) {}, func(h StrategyHint) {
		strategyResolved <- h
		cancel()
	})

	select {
	case hint := <-strategyResolved:
		if hint.Type != MetadataTypeJSON || hint.URL == "" {
			t.Fatalf("unexpected strategy hint: %+v", hint)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("timeout waiting for JSON strategy")
	}
	if statusHits.Load() == 0 {
		t.Fatal("expected status-json to be polled")
	}
}

func buildICYBody(title string) []byte {
	buf := bytes.NewBuffer(nil)
	buf.WriteByte(0) // first audio byte skipped
	meta := fmt.Sprintf("StreamTitle='%s';", title)
	for len(meta)%16 != 0 {
		meta += "\x00"
	}
	buf.WriteByte(byte(len(meta) / 16))
	buf.WriteString(meta)
	return buf.Bytes()
}
