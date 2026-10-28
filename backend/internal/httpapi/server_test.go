package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aozmekik/spellcheck/backend/internal/httpapi"
	"github.com/aozmekik/spellcheck/backend/internal/spell"
)

func newDictionary() *spell.Dictionary {
	return spell.New([]spell.Entry{
		{Word: "the", Count: 100},
		{Word: "quick", Count: 10},
		{Word: "London", Count: 5},
	})
}

func newHandler(logs io.Writer) http.Handler {
	return httpapi.New(newDictionary(), "test", slog.New(slog.NewJSONHandler(logs, nil)))
}

// countingChecker counts the words checked, to show when a loop stopped.
type countingChecker struct {
	*spell.Dictionary
	calls atomic.Int64
}

func (c *countingChecker) Check(word string) spell.Result {
	c.calls.Add(1)
	return c.Dictionary.Check(word)
}

// panickingChecker stands in for a bug in the checker.
type panickingChecker struct{ *spell.Dictionary }

func (panickingChecker) Check(string) spell.Result { panic("boom") }

func serve(t *testing.T, h http.Handler, method, target, contentType, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, target, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCheck(t *testing.T) {
	t.Parallel()

	dict := newDictionary()
	rec := serve(t, httpapi.New(dict, "test", slog.New(slog.DiscardHandler)), http.MethodPost, "/v1/check",
		"application/json; charset=utf-8", `{"words": ["Teh", "quick", "london"]}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if id := rec.Header().Get("X-Request-Id"); len(id) != 16 {
		t.Errorf("X-Request-Id = %q, want 16 characters", id)
	}
	want := `{"version":"` + dict.Version() + `-test","results":[` +
		`{"word":"Teh","correct":false,"suggestions":["The"]},` +
		`{"word":"quick","correct":true,"suggestions":[]},` +
		`{"word":"london","correct":false,"suggestions":["London"]}]}`
	if got := rec.Body.String(); got != want {
		t.Errorf("body = %s\nwant   %s", got, want)
	}
}

func TestCheckIgnoresUnknownMembers(t *testing.T) {
	t.Parallel()

	// A field this version doesn't know is not an error: a newer client may
	// send it to an older server during a rollout.
	rec := serve(t, newHandler(io.Discard), http.MethodPost, "/v1/check", "application/json", `{"words": ["quick"], "lang": "en"}`)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
}

func TestCheckEmptyList(t *testing.T) {
	t.Parallel()

	rec := serve(t, newHandler(io.Discard), http.MethodPost, "/v1/check", "application/json", `{"words": []}`)
	if rec.Code != http.StatusOK || !strings.HasSuffix(rec.Body.String(), `,"results":[]}`) {
		t.Errorf("got %d %s, want 200 with empty results", rec.Code, rec.Body)
	}
}

func TestCheckRejectsBadRequests(t *testing.T) {
	t.Parallel()

	tooMany := `{"words": [` + strings.Repeat(`"a",`, 1000) + `"a"]}`
	tooBig := `{"words": ["` + strings.Repeat("a", 1<<20) + `"]}`
	cases := []struct {
		name        string
		contentType string
		body        string
		status      int
		pointers    []string
	}{
		{"wrong content type", "text/plain", `{"words": ["a"]}`, http.StatusUnsupportedMediaType, nil},
		{"no content type", "", `{"words": ["a"]}`, http.StatusUnsupportedMediaType, nil},
		{"malformed JSON", "application/json", `{"words": [`, http.StatusBadRequest, nil},
		{"wrongly cased member", "application/json", `{"Words": ["a"]}`, http.StatusUnprocessableEntity, []string{"/words"}},
		{"duplicate member", "application/json", `{"words": ["a"], "words": ["b"]}`, http.StatusBadRequest, nil},
		{"invalid UTF-8", "application/json", "{\"words\": [\"a\xff\"]}", http.StatusBadRequest, nil},
		{"trailing data", "application/json", `{"words": ["a"]} {}`, http.StatusBadRequest, nil},
		{"body too large", "application/json", tooBig, http.StatusRequestEntityTooLarge, nil},
		{"missing words", "application/json", `{}`, http.StatusUnprocessableEntity, []string{"/words"}},
		{"null words", "application/json", `{"words": null}`, http.StatusUnprocessableEntity, []string{"/words"}},
		{"too many words", "application/json", tooMany, http.StatusUnprocessableEntity, []string{"/words"}},
		{
			"things that aren't words", "application/json",
			`{"words": ["fine", "", "` + strings.Repeat("é", 65) + `", "two words", "😀", "\u0000", null]}`,
			http.StatusUnprocessableEntity,
			[]string{"/words/1", "/words/2", "/words/3", "/words/4", "/words/5", "/words/6"},
		},
	}
	h := newHandler(io.Discard)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := serve(t, h, http.MethodPost, "/v1/check", tc.contentType, tc.body)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, tc.status, rec.Body)
			}
			p := decodeProblem(t, rec)
			var pointers []string
			for _, e := range p.Errors {
				pointers = append(pointers, e.Pointer)
			}
			if !slices.Equal(pointers, tc.pointers) {
				t.Errorf("error pointers = %q, want %q", pointers, tc.pointers)
			}
			if tc.status == http.StatusUnsupportedMediaType && rec.Header().Get("Accept-Post") != "application/json" {
				t.Errorf("Accept-Post = %q, want application/json", rec.Header().Get("Accept-Post"))
			}
		})
	}
}

func TestCheckStopsForAClientThatLeft(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // the client disconnected before the words were checked
	body := `{"words": [` + strings.Repeat(`"teh",`, 199) + `"teh"]}`
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/check", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	var logs bytes.Buffer
	checker := &countingChecker{Dictionary: newDictionary()}
	httpapi.New(checker, "test", slog.New(slog.NewJSONHandler(&logs, nil))).ServeHTTP(rec, req)

	if rec.Body.Len() != 0 {
		t.Errorf("a response was written for a client that had left: %s", rec.Body)
	}
	if n := checker.calls.Load(); n >= 200 {
		t.Errorf("all %d words were checked for a client that had left", n)
	}
	if !strings.Contains(logs.String(), `"status":499`) {
		t.Errorf("the log line does not say the client left: %s", logs.String())
	}
}

func TestPanicBecomesProblemDetails(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	h := httpapi.New(panickingChecker{newDictionary()}, "test", slog.New(slog.NewJSONHandler(&logs, nil)))
	rec := serve(t, h, http.MethodPost, "/v1/check", "application/json", `{"words": ["teh"]}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body: %s", rec.Code, rec.Body)
	}
	p := decodeProblem(t, rec)
	if p.Instance != "urn:request:"+rec.Header().Get("X-Request-Id") {
		t.Errorf("instance = %q, want the request ID", p.Instance)
	}
	if !strings.Contains(logs.String(), `"msg":"handler panicked"`) || !strings.Contains(logs.String(), `"status":500`) {
		t.Errorf("the panic and the 500 are not both logged: %s", logs.String())
	}
}

func TestCheckAcceptsTheLongestWord(t *testing.T) {
	t.Parallel()

	rec := serve(t, newHandler(io.Discard), http.MethodPost, "/v1/check", "application/json",
		`{"words": ["`+strings.Repeat("é", 64)+`"]}`)
	if rec.Code != http.StatusOK {
		t.Errorf("a 64-character word got %d: %s", rec.Code, rec.Body)
	}
}

func TestRouting(t *testing.T) {
	t.Parallel()

	cases := []struct {
		method, target string
		status         int
		allow          string
	}{
		{http.MethodGet, "/healthz", http.StatusOK, ""},
		{http.MethodHead, "/healthz", http.StatusOK, ""},
		{http.MethodPost, "/healthz", http.StatusMethodNotAllowed, "GET, HEAD"},
		{http.MethodGet, "/v1/check", http.StatusMethodNotAllowed, "POST"},
		{http.MethodGet, "/v1/words", http.StatusNotFound, ""},
		{http.MethodPost, "/v1/check/", http.StatusNotFound, ""},
	}
	h := newHandler(io.Discard)
	for _, tc := range cases {
		rec := serve(t, h, tc.method, tc.target, "", "")
		if rec.Code != tc.status {
			t.Errorf("%s %s: status = %d, want %d", tc.method, tc.target, rec.Code, tc.status)
			continue
		}
		if got := rec.Header().Get("Allow"); got != tc.allow {
			t.Errorf("%s %s: Allow = %q, want %q", tc.method, tc.target, got, tc.allow)
		}
		if tc.status != http.StatusOK {
			decodeProblem(t, rec)
		}
	}
}

func TestRequestLogsLeaveOutUserText(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	h := newHandler(&logs)
	serve(t, h, http.MethodPost, "/v1/check?note=confidential", "application/json", `{"words": ["confidential"]}`)
	serve(t, h, http.MethodGet, "/v1/words/confidential", "", "")
	serve(t, h, http.MethodGet, "/healthz", "", "")

	if strings.Contains(logs.String(), "confidential") {
		t.Errorf("the log contains user text: %s", logs.String())
	}
	// One line per request, labelled by route pattern, with the word count
	// for checks; health checks log at debug level, below the default.
	var got []string
	for line := range bytes.Lines(logs.Bytes()) {
		var entry struct {
			Msg       string `json:"msg"`
			Route     string `json:"route"`
			Status    int    `json:"status"`
			Words     int    `json:"words"`
			RequestID string `json:"request_id"`
		}
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("log line %q: %v", line, err)
		}
		if len(entry.RequestID) != 16 {
			t.Errorf("log line %q: request_id %q, want 16 characters", line, entry.RequestID)
		}
		got = append(got, fmt.Sprintf("%s %s %d words=%d", entry.Msg, entry.Route, entry.Status, entry.Words))
	}
	if want := []string{"request POST /v1/check 200 words=1", "request / 404 words=0"}; !slices.Equal(got, want) {
		t.Errorf("log lines = %q, want %q", got, want)
	}
}

func TestRequestIDTiesResponseAndLogTogether(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	rec := serve(t, newHandler(&logs), http.MethodPost, "/v1/check", "text/plain", "")

	id := rec.Header().Get("X-Request-Id")
	if len(id) != 16 {
		t.Fatalf("X-Request-Id = %q, want 16 characters", id)
	}
	if p := decodeProblem(t, rec); p.Instance != "urn:request:"+id {
		t.Errorf("problem instance = %q, want urn:request:%s", p.Instance, id)
	}
	if !strings.Contains(logs.String(), `"request_id":"`+id+`"`) {
		t.Errorf("the log line does not carry the request ID %s: %s", id, logs.String())
	}
}

type problemDetails struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail"`
	Instance string `json:"instance"`
	Errors   []struct {
		Pointer string `json:"pointer"`
		Detail  string `json:"detail"`
	} `json:"errors"`
}

func decodeProblem(t *testing.T, rec *httptest.ResponseRecorder) problemDetails {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
	var p problemDetails
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decoding problem details: %v; body: %s", err, rec.Body)
	}
	if p.Type != "about:blank" || p.Status != rec.Code || p.Title != http.StatusText(rec.Code) {
		t.Errorf("problem = %+v, want about:blank with status %d", p, rec.Code)
	}
	return p
}
