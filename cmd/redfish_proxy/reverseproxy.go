package main

import (
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

const realIPHeaderName = "X-Real-IP"

// NewMultiHostReverseProxy returns a new instance of ReverseProxy that routes requests
// to multiple targets based on remote address of the request.
func NewMultiHostReverseProxy(logger *slog.Logger, targets map[string]*url.URL, tr *http.Transport) *httputil.ReverseProxy {
	director := func(req *http.Request) {
		rewriteRequestURL(logger, req, targets)
	}

	return &httputil.ReverseProxy{Director: director, Transport: tr}
}

// rewriteRequestURL rewrites the request URL to point to the target found based on
// remote IP addresses found using X-Real-IP header and request's remote address.
//
// We attempt to get the client IP address by multiple methods:
// - Check X-Real-IP header and get all IP addresses set for this header
// - Lookup RemoteAddr and split host IP address from socket address
//
// We merge all the IP addresses found from these two sources and attempt
// to find a target. We will use the first target match that is found from
// all these IP addresses.
//
// So IP addresses set in X-Real-IP has precedence over the request's
// RemoteAddr field.
func rewriteRequestURL(logger *slog.Logger, req *http.Request, targets map[string]*url.URL) {
	// Always use CanonicalHeaderKey as golang always canonicalize headers
	// internally
	remoteIPs := req.Header[http.CanonicalHeaderKey(realIPHeaderName)]
	// Split Host IP from RemoteAddr
	if hostIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		remoteIPs = append(remoteIPs, hostIP)
	}

	for _, ip := range remoteIPs {
		if target, ok := targets[ip]; ok {
			targetQuery := target.RawQuery
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.URL.Path, req.URL.RawPath = joinURLPath(target, req.URL)

			if targetQuery == "" || req.URL.RawQuery == "" {
				req.URL.RawQuery = targetQuery + req.URL.RawQuery
			} else {
				req.URL.RawQuery = targetQuery + "&" + req.URL.RawQuery
			}

			// Strip X-Real-IP header before proxying request to target
			req.Header.Del(realIPHeaderName)

			return
		}
	}

	// If no matches found, log the found remote IPs and return
	logger.Error("Failed to find target", "remote_ips", strings.Join(remoteIPs, ","))
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")

	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}

	return a + b
}

func joinURLPath(a, b *url.URL) (string, string) {
	if a.RawPath == "" && b.RawPath == "" {
		return singleJoiningSlash(a.Path, b.Path), ""
	}
	// Same as singleJoiningSlash, but uses EscapedPath to determine
	// whether a slash should be added
	apath := a.EscapedPath()
	bpath := b.EscapedPath()

	aslash := strings.HasSuffix(apath, "/")
	bslash := strings.HasPrefix(bpath, "/")

	switch {
	case aslash && bslash:
		return a.Path + b.Path[1:], apath + bpath[1:]
	case !aslash && !bslash:
		return a.Path + "/" + b.Path, apath + "/" + bpath
	}

	return a.Path + b.Path, apath + bpath
}
