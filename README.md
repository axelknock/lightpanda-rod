# lightpanda-rod

`lightpanda-rod` is a small Go package that launches a local `lightpanda serve` process and returns a ready-to-use [`*rod.Browser`](https://github.com/go-rod/rod).

It is intentionally narrow in scope:

- starts a local Lightpanda child process
- waits until the browser is actually ready for a Rod/CDP session
- returns a connected Rod browser
- closes browser and process resources together

## Status

This is an early implementation aimed at local development and experimentation.

Current gaps:

- no remote Lightpanda support
- no environment or stdio configuration
- no custom `exec.Cmd` hooks
- no integration with multiple browser backends

## Installation

The repo currently uses this module path in [go.mod](/Users/lucian/.codex/worktrees/9368/lightpanda-rod/go.mod):

```go
module lightpanda-rod
```

That works for local development, but if you plan to consume it as a public module you will likely want the module path to match the GitHub repository path first.

Dependencies:

- Go 1.24+
- a `lightpanda` binary available on disk or in `PATH`

## Usage

```go
package main

import (
	"context"
	"log"
	"time"

	lightpandarod "lightpanda-rod"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	provider := lightpandarod.New()
	defer func() {
		if err := provider.Close(); err != nil {
			log.Printf("close failed: %v", err)
		}
	}()

	browser, err := provider.Launch(ctx)
	if err != nil {
		log.Fatal(err)
	}

	_ = browser
}
```

### Options

- `WithBinary(path string)` overrides the `lightpanda` executable path.
- `WithHost(host string)` sets the bind host. Default: `127.0.0.1`
- `WithPort(port int)` sets the bind port. Default: `9222`
- `WithArgs(args ...string)` appends extra arguments after `lightpanda serve --host ... --port ...`

### Lifecycle Semantics

- A `Provider` supports only one successful `Launch` attempt in its lifetime.
- A second `Launch` call returns an error, even after `Close`.
- `Close` is idempotent.
- If launch fails after the child process starts, the provider cleans up partial state before returning.

## Real-Binary Integration Check

The repo includes a Bash wrapper plus a Go probe that verifies the package against a real Lightpanda nightly binary.

Run:

```bash
./scripts/test-real-binary.sh
```

What it does:

- downloads a nightly `lightpanda` binary for supported platforms if one is not already present
- launches the package against that binary
- creates a page through Rod
- injects probe HTML into `about:blank`
- verifies the expected page title

Environment variables:

- `LIGHTPANDA_BINARY` overrides the binary path
- `LIGHTPANDA_TIMEOUT_SECONDS` sets the probe timeout
- `LIGHTPANDA_HOST` sets the bind host
- `LIGHTPANDA_PORT` sets the bind port; `0` chooses a free port

The downloaded nightly binary is cached under `.tmp/`, which is ignored by git.

## Development

Run unit tests:

```bash
go test ./...
```

Run the real-binary probe:

```bash
./scripts/test-real-binary.sh
```

## Notes

During integration testing, Lightpanda rejected a `data:` URL probe with `UrlMalformat`, so the probe intentionally uses `about:blank` plus DOM injection instead. That behavior is reflected in the integration script.
