// cmd/icypeek/main.go
package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: icypeek <stream-url>")
		return
	}
	url := os.Args[1]

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Icy-MetaData", "1")
	req.Header.Set("User-Agent", "icypeek/1.0")

	tr := &http.Transport{
		ForceAttemptHTTP2: false,
		DialContext: (&net.Dialer{
			Timeout:   7 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 7 * time.Second,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS10},
	}
	client := &http.Client{Transport: tr}

	// follow redirects manually (up to 2 hops) so icy-metaint header is preserved
	for i := 0; i <= 2; i++ {
		resp, err := client.Do(req)
		if err != nil {
			panic(err)
		}
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			loc := resp.Header.Get("Location")
			resp.Body.Close()
			if loc == "" {
				panic("redirect without Location")
			}
			req, _ = http.NewRequest("GET", loc, nil)
			req.Header.Set("Icy-MetaData", "1")
			req.Header.Set("User-Agent", "icypeek/1.0")
			continue
		}

		fmt.Println("=== Response Headers ===")
		for k, v := range resp.Header {
			fmt.Printf("%s: %s\n", k, strings.Join(v, ", "))
		}
		fmt.Println("========================")

		metaInt := 0
		fmt.Sscanf(resp.Header.Get("Icy-Metaint"), "%d", &metaInt)
		if metaInt <= 0 {
			fmt.Println("No icy-metaint => server does not send ICY metadata")
			return
		}

		defer resp.Body.Close()
		r := bufio.NewReader(resp.Body)
		audioBuf := make([]byte, 4096)
		metaBuf := make([]byte, 16*255)

		for block := 1; ; block++ {
			// skip audio bytes
			left := metaInt
			for left > 0 {
				chunk := left
				if chunk > len(audioBuf) {
					chunk = len(audioBuf)
				}
				n, err := io.ReadFull(r, audioBuf[:chunk])
				left -= n
				if err != nil {
					fmt.Println("audio read:", err)
					return
				}
			}
			// metadata block length
			lenByte, err := r.ReadByte()
			if err != nil {
				fmt.Println("len read:", err)
				return
			}
			if lenByte == 0 {
				continue
			}

			mlen := int(lenByte) * 16
			if _, err := io.ReadFull(r, metaBuf[:mlen]); err != nil {
				fmt.Println("meta read:", err)
				return
			}
			raw := string(metaBuf[:mlen])
			// trim at NUL
			if i := strings.IndexByte(raw, 0x00); i >= 0 {
				raw = raw[:i]
			}

			fmt.Printf("\n[Block %d] RAW: %q\n", block, raw)

			title := parseStreamTitle(raw)
			if title != "" {
				fmt.Printf("[Block %d] StreamTitle: %s\n", block, title)
			}
		}
	}
}

func parseStreamTitle(s string) string {
	meta := strings.TrimSpace(s)
	if meta == "" {
		return ""
	}
	lower := strings.ToLower(meta)
	pos := strings.Index(lower, "streamtitle='")
	delim := byte('\'')
	if pos < 0 {
		pos = strings.Index(lower, "streamtitle=\"")
		delim = '"'
	}
	if pos < 0 {
		return ""
	}
	start := pos + len("streamtitle=") + 1
	if start >= len(meta) {
		return ""
	}
	segment := meta[start:]
	end := len(segment)
	for i := 0; i < len(segment); i++ {
		if segment[i] != delim {
			continue
		}
		next := byte(0)
		if i+1 < len(segment) {
			next = segment[i+1]
		}
		if next == ';' || next == 0 || next == delim {
			end = i
			break
		}
	}
	return strings.TrimSpace(segment[:end])
}
