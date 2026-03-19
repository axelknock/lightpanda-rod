package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"runtime"
	"time"

	lightpandarod "lightpanda-rod"

	"github.com/go-rod/rod/lib/proto"
)

const (
	linuxAMD64NightlyURL = "https://github.com/lightpanda-io/browser/releases/download/nightly/lightpanda-x86_64-linux"
	macOSARM64NightlyURL = "https://github.com/lightpanda-io/browser/releases/download/nightly/lightpanda-aarch64-macos"
)

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {
	binaryPath := flag.String("binary", "", "Path to the Lightpanda binary")
	host := flag.String("host", "127.0.0.1", "Host for Lightpanda to listen on")
	port := flag.Int("port", 0, "Port for Lightpanda to listen on; 0 selects a free port")
	timeout := flag.Duration("timeout", 30*time.Second, "Timeout for launch and browser verification")
	flag.Parse()

	if *binaryPath == "" {
		return errors.New("binary path is required")
	}

	selectedPort := *port
	if selectedPort == 0 {
		freePort, err := reserveFreePort(*host)
		if err != nil {
			return err
		}
		selectedPort = freePort
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	provider := lightpandarod.New(
		lightpandarod.WithBinary(*binaryPath),
		lightpandarod.WithHost(*host),
		lightpandarod.WithPort(selectedPort),
	)
	defer func() {
		_ = provider.Close()
	}()

	browser, err := provider.Launch(ctx)
	if err != nil {
		return fmt.Errorf("launch provider: %w", err)
	}

	html, wantTitle := probeHTML()
	page, err := browser.Page(proto.TargetCreateTarget{
		URL: probeTargetURL(),
	})
	if err != nil {
		return fmt.Errorf("create probe page: %w", err)
	}

	if err := page.WaitLoad(); err != nil {
		return fmt.Errorf("wait for probe page load: %w", err)
	}

	if _, err := page.Eval(fmt.Sprintf(`() => {
		document.open();
		document.write(%q);
		document.close();
		return document.title;
	}`, html)); err != nil {
		return fmt.Errorf("set probe page content: %w", err)
	}

	info, err := page.Info()
	if err != nil {
		return fmt.Errorf("read probe page info: %w", err)
	}

	if info.Title != wantTitle {
		return fmt.Errorf("probe page title = %q, want %q", info.Title, wantTitle)
	}

	fmt.Printf("integration probe succeeded on %s:%d with %s\n", *host, selectedPort, *binaryPath)
	return nil
}

func nightlyDownloadURL(goos, goarch string) (string, error) {
	switch {
	case goos == "linux" && goarch == "amd64":
		return linuxAMD64NightlyURL, nil
	case goos == "darwin" && goarch == "arm64":
		return macOSARM64NightlyURL, nil
	default:
		return "", fmt.Errorf("unsupported platform: %s/%s", goos, goarch)
	}
}

func defaultNightlyDownloadURL() (string, error) {
	return nightlyDownloadURL(runtime.GOOS, runtime.GOARCH)
}

func probeHTML() (html string, expectedTitle string) {
	expectedTitle = "lightpanda rod integration ok"
	html = "<!doctype html><html><head><title>" + expectedTitle + "</title></head><body>lightpanda rod integration</body></html>"
	return html, expectedTitle
}

func probeTargetURL() string {
	return "about:blank"
}

func reserveFreePort(host string) (int, error) {
	listener, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		return 0, fmt.Errorf("reserve free port: %w", err)
	}
	defer listener.Close()

	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("reserve free port: unexpected address type %T", listener.Addr())
	}

	return tcpAddr.Port, nil
}
