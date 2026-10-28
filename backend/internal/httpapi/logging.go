package httpapi

import (
	"context"
	"crypto/rand"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

// statusClientClosed is logged for a request the client abandoned before an
// answer was written. It is nginx's convention; it never goes on the wire.
const statusClientClosed = 499

// requestInfo travels with a request: the ID that ties its log line, its
// X-Request-Id header and any problem details together, and what the handler
// learns about it for the log line.
type requestInfo struct {
	id    string
	words int // how many words a check request held
}

type requestInfoKey struct{}

func infoFrom(ctx context.Context) *requestInfo {
	info, _ := ctx.Value(requestInfoKey{}).(*requestInfo)
	return info
}

// logRequests gives each request an ID and logs one line for it. Nothing
// between here and the ServeMux may replace the request: the mux records the
// matched route pattern on the request it is given, and logging that pattern
// instead of the path keeps user text out of the logs. IDs are issued here,
// never taken from the client, so a proxy's own ID is not reflected.
func logRequests(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		info := &requestInfo{id: newRequestID()}
		w.Header().Set("X-Request-Id", info.id)
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		r = r.WithContext(context.WithValue(r.Context(), requestInfoKey{}, info))
		next.ServeHTTP(rec, r) // the mux records the matched pattern on this request

		status := rec.status
		if !rec.wrote && r.Context().Err() != nil {
			status = statusClientClosed
		}
		level := slog.LevelInfo
		if r.URL.Path == "/healthz" {
			level = slog.LevelDebug // probes would drown out everything else
		}
		attrs := []slog.Attr{
			slog.String("request_id", info.id),
			slog.String("method", r.Method),
			slog.String("route", r.Pattern),
			slog.Int("status", status),
			slog.Duration("duration", time.Since(start)),
		}
		if info.words > 0 {
			attrs = append(attrs, slog.Int("words", info.words))
		}
		logger.LogAttrs(r.Context(), level, "request", attrs...)
	})
}

// recoverPanics turns a panic in a handler into a 500 with problem details
// and a log line, instead of a dropped connection with no request log.
func recoverPanics(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			p := recover()
			if p == nil {
				return
			}
			if err, ok := p.(error); ok && err == http.ErrAbortHandler { //nolint:errorlint // a sentinel compared by identity, as net/http does
				panic(p)
			}
			attrs := []slog.Attr{slog.Any("panic", p), slog.String("stack", string(debug.Stack()))}
			if info := infoFrom(r.Context()); info != nil {
				attrs = append(attrs, slog.String("request_id", info.id))
			}
			logger.LogAttrs(r.Context(), slog.LevelError, "handler panicked", attrs...)
			if rec, ok := w.(*statusRecorder); !ok || !rec.wrote {
				writeProblem(logger, w, r, http.StatusInternalServerError, "")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// newRequestID returns 16 random base32 characters: unguessable, and unique
// enough to find one request among a server's logs.
func newRequestID() string {
	return strings.ToLower(rand.Text()[:16])
}

// statusRecorder remembers the status code a handler sends, and whether it
// sent anything at all.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.wrote = true
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.wrote = true
	return r.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
