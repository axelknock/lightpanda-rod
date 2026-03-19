package lightpandarod

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"
)

type processConfig struct {
	binary string
	host   string
	port   int
	args   []string
}

func (cfg processConfig) address() string {
	return net.JoinHostPort(cfg.host, strconv.Itoa(cfg.port))
}

func (cfg processConfig) commandArgs() []string {
	args := []string{"serve", "--host", cfg.host, "--port", strconv.Itoa(cfg.port)}
	return append(args, cfg.args...)
}

type processHandle interface {
	Close() error
}

type execProcessHandle struct {
	cmd    *exec.Cmd
	waitCh chan error
	once   sync.Once
}

func (p *Provider) startProcessDefault(ctx context.Context, cfg processConfig) (processHandle, error) {
	cmd := exec.CommandContext(ctx, cfg.binary, cfg.commandArgs()...)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("lightpandarod: start process: %w", err)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	return &execProcessHandle{
		cmd:    cmd,
		waitCh: waitCh,
	}, nil
}

func (h *execProcessHandle) Close() error {
	if h == nil || h.cmd == nil {
		return nil
	}

	var err error
	h.once.Do(func() {
		if h.cmd.Process != nil {
			_ = h.cmd.Process.Signal(syscall.SIGTERM)
			time.Sleep(10 * time.Millisecond)
			_ = h.cmd.Process.Kill()
		}

		err = <-h.waitCh
	})

	if isIgnorableCloseError(err) {
		return nil
	}

	return err
}

func ensureAddressAvailable(ctx context.Context, host string, port int) error {
	address := net.JoinHostPort(host, strconv.Itoa(port))
	if err := dialAddress(ctx, address); err == nil {
		return fmt.Errorf("lightpandarod: address already in use: %s", address)
	}

	return nil
}

func dialAddress(ctx context.Context, address string) error {
	dialer := &net.Dialer{Timeout: 50 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err == nil {
		_ = conn.Close()
		return nil
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ctx.Err()
	}

	return err
}
