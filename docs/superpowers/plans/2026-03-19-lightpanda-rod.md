# Lightpanda Rod Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a small Go library that launches a local `lightpanda serve` process, connects it to go-rod, and returns a ready `*rod.Browser` with safe cleanup semantics.

**Architecture:** Keep the package split into three focused files: `provider.go` for public API and lifecycle, `process.go` for child-process management, and `connect.go` for the websocket/CDP-to-Rod connection path. Use internal function injection on `Provider` to keep the production API small while enabling deterministic unit tests for launch, partial-failure cleanup, and shutdown aggregation.

**Tech Stack:** Go, standard library (`context`, `net`, `os/exec`, `errors`, `sync`), `github.com/go-rod/rod`, `github.com/go-rod/rod/lib/cdp`

---

## Chunk 1: Module Bootstrap And Provider Contract

### Task 1: Create the initial module skeleton

**Files:**
- Create: `go.mod`
- Create: `provider.go`
- Create: `process.go`
- Create: `connect.go`
- Create: `provider_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestNewAppliesDefaults(t *testing.T) {
	p := New()

	if p.binary != "lightpanda" {
		t.Fatalf("binary = %q, want %q", p.binary, "lightpanda")
	}
	if p.host != "127.0.0.1" {
		t.Fatalf("host = %q, want %q", p.host, "127.0.0.1")
	}
	if p.port != 9222 {
		t.Fatalf("port = %d, want %d", p.port, 9222)
	}
	if len(p.args) != 0 {
		t.Fatalf("args = %v, want empty", p.args)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestNewAppliesDefaults`
Expected: FAIL because the package and `New` do not exist yet

- [ ] **Step 3: Write minimal implementation**

```go
type Provider struct {
	binary string
	host   string
	port   int
	args   []string
}

func New(opts ...Option) *Provider {
	p := &Provider{
		binary: "lightpanda",
		host:   "127.0.0.1",
		port:   9222,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestNewAppliesDefaults`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go.mod provider.go process.go connect.go provider_test.go
git commit -m "feat: bootstrap lightpanda rod provider package"
```

### Task 2: Lock in option behavior

**Files:**
- Modify: `provider.go`
- Modify: `provider_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestNewAppliesOptions(t *testing.T) {
	p := New(
		WithBinary("/tmp/lightpanda"),
		WithHost("0.0.0.0"),
		WithPort(9444),
		WithArgs("--foo", "--bar"),
	)

	if p.binary != "/tmp/lightpanda" || p.host != "0.0.0.0" || p.port != 9444 {
		t.Fatalf("options were not applied: %#v", p)
	}
	if !reflect.DeepEqual(p.args, []string{"--foo", "--bar"}) {
		t.Fatalf("args = %v", p.args)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestNewAppliesOptions`
Expected: FAIL because option functions do not exist yet

- [ ] **Step 3: Write minimal implementation**

```go
type Option func(*Provider)

func WithBinary(path string) Option { return func(p *Provider) { p.binary = path } }
func WithHost(host string) Option   { return func(p *Provider) { p.host = host } }
func WithPort(port int) Option      { return func(p *Provider) { p.port = port } }
func WithArgs(args ...string) Option {
	return func(p *Provider) {
		p.args = append([]string(nil), args...)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestNewAppliesOptions`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add provider.go provider_test.go
git commit -m "feat: add lightpanda provider options"
```

## Chunk 2: Launch Lifecycle And Failure Cleanup

### Task 3: Reject double launch

**Files:**
- Modify: `provider.go`
- Modify: `provider_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestLaunchRejectsSecondCall(t *testing.T) {
	p := New()
	p.startProcess = func(context.Context) (*exec.Cmd, processHandle, error) { return &exec.Cmd{}, noopProcessHandle{}, nil }
	p.connectBrowser = func(context.Context) (*rod.Browser, browserCloser, error) { return &rod.Browser{}, noopBrowserCloser{}, nil }

	if _, err := p.Launch(context.Background()); err != nil {
		t.Fatalf("first launch failed: %v", err)
	}

	if _, err := p.Launch(context.Background()); err == nil {
		t.Fatal("second launch succeeded, want error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestLaunchRejectsSecondCall`
Expected: FAIL because `Launch` does not exist yet

- [ ] **Step 3: Write minimal implementation**

```go
func (p *Provider) Launch(ctx context.Context) (*rod.Browser, error) {
	p.mu.Lock()
	if p.launched {
		p.mu.Unlock()
		return nil, errors.New("lightpandarod: launch already attempted")
	}
	p.launched = true
	p.mu.Unlock()
	return nil, errors.New("lightpandarod: not implemented")
}
```

- [ ] **Step 4: Run test to verify it fails for the right reason, then implement enough to pass**

Run: `go test ./... -run TestLaunchRejectsSecondCall`
Expected: first launch fails with the placeholder error

Then replace the placeholder path with real orchestration:

```go
cmd, proc, err := p.startProcess(ctx)
browser, closer, err := p.connectBrowser(ctx)
```

Store state only after success and return the browser.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./... -run TestLaunchRejectsSecondCall`
Expected: PASS

### Task 4: Fail fast when the configured address is already in use

**Files:**
- Modify: `provider.go`
- Modify: `process.go`
- Modify: `provider_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestLaunchFailsWhenAddressAlreadyInUse(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	p := New(WithPort(port))
	p.startProcess = func(context.Context) (*exec.Cmd, processHandle, error) {
		t.Fatal("process should not start when address is already in use")
		return nil, nil, nil
	}

	if _, err := p.Launch(context.Background()); err == nil {
		t.Fatal("Launch succeeded, want address-in-use error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestLaunchFailsWhenAddressAlreadyInUse`
Expected: FAIL because launch does not check the port first

- [ ] **Step 3: Write minimal implementation**

Add an `ensureAddressAvailable(ctx, host, port)` helper in `process.go` that attempts a short TCP dial and reports an error when a listener already exists.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestLaunchFailsWhenAddressAlreadyInUse`
Expected: PASS

### Task 5: Clean up partial launch failures

**Files:**
- Modify: `provider.go`
- Modify: `provider_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestLaunchCleansUpAfterConnectFailure(t *testing.T) {
	var closed bool

	p := New()
	p.startProcess = func(context.Context) (*exec.Cmd, processHandle, error) {
		return &exec.Cmd{}, processHandleFunc(func() error {
			closed = true
			return nil
		}), nil
	}
	p.connectBrowser = func(context.Context) (*rod.Browser, browserCloser, error) {
		return nil, nil, errors.New("connect failed")
	}

	if _, err := p.Launch(context.Background()); err == nil {
		t.Fatal("Launch succeeded, want connect failure")
	}
	if !closed {
		t.Fatal("process cleanup did not run after partial launch failure")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestLaunchCleansUpAfterConnectFailure`
Expected: FAIL because failed launch leaks the started process

- [ ] **Step 3: Write minimal implementation**

In `Launch`, if any step after process startup fails, call the same internal close path used by `Close` before returning the original launch error.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestLaunchCleansUpAfterConnectFailure`
Expected: PASS

## Chunk 3: Process Wiring, Shutdown Semantics, And Real Connection Logic

### Task 6: Implement idempotent close and shutdown aggregation

**Files:**
- Modify: `provider.go`
- Modify: `provider_test.go`

- [ ] **Step 1: Write the failing tests**

```go
func TestCloseIsIdempotent(t *testing.T) { /* call Close twice and expect nil */ }

func TestCloseAggregatesShutdownErrors(t *testing.T) {
	p := New()
	p.browserCloser = browserCloserFunc(func() error { return errors.New("browser close") })
	p.processCloser = processHandleFunc(func() error { return errors.New("process close") })

	err := p.Close()
	if err == nil || !strings.Contains(err.Error(), "browser close") || !strings.Contains(err.Error(), "process close") {
		t.Fatalf("Close error = %v, want combined error", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run 'TestClose(IsIdempotent|AggregatesShutdownErrors)'`
Expected: FAIL because `Close` is incomplete or missing

- [ ] **Step 3: Write minimal implementation**

Use `errors.Join` to combine close failures, clear stored closers after shutdown, and tolerate nil or already-cleared state.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run 'TestClose(IsIdempotent|AggregatesShutdownErrors)'`
Expected: PASS

### Task 7: Implement process startup and readiness retry behavior

**Files:**
- Modify: `process.go`
- Modify: `provider.go`
- Modify: `provider_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestLaunchRetriesProtocolConnectionUntilContextExpires(t *testing.T) {
	p := New()
	p.startProcess = func(context.Context) (*exec.Cmd, processHandle, error) { return &exec.Cmd{}, noopProcessHandle{}, nil }
	p.connectBrowser = func(context.Context) (*rod.Browser, browserCloser, error) {
		return nil, nil, errors.New("not ready yet")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if _, err := p.Launch(ctx); err == nil {
		t.Fatal("Launch succeeded, want timeout")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestLaunchRetriesProtocolConnectionUntilContextExpires`
Expected: FAIL because launch tries the protocol connection only once

- [ ] **Step 3: Write minimal implementation**

Add a retry loop in `Launch` that keeps attempting `connectBrowser(ctx)` until it succeeds or `ctx.Done()` fires. Use a short ticker and treat successful TCP dial as a hint to retry immediately, not as launch success on its own.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestLaunchRetriesProtocolConnectionUntilContextExpires`
Expected: PASS

### Task 8: Implement the real Rod/CDP connection path

**Files:**
- Modify: `connect.go`
- Modify: `process.go`
- Modify: `provider.go`

- [ ] **Step 1: Write the failing test**

Use a focused unit test that asserts the produced websocket URL / host-port wiring for a stubbed Lightpanda endpoint helper, or if the API shape makes that awkward, add a test for a small helper that builds the endpoint from configured host/port.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestConnect`
Expected: FAIL because `connect.go` still has placeholders

- [ ] **Step 3: Write minimal implementation**

Implement the production path that:

```go
endpoint := fmt.Sprintf("ws://%s:%d", p.host, p.port)
client := cdp.New().Start(cdp.MustConnectWS(endpoint))
browser := rod.New().Client(client)
```

Adjust to the actual current `rod` / `cdp` API after confirming with local module docs. Keep the public package API unchanged if the dependency API needs helper glue.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run TestConnect`
Expected: PASS

### Task 9: Verify the full package

**Files:**
- Modify: `provider_test.go` (only if additional coverage gaps remain)

- [ ] **Step 1: Run the focused suite**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 2: Refactor while staying green**

Tighten helper names, remove duplication in tests, and keep file responsibilities aligned with the spec.

- [ ] **Step 3: Run the full suite again**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum provider.go process.go connect.go provider_test.go docs/superpowers/plans/2026-03-19-lightpanda-rod.md
git commit -m "feat: implement lightpanda rod provider"
```
