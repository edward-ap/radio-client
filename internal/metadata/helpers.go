package metadata

import (
	"bufio"
	"context"
	"html"
	"io"
	"net/url"
	"path"
	"strings"
	"unicode"
)

// defaultUA is intentionally generic: ICY sources often reject exotic user
// agents, so we mimic a simple desktop client.
var defaultUA = "MiniRadio/1.0 (+https://local)"

// ioReadFull mirrors io.ReadFull but aborts promptly when ctx is cancelled.
func ioReadFull(ctx context.Context, r io.Reader, buf []byte) (int, error) {
	type reader interface {
		Read([]byte) (int, error)
	}
	ch := make(chan struct {
		n   int
		err error
	}, 1)
	go func() {
		n, err := io.ReadFull(r, buf)
		ch <- struct {
			n   int
			err error
		}{n, err}
	}()
	select {
	case res := <-ch:
		return res.n, res.err
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// fixMojibake keeps ASCII text untouched (some stations send pre-encoded UTF-8
// via ISO-8859-1); for non-ASCII we defer to the original string.
func fixMojibake(s string) string {
	s = strings.TrimSpace(s)
	s = html.UnescapeString(s)
	if !isASCII(s) {
		return s
	}
	return s
}

// isASCII returns true if the string only contains ASCII runes.
func isASCII(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII {
			return false
		}
	}
	return true
}

// extractStreamTitle parses ICY metadata blocks, extracting `StreamTitle`.
func extractStreamTitle(meta string) string {
	if meta == "" {
		return ""
	}

	idx := strings.Index(meta, "StreamTitle=")
	if idx < 0 {
		return ""
	}

	meta = meta[idx+12:]
	meta = strings.TrimSpace(meta)
	if meta == "" {
		return ""
	}

	quote := rune(0)
	if meta[0] == '\'' || meta[0] == '"' {
		quote = rune(meta[0])
		meta = meta[1:]
	}
	if quote != 0 {
		end := -1
		for i := 0; i < len(meta); i++ {
			if meta[i] != byte(quote) {
				continue
			}
			j := i + 1
			for j < len(meta) && (meta[j] == ' ' || meta[j] == '\t') {
				j++
			}
			if j >= len(meta) {
				end = i
				break
			}
			if meta[j] == ';' {
				rest := meta[j+1:]
				if strings.Contains(rest, "=") {
					end = i
					break
				}
			}
		}
		if end < 0 {
			end = strings.LastIndexByte(meta, byte(quote))
		}
		if end >= 0 {
			meta = meta[:end]
		}
	} else {
		if end := strings.Index(meta, ";"); end >= 0 {
			meta = meta[:end]
		}
	}
	meta = strings.TrimSpace(meta)
	return html.UnescapeString(meta)
}

// firstMetaBlock skips the audio payload and decodes the next metadata frame.
func firstMetaBlock(r *bufio.Reader, metaInt int) (string, error) {
	if r == nil || metaInt <= 0 {
		return "", io.EOF
	}
	buf := make([]byte, 4096)
	left := metaInt
	for left > 0 {
		chunk := left
		if chunk > len(buf) {
			chunk = len(buf)
		}
		n, err := io.ReadFull(r, buf[:chunk])
		left -= n
		if err != nil {
			return "", err
		}
	}

	lb, err := r.ReadByte()
	if err != nil {
		return "", err
	}
	if lb == 0 {
		return "", io.EOF
	}

	metaLen := int(lb) * 16
	meta := make([]byte, metaLen)
	if _, err := io.ReadFull(r, meta); err != nil {
		return "", err
	}

	text := extractStreamTitle(string(meta))
	if text == "" {
		return "", io.EOF
	}

	return text, nil
}

// baseNameFromURL returns the lowercase mount name without numeric suffixes.
func baseNameFromURL(u *url.URL) string {
	if u == nil {
		return ""
	}

	p := path.Base(u.Path)
	p = strings.TrimSuffix(p, path.Ext(p))
	for i := 0; i < len(p); i++ {
		if p[i] == '-' || p[i] == '_' {
			return strings.ToLower(p[:i])
		}
	}
	return strings.ToLower(p)
}

// stationCacheKey creates a host+basename key for sibling discovery cache.
func stationCacheKey(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return ""
	}

	return strings.ToLower(u.Host) + "|" + baseNameFromURL(u)
}
