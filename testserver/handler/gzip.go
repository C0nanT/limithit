package handler

import (
	"compress/gzip"
	"io"
	"net/http"
)

// NewGzipHandler returns a handler that accepts gzip-encoded POST bodies,
// decompresses them, and responds with the decompressed byte count.
// maxDecompress is the decompression cap in bytes; 0 means unlimited.
func NewGzipHandler(maxDecompress int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		var body []byte
		var err error
		isGzip := r.Header.Get("Content-Encoding") == "gzip"

		if isGzip {
			gr, gerr := gzip.NewReader(r.Body)
			if gerr != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid gzip"})
				return
			}
			defer gr.Close()
			if maxDecompress > 0 {
				limited := io.LimitReader(gr, maxDecompress+1)
				body, err = io.ReadAll(limited)
				if err == nil && int64(len(body)) > maxDecompress {
					writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "decompressed body exceeds limit"})
					return
				}
			} else {
				body, err = io.ReadAll(gr)
			}
		} else {
			body, err = io.ReadAll(io.LimitReader(r.Body, 1<<20))
		}

		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"bytes":      len(body),
			"compressed": isGzip,
		})
	}
}
