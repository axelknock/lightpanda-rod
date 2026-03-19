# Lightpanda Rod Initial Design

## Goal

Create a small Go library that makes Lightpanda usable through go-rod with a locally managed `lightpanda serve` process and a high-level API that returns a ready `*rod.Browser`.

## Scope

This first version supports:

- launching a local Lightpanda child process
- waiting until the process is ready to accept connections
- connecting Rod to Lightpanda through the required WebSocket/CDP setup
- shutting down browser and process resources cleanly

This first version does not support:

- remote Lightpanda instances
- custom `exec.Cmd` hooks
- stdout/stderr routing configuration
- environment variable injection
- multi-backend browser providers

## Recommended Approach

Use a high-level provider API that owns both process lifecycle and Rod connection setup.

Alternative approaches considered:

1. Split `Launcher` and `Connect` as separate public primitives
   - More flexible, but leaks more setup concerns into caller code.
2. Clone Rod launcher semantics closely
   - Familiar, but optimizes for Chrome-shaped ergonomics rather than a Lightpanda-native abstraction.

The chosen approach keeps caller code simple while preserving internal separation so lower-level primitives can be exposed later without redesigning the package.

## Public API

```go
package lightpandarod

type Provider struct { ... }

func New(opts ...Option) *Provider
func (p *Provider) Launch(ctx context.Context) (*rod.Browser, error)
func (p *Provider) MustLaunch(ctx context.Context) *rod.Browser
func (p *Provider) Close() error

type Option func(*Provider)

func WithBinary(path string) Option
func WithHost(host string) Option
func WithPort(port int) Option
func WithArgs(args ...string) Option
```

### Defaults

- binary: `lightpanda`
- host: `127.0.0.1`
- port: `9222`
- extra args: none

### Semantics

- `New` applies defaults plus user options.
- `Launch` starts Lightpanda, waits for readiness, establishes the Rod/CDP connection, and returns a ready browser.
- `MustLaunch` panics on launch error.
- `Close` is safe to call multiple times and releases browser, websocket, and process resources.

### State Model

The provider supports a single successful launch in its lifetime.

- `Launch` may be called exactly once on a given provider instance.
- a second `Launch` call returns an error, even if `Close` has already been called
- `Close` is idempotent and may be called before or after a failed `Launch`
- if the caller independently closes the returned browser, `Close` still attempts to release remaining resources and treats already-closed browser state as non-fatal

## Internal Structure

Use three files with distinct responsibilities:

- `provider.go`
  - public API
  - option parsing
  - lifecycle orchestration
  - shutdown coordination
- `process.go`
  - `exec.Cmd` construction
  - child process startup
  - readiness polling via TCP dial
  - process termination
- `connect.go`
  - websocket setup
  - Lightpanda-specific handshake details
  - CDP client creation
  - Rod browser wiring

This split keeps the initial package small while preserving clear boundaries between process management and protocol connection logic.

## Launch Flow

`Launch(ctx)` executes the following sequence:

1. Reject a second launch attempt on the same provider.
2. Resolve defaults and options.
3. Confirm the configured TCP address is not already accepting connections before process start. If it is occupied, fail immediately rather than attempting to attach to an unknown service.
4. Start `lightpanda serve --host <host> --port <port>` plus any extra args.
5. Repeatedly attempt the real websocket/CDP connection flow until it succeeds or the context expires. A raw TCP accept is treated only as a hint for retry timing, not as proof of readiness.
6. Once the websocket/CDP handshake succeeds, create the Rod browser and return it.

## Shutdown Flow

`Close()` executes the following sequence:

1. Close browser or websocket state if initialized.
2. Terminate the child process if still running.
3. Clear internal state so repeated `Close` calls are harmless.
4. Return a combined error if multiple shutdown steps fail.

## Failure Handling

If a failure happens after the child process starts but before launch completes, the provider must clean up partial state before returning the original error.

Expected error cases:

- launch called twice on the same provider
- configured address already in use before launch
- binary startup failure
- readiness timeout or context cancellation
- websocket connection failure
- CDP or Rod setup failure

Custom error types are not needed in the first version. Wrapped errors are sufficient.

## Testing Strategy

The initial implementation should focus on deterministic unit tests:

- defaults and option application
- double launch rejection
- configured-address-in-use failure before process start
- readiness timeout behavior based on failed protocol connection, not just failed TCP dial
- cleanup after partial launch failure
- idempotent close behavior
- caller-closed-browser behavior during provider shutdown
- shutdown error aggregation where practical to simulate

Integration tests against a real Lightpanda binary are intentionally deferred until the API and test environment expectations are clearer.

## Package Evolution

This design leaves room for future additions without breaking the initial shape:

- exposing lower-level launcher/connector primitives
- adding remote endpoint support
- adding richer process options
- introducing a broader browser provider abstraction
