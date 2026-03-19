package lightpandarod

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/go-rod/rod"
)

const defaultRetryDelay = 10 * time.Millisecond

type Provider struct {
	binary string
	host   string
	port   int
	args   []string

	mu sync.Mutex

	launched bool

	process       processHandle
	browser       *rod.Browser
	browserCloser resourceCloser

	startProcess   func(context.Context, processConfig) (processHandle, error)
	connectBrowser func(context.Context, connectConfig) (*rod.Browser, resourceCloser, error)
	dialAddress    func(context.Context, string) error
	retryDelay     time.Duration
}

type Option func(*Provider)

func New(opts ...Option) *Provider {
	p := &Provider{
		binary: "lightpanda",
		host:   "127.0.0.1",
		port:   9222,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}

	if p.retryDelay == 0 {
		p.retryDelay = defaultRetryDelay
	}
	if p.startProcess == nil {
		p.startProcess = p.startProcessDefault
	}
	if p.connectBrowser == nil {
		p.connectBrowser = p.connectBrowserDefault
	}
	if p.dialAddress == nil {
		p.dialAddress = dialAddress
	}

	return p
}

func WithBinary(path string) Option {
	return func(p *Provider) {
		p.binary = path
	}
}

func WithHost(host string) Option {
	return func(p *Provider) {
		p.host = host
	}
}

func WithPort(port int) Option {
	return func(p *Provider) {
		p.port = port
	}
}

func WithArgs(args ...string) Option {
	return func(p *Provider) {
		p.args = append([]string(nil), args...)
	}
}

func (p *Provider) Launch(ctx context.Context) (*rod.Browser, error) {
	p.mu.Lock()
	if p.launched {
		p.mu.Unlock()
		return nil, errors.New("lightpandarod: launch already attempted")
	}
	p.launched = true
	p.mu.Unlock()

	if err := ensureAddressAvailable(ctx, p.host, p.port); err != nil {
		return nil, err
	}

	proc, err := p.startProcess(ctx, p.processConfig())
	if err != nil {
		return nil, err
	}

	browser, browserCloser, err := p.connectUntilReady(ctx)
	if err != nil {
		return nil, errors.Join(err, closeProcess(proc))
	}

	p.mu.Lock()
	p.process = proc
	p.browser = browser
	p.browserCloser = browserCloser
	p.mu.Unlock()

	return browser, nil
}

func (p *Provider) MustLaunch(ctx context.Context) *rod.Browser {
	browser, err := p.Launch(ctx)
	if err != nil {
		panic(err)
	}

	return browser
}

func (p *Provider) Close() error {
	p.mu.Lock()
	browserCloser := p.browserCloser
	process := p.process
	p.browser = nil
	p.browserCloser = nil
	p.process = nil
	p.mu.Unlock()

	var errs []error

	if browserCloser != nil {
		if err := browserCloser.Close(); !isIgnorableCloseError(err) {
			errs = append(errs, err)
		}
	}

	if process != nil {
		if err := process.Close(); !isIgnorableCloseError(err) {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (p *Provider) processConfig() processConfig {
	return processConfig{
		binary: p.binary,
		host:   p.host,
		port:   p.port,
		args:   append([]string(nil), p.args...),
	}
}

func (p *Provider) connectConfig() connectConfig {
	return connectConfig{
		host: p.host,
		port: p.port,
	}
}

func (p *Provider) connectUntilReady(ctx context.Context) (*rod.Browser, resourceCloser, error) {
	var lastErr error

	for {
		browser, closer, err := p.connectBrowser(ctx, p.connectConfig())
		if err == nil {
			return browser, closer, nil
		}

		lastErr = err

		select {
		case <-ctx.Done():
			return nil, nil, fmt.Errorf("lightpandarod: launch failed: %w", errors.Join(lastErr, ctx.Err()))
		default:
		}

		if err := p.dialAddress(ctx, net.JoinHostPort(p.host, fmt.Sprintf("%d", p.port))); err == nil {
			continue
		}

		timer := time.NewTimer(p.retryDelay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, nil, fmt.Errorf("lightpandarod: launch failed: %w", errors.Join(lastErr, ctx.Err()))
		case <-timer.C:
		}
	}
}

type resourceCloser interface {
	Close() error
}

func isIgnorableCloseError(err error) bool {
	if err == nil {
		return true
	}

	return errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) || errors.Is(err, context.Canceled)
}

func closeProcess(proc processHandle) error {
	if proc == nil {
		return nil
	}

	err := proc.Close()
	if isIgnorableCloseError(err) {
		return nil
	}

	return err
}
