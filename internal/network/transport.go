package network

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"cnb.cool/dtapp/kai/internal/httplogstore"
	"cnb.cool/dtapp/kai/internal/settings"
	"cnb.cool/dtapp/kai/internal/useragent"
)

// unwrapHTTPTransport unwraps, layer by layer, a RoundTripper wrapped by useragent.Wrap
// (with httplog.WrapTransport outermost), recovering the underlying *http.Transport.
// BuildHTTPClient uses this to set the proxy outside the wrapping layers; tests also use it
// to assert the underlying Transport type. Only the single *useragent.Transport layer is
// unwrapped (enough for tests / non-DEBUG); the multi-wrapped LoggingRoundTripper under
// DEBUG is not handled here.
func unwrapHTTPTransport(rt http.RoundTripper) (*http.Transport, bool) {
	for rt != nil {
		if t, ok := rt.(*http.Transport); ok {
			return t, true
		}
		if t, ok := rt.(*useragent.Transport); ok {
			rt = t.Base
			continue
		}
		return nil, false
	}
	return nil, false
}

// BuildHTTPClient builds an HTTP client with custom DNS and proxy per settings
func BuildHTTPClient(s settings.Settings) *http.Client {
	transport := httplogstore.WrapTransport(&http.Transport{
		// Custom DNS resolution
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}

			// Use custom DNS resolution
			ips, err := resolveHost(s, host)
			if err != nil || len(ips) == 0 {
				// Fall back to the system default
				d := net.Dialer{Timeout: 10 * time.Second}
				return d.DialContext(ctx, network, addr)
			}

			d := net.Dialer{Timeout: 10 * time.Second}
			return d.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		},
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	})

	// Configure proxy
	if s.Proxy.Enabled && s.Proxy.Host != "" {
		proxyURL := buildProxyURL(s.Proxy)
		if proxyURL != nil {
			// WrapTransport returns a RoundTripper wrapped by useragent.Transport, so unwrap
			// first to reach the underlying *http.Transport before setting the proxy (a direct
			// *http.Transport assertion would always fail).
			if t, ok := unwrapHTTPTransport(transport); ok {
				t.Proxy = http.ProxyURL(proxyURL)
			}
		}
	}

	return &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second,
	}
}

// resolveHost resolves a hostname using the settings-enabled DNS servers
func resolveHost(s settings.Settings, host string) ([]net.IP, error) {
	var servers []string
	for _, dns := range s.DNSConfigs {
		if dns.Enabled && len(dns.Servers) > 0 {
			servers = append(servers, dns.Servers...)
		}
	}

	if len(servers) == 0 {
		// No custom DNS enabled; use the system default
		return net.DefaultResolver.LookupIP(context.Background(), "ip4", host)
	}

	// Use the first enabled DNS server
	for _, server := range servers {
		ips, err := queryDNSServer(server, host)
		if err == nil && len(ips) > 0 {
			return ips, nil
		}
	}

	// All failed; fall back to the system default
	return net.DefaultResolver.LookupIP(context.Background(), "ip4", host)
}

// queryDNSServer queries an A record from the given DNS server
func queryDNSServer(server, host string) ([]net.IP, error) {
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, "udp", net.JoinHostPort(server, "53"))
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return resolver.LookupIP(ctx, "ip4", host)
}

// buildProxyURL builds the proxy URL from the proxy config
func buildProxyURL(proxy settings.ProxyConfig) *url.URL {
	host := fmt.Sprintf("%s:%d", proxy.Host, proxy.Port)
	if proxy.Username != "" {
		return &url.URL{
			Scheme: proxy.Protocol,
			User:   url.UserPassword(proxy.Username, proxy.Password),
			Host:   host,
		}
	}
	return &url.URL{
		Scheme: proxy.Protocol,
		Host:   host,
	}
}
