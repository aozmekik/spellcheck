package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestRun starts the service the way main does, on a free port, checks words
// over real HTTP, then cancels the context as SIGTERM would and expects a
// clean shutdown.
func TestRun(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	logs, logWriter := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, []string{"-addr", "127.0.0.1:0"}, logWriter)
		logWriter.Close()
	}()

	// The address is only known once the service logs it.
	lines := bufio.NewScanner(logs)
	var addr string
	for addr == "" && lines.Scan() {
		if e := decodeLog(t, lines.Bytes()); e.Msg == "listening" {
			addr = e.Addr
		}
	}
	if addr == "" {
		t.Fatalf("the service stopped before listening: %v", <-done)
	}
	// Keep reading: the service blocks on a log line nobody reads.
	rest := make(chan []string, 1)
	go func() {
		var raw []string
		for lines.Scan() {
			raw = append(raw, lines.Text())
		}
		rest <- raw
	}()

	if body := request(t, http.MethodGet, "http://"+addr+"/healthz", ""); body != "ok\n" {
		t.Errorf("/healthz = %q, want \"ok\\n\"", body)
	}
	body := request(t, http.MethodPost, "http://"+addr+"/v1/check", `{"words": ["recieve", "receive"]}`)
	var resp struct {
		Results []struct {
			Correct     bool     `json:"correct"`
			Suggestions []string `json:"suggestions"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decoding %s: %v", body, err)
	}
	if len(resp.Results) != 2 || resp.Results[0].Correct || !resp.Results[1].Correct ||
		len(resp.Results[0].Suggestions) == 0 || resp.Results[0].Suggestions[0] != "receive" {
		t.Errorf("checking recieve and receive: %s", body)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("run: %v", err)
	}
	var entries []logEntry
	for _, line := range <-rest {
		entries = append(entries, decodeLog(t, []byte(line)))
	}
	if !slices.ContainsFunc(entries, func(e logEntry) bool { return e.Route == "POST /v1/check" && e.Status == http.StatusOK }) {
		t.Errorf("no request log line for the check: %+v", entries)
	}
	if !slices.ContainsFunc(entries, func(e logEntry) bool { return e.Msg == "stopped" }) {
		t.Errorf("no \"stopped\" log line after the shutdown: %+v", entries)
	}
}

func TestRunRejectsUnknownFlags(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	err := run(t.Context(), []string{"-bogus"}, &stderr)
	if !errors.Is(err, errUsage) {
		t.Errorf("run(-bogus) = %v, want errUsage", err)
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined: -bogus") || !strings.Contains(stderr.String(), "Usage of spellcheck") {
		t.Errorf("stderr does not explain the flag:\n%s", stderr.String())
	}
}

func TestLoadDictionaryFromFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "words.txt")
	if err := os.WriteFile(path, []byte("zzyzx\t1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dict, err := loadDictionary(path)
	if err != nil {
		t.Fatal(err)
	}
	if dict.Len() != 1 || !dict.Check("zzyzx").Correct {
		t.Errorf("the dictionary from %s does not hold zzyzx: %d words", path, dict.Len())
	}
	if _, err := loadDictionary(filepath.Join(t.TempDir(), "missing.txt")); err == nil {
		t.Error("loading a missing file succeeded")
	}
}

type logEntry struct {
	Msg    string `json:"msg"`
	Addr   string `json:"addr"`
	Route  string `json:"route"`
	Status int    `json:"status"`
}

func decodeLog(t *testing.T, line []byte) logEntry {
	t.Helper()
	var e logEntry
	if err := json.Unmarshal(line, &e); err != nil {
		t.Errorf("log line %q is not JSON: %v", line, err)
	}
	return e
}

func request(t *testing.T, method, url, body string) string {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("%s %s: status %d, body %s", method, url, res.StatusCode, b)
	}
	return string(b)
}
