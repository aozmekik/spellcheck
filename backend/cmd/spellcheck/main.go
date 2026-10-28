// Command spellcheck serves the spell-checking API.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/aozmekik/spellcheck/backend/internal/httpapi"
	"github.com/aozmekik/spellcheck/backend/internal/spell"
)

// errUsage means the flags were wrong; the flag package has already said how.
var errUsage = errors.New("usage")

func main() {
	err := run(context.Background(), os.Args[1:], os.Stderr)
	switch {
	case errors.Is(err, errUsage):
		os.Exit(2)
	case err != nil:
		fmt.Fprintln(os.Stderr, "spellcheck:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stderr io.Writer) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	flags := flag.NewFlagSet("spellcheck", flag.ContinueOnError)
	flags.SetOutput(stderr)
	addr := flags.String("addr", "127.0.0.1:8080", "address to listen on; use :8080 to accept outside connections, as in a container")
	dictionary := flags.String("dictionary", "", "word list to serve instead of the built-in one: one word<TAB>count per line")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return errUsage
	}
	logger := slog.New(slog.NewJSONHandler(stderr, nil))
	build := buildRevision()
	logger.Info("starting", slog.String("build", build))

	start := time.Now()
	dict, err := loadDictionary(*dictionary)
	if err != nil {
		return fmt.Errorf("loading dictionary: %w", err)
	}
	logger.Info("dictionary loaded",
		slog.String("version", dict.Version()),
		slog.Int("words", dict.Len()),
		slog.Duration("took", time.Since(start)),
		slog.Uint64("heap_mib", liveHeapMiB()),
	)

	srv := &http.Server{
		Handler:           httpapi.New(dict, build, logger),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}
	ln, err := new(net.ListenConfig).Listen(ctx, "tcp", *addr)
	if err != nil {
		return err
	}
	logger.Info("listening", slog.String("addr", ln.Addr().String()))

	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	select {
	case err := <-served:
		return fmt.Errorf("serving: %w", err)
	case <-ctx.Done():
	}
	stop() // from here on, a second Ctrl-C ends the process at once

	// Stop accepting connections and let in-flight requests finish.
	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutting down: %w", err)
	}
	logger.Info("stopped")
	return nil
}

// loadDictionary builds the dictionary from the word list at path, or the
// built-in list when path is empty. The built-in list makes the binary a
// single artifact; the flag lets a deployment ship its own list without a
// rebuild.
func loadDictionary(path string) (*spell.Dictionary, error) {
	if path == "" {
		return spell.Default()
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return spell.Load(f)
}

// buildRevision identifies the running code by its commit, as stamped by the
// Go toolchain when built from a checkout, or "dev" otherwise, as under go
// run. It tells instances apart in a rollout, and clients key their cached
// answers on it together with the word list.
func buildRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	revision, modified := "", false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if revision == "" {
		return "dev"
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	if modified {
		revision += "-dirty"
	}
	return revision
}

// liveHeapMiB reports the heap in use after a collection. At startup that is
// mostly the dictionary.
func liveHeapMiB() uint64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc >> 20
}
