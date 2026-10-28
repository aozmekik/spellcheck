// Package httpapi serves spell checks over HTTP.
package httpapi

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/aozmekik/spellcheck/backend/internal/spell"
)

// Limits on a check request. Even 1000 words of 64 escaped emoji stay under
// the body limit.
const (
	maxBodyBytes = 1 << 20
	maxWords     = 1000
	maxWordRunes = 64
)

// Checker checks the spelling of one word against a word list.
type Checker interface {
	Check(word string) spell.Result
	// Version identifies the word list; it changes whenever the list does.
	Version() string
}

// New returns the API's HTTP handler. build identifies the running code; it
// becomes part of the version clients cache answers under.
func New(checker Checker, build string, logger *slog.Logger) http.Handler {
	s := &server{checker: checker, version: checker.Version() + "-" + build, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/check", s.check)
	mux.HandleFunc("GET /healthz", healthz)
	// Without these, the mux would answer wrong methods and unknown paths in
	// plain text rather than problem details.
	mux.HandleFunc("/v1/check", s.methodNotAllowed(http.MethodPost))
	mux.HandleFunc("/healthz", s.methodNotAllowed(http.MethodGet, http.MethodHead))
	mux.HandleFunc("/", s.notFound)
	return logRequests(logger, recoverPanics(logger, mux))
}

type server struct {
	checker Checker
	version string
	logger  *slog.Logger
}

type checkRequest struct {
	Words []string `json:"words"`
}

type checkResponse struct {
	// Version identifies the checker: the word list and the build. Answers
	// only change when it does, so a client may cache them until it sees a
	// new value.
	Version string       `json:"version"`
	Results []wordResult `json:"results"`
}

type wordResult struct {
	Word        string   `json:"word"`
	Correct     bool     `json:"correct"`
	Suggestions []string `json:"suggestions"`
}

func (s *server) check(w http.ResponseWriter, r *http.Request) {
	if mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mediaType != "application/json" {
		w.Header().Set("Accept-Post", "application/json")
		writeProblem(s.logger, w, r, http.StatusUnsupportedMediaType, "send a JSON body with Content-Type: application/json")
		return
	}

	// Strict about what is there: duplicate members, invalid UTF-8 and
	// trailing data are errors rather than silently accepted. Unknown members
	// are ignored, so a newer client can add a field and still talk to an
	// older server during a rollout.
	var req checkRequest
	body := http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := json.UnmarshalRead(body, &req); err != nil {
		if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge {
			writeProblem(s.logger, w, r, http.StatusRequestEntityTooLarge, fmt.Sprintf("the body must be at most %d bytes", maxBodyBytes))
			return
		}
		writeProblem(s.logger, w, r, http.StatusBadRequest, `the body must be a JSON object like {"words": ["teh"]}`)
		return
	}
	if errs := validate(req); len(errs) > 0 {
		writeProblem(s.logger, w, r, http.StatusUnprocessableEntity, "some words can't be checked", errs...)
		return
	}
	if info := infoFrom(r.Context()); info != nil {
		info.words = len(req.Words)
	}

	resp := checkResponse{Version: s.version, Results: make([]wordResult, len(req.Words))}
	for i, word := range req.Words {
		// Checks run one after another: Go already runs requests in parallel,
		// and fanning out inside one would let a single large request take
		// every core. The largest costs 0.2 s of CPU, so stop for a client
		// that has gone away; the log line then says 499.
		if i%64 == 63 && r.Context().Err() != nil {
			return
		}
		res := s.checker.Check(word)
		resp.Results[i] = wordResult{Word: word, Correct: res.Correct, Suggestions: res.Suggestions}
	}
	writeJSON(s.logger, w, r, http.StatusOK, resp)
}

// validate reports every invalid part of req, each located by a JSON Pointer.
func validate(req checkRequest) []fieldError {
	switch {
	case req.Words == nil:
		return []fieldError{{Pointer: "/words", Detail: "is required"}}
	case len(req.Words) > maxWords:
		return []fieldError{{Pointer: "/words", Detail: fmt.Sprintf("must hold at most %d words", maxWords)}}
	}
	var errs []fieldError
	for i, word := range req.Words {
		if !isWord(word) {
			errs = append(errs, fieldError{
				Pointer: fmt.Sprintf("/words/%d", i),
				Detail:  fmt.Sprintf("must be a single word of 1 to %d characters", maxWordRunes),
			})
		}
	}
	return errs
}

// isWord reports whether s can be a word: 1 to maxWordRunes characters, at
// least one of them a letter, and no spaces or control characters.
func isWord(s string) bool {
	n := utf8.RuneCountInString(s)
	return n > 0 && n <= maxWordRunes &&
		strings.ContainsFunc(s, unicode.IsLetter) &&
		!strings.ContainsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, "ok\n")
}

func (s *server) methodNotAllowed(methods ...string) http.HandlerFunc {
	allow := strings.Join(methods, ", ")
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", allow)
		writeProblem(s.logger, w, r, http.StatusMethodNotAllowed, "allowed methods: "+allow)
	}
}

func (s *server) notFound(w http.ResponseWriter, r *http.Request) {
	writeProblem(s.logger, w, r, http.StatusNotFound, "")
}
