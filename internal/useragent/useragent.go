// Package useragent maintains the global User-Agent (passed by the frontend at startup from
// the WebView's navigator.userAgent) and provides a UA-injecting RoundTripper: requests
// without an explicit User-Agent automatically get the global one.
// Requests that already set a UA (e.g. cloud SDKs' own UA, monitor/scanner's CertFlow/1.0)
// are untouched.
package useragent

import (
	"net/http"
	"sync"
)

var (
	mu sync.RWMutex
	ua string
)

// Set sets the global User-Agent (passed by the frontend at app startup via
// MonitorService.SetUserAgent).
func Set(v string) {
	mu.Lock()
	ua = v
	mu.Unlock()
}

// Get returns the current global User-Agent; an empty string when unset (nothing injected,
// Go's default UA applies).
func Get() string {
	mu.RLock()
	defer mu.RUnlock()
	return ua
}

// Transport is a RoundTripper injecting the global UA when a request has no explicit
// User-Agent.
type Transport struct {
	Base http.RoundTripper
}

// RoundTrip implements http.RoundTripper. Per convention it never mutates the original
// request; on injection it Clones one.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	if req.Header.Get("User-Agent") == "" {
		if v := Get(); v != "" {
			req = req.Clone(req.Context())
			req.Header.Set("User-Agent", v)
		}
	}
	return base.RoundTrip(req)
}

// Wrap wraps base into a RoundTripper injecting the global UA; if base is already a
// *Transport it is returned as-is to avoid double wrapping
// (double wrapping would be harmless anyway: the inner layer sees UA set and won't
// override).
func Wrap(base http.RoundTripper) http.RoundTripper {
	if t, ok := base.(*Transport); ok {
		return t
	}
	return &Transport{Base: base}
}
