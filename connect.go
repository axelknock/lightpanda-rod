package lightpandarod

import (
	"context"
	"errors"
	"net"
	"strconv"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/cdp"
)

type connectConfig struct {
	host string
	port int
}

func (cfg connectConfig) wsURL() string {
	return "ws://" + net.JoinHostPort(cfg.host, strconv.Itoa(cfg.port))
}

func (p *Provider) connectBrowserDefault(ctx context.Context, cfg connectConfig) (*rod.Browser, resourceCloser, error) {
	ws := &cdp.WebSocket{}
	if err := ws.Connect(ctx, cfg.wsURL(), nil); err != nil {
		return nil, nil, err
	}

	client := cdp.New().Start(ws)
	browser := rod.New().Client(client)
	if err := browser.Connect(); err != nil {
		_ = ws.Close()
		return nil, nil, err
	}

	return browser, resourceCloserFunc(func() error {
		var errs []error

		if err := browser.Close(); !isIgnorableCloseError(err) {
			errs = append(errs, err)
		}
		if err := ws.Close(); !isIgnorableCloseError(err) {
			errs = append(errs, err)
		}

		return errors.Join(errs...)
	}), nil
}

type resourceCloserFunc func() error

func (fn resourceCloserFunc) Close() error {
	return fn()
}
