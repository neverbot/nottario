package web

import "net/http"

// maxRequestBodyBytes caps every request body the server will read.
//
// Nothing bounded bodies before, so any authenticated caller could make
// the server buffer and decode an arbitrarily large JSON payload — and
// POST /api/docs/write then stores the body twice, in the document row
// and again in its version history, so repeated large writes grew the
// database without limit. While documents only arrived through the MCP
// tool the body was model output and naturally small; scripted uploads
// through REST with the same token removed that implicit ceiling.
//
// 8 MiB is far beyond any real markdown document and still small
// enough that the worst case is harmless.
const maxRequestBodyBytes = 8 << 20

// withBodyLimit enforces maxRequestBodyBytes on every route, the MCP
// endpoint included.
//
// A declared Content-Length over the cap is refused up front with 413,
// before a byte is read — that covers curl, browsers and every
// well-behaved client. A body without a declared length (chunked) is
// wrapped in MaxBytesReader, which stops reading at the cap; the
// handler then sees a decode error and answers 400. The status differs
// but the property that matters holds either way: the server never
// reads past the limit.
func withBodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > maxRequestBodyBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		next.ServeHTTP(w, r)
	})
}
