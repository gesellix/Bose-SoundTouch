package handlers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"
)

// SnapshotMiddleware creates an immutable snapshot of the request body and metadata.
func (s *Server) SnapshotMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Check if we already have a snapshot (shouldn't happen with correct middleware order)
		if _, ok := r.Context().Value(SnapshotKey).(*RequestSnapshot); ok {
			next.ServeHTTP(w, r)
			return
		}

		// 2. Capture body with size limit (e.g. 2MB)
		const maxBodySize = 2 * 1024 * 1024

		var body []byte

		if r.Body != nil {
			buf, ok := bufferPool.Get().(*bytes.Buffer)
			if !ok {
				buf = new(bytes.Buffer)
			}

			buf.Reset()
			defer bufferPool.Put(buf)

			// Read up to maxBodySize + 1 to detect truncation
			_, err := io.CopyN(buf, r.Body, maxBodySize+1)
			_ = r.Body.Close()

			if err != nil && !errors.Is(err, io.EOF) {
				// If reading fails, proceed with empty body but log it?
				// For now, we follow the concept and proceed.
				body = []byte{}
			} else {
				body = buf.Bytes()
				if int64(len(body)) > maxBodySize {
					body = body[:maxBodySize]
					// Optional: mark as truncated if we add that field later
				}
				// Copy to a fresh byte slice because buf.Bytes() is a slice into the buffer
				body = append([]byte(nil), body...)
			}
		}

		// 3. Create snapshot
		snapshot := &RequestSnapshot{
			Method:    r.Method,
			URL:       cloneURL(r.URL),
			Headers:   r.Header.Clone(),
			Body:      body,
			Host:      r.Host,
			Timestamp: time.Now(),
		}

		// 4. Inject into context
		ctx := context.WithValue(r.Context(), SnapshotKey, snapshot)
		r = r.WithContext(ctx)

		// 5. Restore r.Body for downstream compatibility
		r.Body = io.NopCloser(bytes.NewReader(snapshot.Body))

		next.ServeHTTP(w, r)
	})
}

// cloneURL provides a deep copy of a URL.
func cloneURL(u *url.URL) *url.URL {
	if u == nil {
		return nil
	}

	u2 := *u
	if u.User != nil {
		u2.User = new(url.Userinfo)
		*u2.User = *u.User
	}

	return &u2
}
