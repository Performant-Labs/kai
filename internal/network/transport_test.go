package network

import (
	"testing"

	"cnb.cool/dtapp/kai/internal/settings"
)

// Issue #109 (criterion 1): the shared HTTP client has no total or read deadline, so a slow but
// alive response is never cut; the connect and handshake limits stay.
func TestBuildHTTPClientHasNoTotalOrReadDeadline(t *testing.T) {
	c := BuildHTTPClient(settings.Settings{})
	if c.Timeout != 0 {
		t.Errorf("client Timeout = %v, want 0 (no total deadline)", c.Timeout)
	}
	tr, ok := unwrapHTTPTransport(c.Transport)
	if !ok {
		t.Fatalf("could not unwrap the transport from %T", c.Transport)
	}
	if tr.ResponseHeaderTimeout != 0 {
		t.Errorf("ResponseHeaderTimeout = %v, want 0 (a non-streaming completion sends no headers until done)", tr.ResponseHeaderTimeout)
	}
	if tr.TLSHandshakeTimeout <= 0 {
		t.Errorf("TLSHandshakeTimeout = %v, want it kept (connect-phase limit)", tr.TLSHandshakeTimeout)
	}
}
