package lightpandarod

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-rod/rod"
)

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

func TestNewAppliesOptions(t *testing.T) {
	p := New(
		WithBinary("/tmp/lightpanda"),
		WithHost("0.0.0.0"),
		WithPort(9444),
		WithArgs("--foo", "--bar"),
	)

	if p.binary != "/tmp/lightpanda" {
		t.Fatalf("binary = %q, want %q", p.binary, "/tmp/lightpanda")
	}

	if p.host != "0.0.0.0" {
		t.Fatalf("host = %q, want %q", p.host, "0.0.0.0")
	}

	if p.port != 9444 {
		t.Fatalf("port = %d, want %d", p.port, 9444)
	}

	wantArgs := []string{"--foo", "--bar"}
	if len(p.args) != len(wantArgs) {
		t.Fatalf("args length = %d, want %d", len(p.args), len(wantArgs))
	}

	for i, want := range wantArgs {
		if p.args[i] != want {
			t.Fatalf("args[%d] = %q, want %q", i, p.args[i], want)
		}
	}
}

func TestLaunchRejectsSecondCall(t *testing.T) {
	p := New()
	p.startProcess = func(context.Context, processConfig) (processHandle, error) {
		return noopProcessHandle{}, nil
	}
	p.connectBrowser = func(context.Context, connectConfig) (*rod.Browser, resourceCloser, error) {
		return &rod.Browser{}, noopCloser{}, nil
	}

	if _, err := p.Launch(context.Background()); err != nil {
		t.Fatalf("first launch failed: %v", err)
	}

	if _, err := p.Launch(context.Background()); err == nil {
		t.Fatal("second launch succeeded, want error")
	}
}

func TestLaunchStillRejectsSecondCallAfterClose(t *testing.T) {
	p := New()
	p.startProcess = func(context.Context, processConfig) (processHandle, error) {
		return noopProcessHandle{}, nil
	}
	p.connectBrowser = func(context.Context, connectConfig) (*rod.Browser, resourceCloser, error) {
		return &rod.Browser{}, noopCloser{}, nil
	}

	if _, err := p.Launch(context.Background()); err != nil {
		t.Fatalf("first launch failed: %v", err)
	}

	if err := p.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if _, err := p.Launch(context.Background()); err == nil {
		t.Fatal("second launch succeeded after Close, want error")
	}
}

func TestLaunchFailsWhenAddressAlreadyInUse(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	p := New(WithPort(port))
	p.startProcess = func(context.Context, processConfig) (processHandle, error) {
		t.Fatal("process should not start when address is already in use")
		return nil, nil
	}

	if _, err := p.Launch(context.Background()); err == nil {
		t.Fatal("Launch succeeded, want address-in-use error")
	}
}

func TestLaunchRetriesProtocolConnectionUntilContextExpires(t *testing.T) {
	var attempts atomic.Int32

	p := New()
	p.retryDelay = time.Millisecond
	p.startProcess = func(context.Context, processConfig) (processHandle, error) {
		return noopProcessHandle{}, nil
	}
	p.connectBrowser = func(context.Context, connectConfig) (*rod.Browser, resourceCloser, error) {
		attempts.Add(1)
		return nil, nil, errors.New("not ready yet")
	}
	p.dialAddress = func(context.Context, string) error {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	if _, err := p.Launch(ctx); err == nil {
		t.Fatal("Launch succeeded, want timeout")
	}

	if attempts.Load() < 2 {
		t.Fatalf("connect attempts = %d, want at least 2", attempts.Load())
	}
}

func TestLaunchCleansUpAfterConnectFailure(t *testing.T) {
	var closed atomic.Bool

	p := New()
	p.retryDelay = time.Millisecond
	p.startProcess = func(context.Context, processConfig) (processHandle, error) {
		return processHandleFunc(func() error {
			closed.Store(true)
			return nil
		}), nil
	}
	p.connectBrowser = func(ctx context.Context, cfg connectConfig) (*rod.Browser, resourceCloser, error) {
		<-ctx.Done()
		return nil, nil, errors.New("connect failed")
	}
	p.dialAddress = func(context.Context, string) error {
		return errors.New("not listening")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	if _, err := p.Launch(ctx); err == nil {
		t.Fatal("Launch succeeded, want connect failure")
	}

	if !closed.Load() {
		t.Fatal("process cleanup did not run after partial launch failure")
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	p := New()

	if err := p.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}

	if err := p.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}
}

func TestCloseAggregatesShutdownErrors(t *testing.T) {
	p := New()
	p.browserCloser = resourceCloserFunc(func() error {
		return errors.New("browser close")
	})
	p.process = processHandleFunc(func() error {
		return errors.New("process close")
	})

	err := p.Close()
	if err == nil {
		t.Fatal("Close succeeded, want combined error")
	}

	if !strings.Contains(err.Error(), "browser close") {
		t.Fatalf("Close error = %v, want browser close error", err)
	}

	if !strings.Contains(err.Error(), "process close") {
		t.Fatalf("Close error = %v, want process close error", err)
	}
}

func TestCloseIgnoresAlreadyClosedBrowserError(t *testing.T) {
	var processClosed atomic.Bool

	p := New()
	p.browserCloser = resourceCloserFunc(func() error {
		return net.ErrClosed
	})
	p.process = processHandleFunc(func() error {
		processClosed.Store(true)
		return nil
	})

	if err := p.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if !processClosed.Load() {
		t.Fatal("process close did not run")
	}
}

type noopCloser struct{}

func (noopCloser) Close() error {
	return nil
}

type noopProcessHandle struct{}

func (noopProcessHandle) Close() error {
	return nil
}

type processHandleFunc func() error

func (fn processHandleFunc) Close() error {
	return fn()
}
