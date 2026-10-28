# API

The service has two routes.

## `POST /v1/check`

```bash
curl -s localhost:8080/v1/check -H 'Content-Type: application/json' \
  -d '{"words": ["Teh", "quick", "dont"]}'
```

```json
{"version":"d0ca44581fd08c15-<commit>",
 "results":[
  {"word":"Teh","correct":false,"suggestions":["The","Tech","Tel","Ten","Tea"]},
  {"word":"quick","correct":true,"suggestions":[]},
  {"word":"dont","correct":false,"suggestions":["don't","donut","done","font","dot"]}
]}
```

### Request

- `Content-Type: application/json`, a body of at most 1 MiB.
- `words` holds 0 to 1000 entries. Each must be a single word: 1 to 64 characters (Unicode code points), at least one letter, no spaces or control characters.
- Words are compared as sent, so send them in NFC; the editor does.
- Members the server doesn't know are ignored, so a newer client can add a field while an older server is still running. Duplicate members, invalid UTF-8 and data after the object are rejected.

### Response

- `results` come back in request order. `suggestions` holds up to 5 fixes, best first, capitalized like the input; a fix may be two words, as "a lot" for "alot". It is empty when the word is correct, or when no word is within 2 edits; a word of one or two letters also needs a fix that keeps one of its letters, so "он" does not get "of".
- `version` identifies the checker: the word list's hash, then the twelve-character commit it was built from, or `dev` when run with `go run`. Answers only change when it changes, so a client may cache them until it sees a new value; the editor does.
- Every response the handler produces, success or error, carries an `X-Request-Id` header. The same ID is on the server's log line for the request. IDs are issued by the server; one sent by a client or proxy is not reflected.

### Errors

Errors are RFC 9457 `application/problem+json`. `instance` names the request by its ID, so a client can quote it and the operator can find the request in the logs.

| Status | Cause |
|---|---|
| 400 | Malformed JSON |
| 404 | Unknown path |
| 405 | Wrong method; comes with `Allow` |
| 413 | Body too large |
| 415 | Wrong content type; comes with `Accept-Post` |
| 422 | Invalid words; every problem is listed with a JSON Pointer |
| 500 | A bug in the handler; the log line has the stack |

An example 422:

```json
{"type":"about:blank","title":"Unprocessable Entity","status":422,
 "detail":"some words can't be checked",
 "instance":"urn:request:k7m2p3q4x5c6v3z7",
 "errors":[{"pointer":"/words/1","detail":"must be a single word of 1 to 64 characters"}]}
```

A client that gets a 422 can keep the words the pointers don't name; the editor asks about those again and gives up only on the named ones.

Two cases come from Go's `net/http` rather than the handler: a path that is not in canonical form, such as `//v1/check`, gets a 307 redirect to the clean path, and a request the server cannot parse gets a plain-text error.

## `GET /healthz`

Returns `ok` and a newline while the server is up. The service only starts listening once the dictionary is built, so up also means ready.

## Running the service

```
spellcheck [-addr 127.0.0.1:8080] [-dictionary words.txt]
```

- `-addr` defaults to loopback; use `:8080` in a container.
- `-dictionary` serves a word list from a file instead of the built-in one, one `word<TAB>count` per line in UTF-8, so a deployment can ship its own list without a rebuild. A byte order mark and surrounding whitespace are ignored; invalid UTF-8 and an empty list are refused at startup.

It logs JSON lines to stderr: the build at startup, then the dictionary's version, size and build time, then one line per request with its ID, method, route pattern, status, duration in nanoseconds and, for checks, how many words were checked. The words themselves are never logged. A request its client abandoned before the answer was written is logged with status 499. Health checks are logged at debug level, which the default level hides. It shuts down gracefully on SIGTERM or Ctrl-C: it stops accepting at once and gives in-flight requests up to 10 seconds; a second Ctrl-C exits immediately.
