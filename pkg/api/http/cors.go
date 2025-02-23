package http

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// Nicked from Prometheus
// https://github.com/prometheus/prometheus/blob/7bbbb5cb9701acb2225186f310f9d4ecc7f99752/util/httputil/cors.go#L30

// server struct.
var baseCorsHeaders = map[string]string{
	"Access-Control-Allow-Headers":  "Accept, Authorization, Content-Type, Origin",
	"Access-Control-Allow-Methods":  "GET",
	"Access-Control-Expose-Headers": "Date",
	"Vary":                          "Origin",
}

type cors struct {
	origin  *regexp.Regexp
	headers map[string]string
}

// newCORS returns a new instance of CORS struct.
func newCORS(origin *regexp.Regexp, userHeaders []string) *cors {
	// Setup CORS headers based on user headers names
	corsHeaders := baseCorsHeaders
	if len(userHeaders) > 0 {
		corsHeaders["Access-Control-Allow-Headers"] = fmt.Sprintf(
			"%s, %s", corsHeaders["Access-Control-Allow-Headers"], strings.Join(userHeaders, ", "),
		)
	}

	return &cors{origin: origin, headers: corsHeaders}
}

// wrapCORS wraps recource handlers around CORS handler.
func (c *cors) wrap(f func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		// Setup CORS headers
		c.set(w, r)

		// If it is a preflight request, return with OK status
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)

			return
		} else {
			f(w, r)
		}
	}
}

// setCORS enables cross-site script calls.
func (c *cors) set(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return
	}

	for k, v := range c.headers {
		w.Header().Set(k, v)
	}

	if c.origin.String() == "^(?:.*)$" {
		w.Header().Set("Access-Control-Allow-Origin", "*")

		return
	}

	if c.origin.MatchString(origin) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
}
