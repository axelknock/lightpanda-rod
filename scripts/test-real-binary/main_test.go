package main

import "testing"

func TestNightlyDownloadURL(t *testing.T) {
	tests := []struct {
		goos   string
		goarch string
		want   string
	}{
		{
			goos:   "linux",
			goarch: "amd64",
			want:   "https://github.com/lightpanda-io/browser/releases/download/nightly/lightpanda-x86_64-linux",
		},
		{
			goos:   "darwin",
			goarch: "arm64",
			want:   "https://github.com/lightpanda-io/browser/releases/download/nightly/lightpanda-aarch64-macos",
		},
	}

	for _, tc := range tests {
		got, err := nightlyDownloadURL(tc.goos, tc.goarch)
		if err != nil {
			t.Fatalf("nightlyDownloadURL(%q, %q) returned error: %v", tc.goos, tc.goarch, err)
		}
		if got != tc.want {
			t.Fatalf("nightlyDownloadURL(%q, %q) = %q, want %q", tc.goos, tc.goarch, got, tc.want)
		}
	}
}

func TestNightlyDownloadURLRejectsUnsupportedPlatforms(t *testing.T) {
	if _, err := nightlyDownloadURL("darwin", "amd64"); err == nil {
		t.Fatal("nightlyDownloadURL returned nil error for unsupported platform")
	}
}

func TestProbeHTMLContainsExpectedTitle(t *testing.T) {
	html, wantTitle := probeHTML()

	if wantTitle == "" {
		t.Fatal("probeHTML returned empty expected title")
	}

	wantDocument := "<title>" + wantTitle + "</title>"
	if html != "<!doctype html><html><head>"+wantDocument+"</head><body>lightpanda rod integration</body></html>" {
		t.Fatalf("probeHTML returned unexpected HTML: %q", html)
	}
}

func TestProbeTargetURLIsAboutBlank(t *testing.T) {
	if got := probeTargetURL(); got != "about:blank" {
		t.Fatalf("probeTargetURL() = %q, want %q", got, "about:blank")
	}
}
