package main

import (
	"net/http"
	"strconv"
	"strings"
)

// End long open-ended video reads regularly so HTTP/1.1 connections can serve
// images and UI requests. Downloads and non-range requests retain normal semantics.
// RFC 9110 section 15.3.7 explicitly permits a subset of a requested range.
func fairBrowserVideoRange(r *http.Request, size int64, local bool) *http.Request {
	if r.Method != http.MethodGet || size <= 0 || r.URL.Query().Get("download") == "1" {
		return r
	}
	header := r.Header.Get("Range")
	if !strings.HasPrefix(header, "bytes=") || strings.Contains(header, ",") {
		return r
	}
	parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(header, "bytes=")), "-")
	if len(parts) != 2 || parts[0] == "" {
		return r
	}
	start, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil || start < 0 || start >= size {
		return r
	}
	end := size - 1
	if parts[1] != "" {
		end, err = strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil || end < start {
			return r
		}
		if end >= size {
			end = size - 1
		}
	}
	chunk := int64(256 << 10)
	if local {
		chunk = 1 << 20
	}
	if end-start+1 <= chunk {
		return r
	}
	copy := r.Clone(r.Context())
	copy.Header = r.Header.Clone()
	copy.Header.Set("Range", "bytes="+strconv.FormatInt(start, 10)+"-"+strconv.FormatInt(start+chunk-1, 10))
	return copy
}
