.DEFAULT_GOAL := help

.PHONY: help dev build test lint bench fuzz accuracy

help: ## Show available targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "} {printf "  %-8s %s\n", $$1, $$2}'

# Both processes stop when either one does, so a failed API start doesn't
# leave the editor running against nothing, and make exits with the status
# of the one that stopped.
dev: frontend/node_modules backend/bin/spellcheck ## Run the API and the editor at http://localhost:5173
	@backend/bin/spellcheck & api=$$!; \
	frontend/node_modules/.bin/vite frontend --port 5173 --strictPort & web=$$!; \
	trap 'kill $$api $$web 2>/dev/null' EXIT INT TERM; \
	while kill -0 $$api 2>/dev/null && kill -0 $$web 2>/dev/null; do sleep 1; done; \
	if kill -0 $$api 2>/dev/null; then wait $$web; else wait $$api; fi

build: backend/bin/spellcheck frontend/dist ## Build the service binary and the static editor

backend/bin/spellcheck: backend/go.mod $(shell find backend -name '*.go' -not -name '*_test.go') backend/internal/spell/data/words.txt
	go -C backend build -o bin/spellcheck ./cmd/spellcheck

frontend/dist: frontend/node_modules frontend/index.html frontend/vite.config.ts frontend/tsconfig.json $(shell find frontend/src -type f)
	npm --prefix frontend run build
	@touch $@

test: frontend/node_modules ## Run all tests
	go -C backend test -race -short ./...
	go -C backend test ./...
	npm --prefix frontend test

lint: frontend/node_modules ## Lint the Go code and type-check the TypeScript
	cd backend && golangci-lint run
	npm --prefix frontend run typecheck

bench: ## Run the Go benchmarks
	go -C backend test -run '^$$' -bench . -benchmem ./...

fuzz: ## Fuzz the edit distance against the reference implementation for 30 seconds
	go -C backend test -run '^$$' -fuzz FuzzOSA -fuzztime 30s ./internal/spell

accuracy: ## Measure suggestions on Norvig's misspellings and false alarms on his corpus (downloads them)
	@dir=$$(mktemp -d) && \
	for f in spell-testset1.txt spell-testset2.txt big.txt; do \
		curl -fsSL -o $$dir/$$f https://norvig.com/$$f || exit 1; \
	done && \
	SPELL_TESTSETS=$$dir go -C backend test -count=1 -run 'TestValidationAccuracy|TestRankingVariants|TestFalseAlarms' -v ./internal/spell; \
	status=$$?; rm -rf $$dir; exit $$status

frontend/node_modules: frontend/package-lock.json
	npm --prefix frontend ci
	@touch $@
