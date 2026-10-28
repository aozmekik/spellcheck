package httpapi

import (
	"encoding/json/v2"
	"log/slog"
	"net/http"
)

// problem is an RFC 9457 problem details object.
type problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
	// Instance names this occurrence by its request ID, so a client can
	// quote it and the operator can find the request in the logs.
	Instance string       `json:"instance,omitempty"`
	Errors   []fieldError `json:"errors,omitempty"`
}

// fieldError points at one invalid part of a request.
type fieldError struct {
	Pointer string `json:"pointer"`
	Detail  string `json:"detail"`
}

func writeProblem(logger *slog.Logger, w http.ResponseWriter, r *http.Request, status int, detail string, errs ...fieldError) {
	p := problem{
		Type:   "about:blank",
		Title:  http.StatusText(status),
		Status: status,
		Detail: detail,
		Errors: errs,
	}
	if info := infoFrom(r.Context()); info != nil {
		p.Instance = "urn:request:" + info.id
	}
	w.Header().Set("Content-Type", "application/problem+json")
	writeBody(logger, w, r, status, p)
}

func writeJSON(logger *slog.Logger, w http.ResponseWriter, r *http.Request, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	writeBody(logger, w, r, status, v)
}

func writeBody(logger *slog.Logger, w http.ResponseWriter, r *http.Request, status int, v any) {
	w.WriteHeader(status)
	// The status line is already sent, so a failure here can't change the
	// answer. Usually it means the client went away; a value the encoder
	// refuses would mean a bug, so it is logged.
	if err := json.MarshalWrite(w, v); err != nil && r.Context().Err() == nil {
		logger.LogAttrs(r.Context(), slog.LevelError, "writing the response", slog.Any("error", err))
	}
}
