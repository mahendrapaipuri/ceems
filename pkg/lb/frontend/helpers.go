package frontend

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/lb/serverpool"
)

// These functions are nicked from https://github.com/prometheus/prometheus/blob/main/web/api/v1/api.go
var (
	// MinTime is the default timestamp used for the begin of optional time ranges.
	// Exposed to let downstream projects to reference it.
	MinTime = time.Unix(math.MinInt64/1000+62135596801, 0).UTC()

	// MaxTime is the default timestamp used for the end of optional time ranges.
	// Exposed to let downstream projects to reference it.
	MaxTime = time.Unix(math.MaxInt64/1000-62135596801, 999999999).UTC()

	minTimeFormatted = MinTime.Format(time.RFC3339Nano)
	maxTimeFormatted = MaxTime.Format(time.RFC3339Nano)
)

// AllowRetry checks if a failed request can be retried.
func AllowRetry(r *http.Request) bool {
	if _, ok := r.Context().Value(RetryContextKey{}).(bool); ok {
		return false
	}

	return true
}

// Monitor checks the backend servers health.
func Monitor(ctx context.Context, manager serverpool.Manager, logger *slog.Logger) {
	t := time.NewTicker(time.Second * 20)

	logger.Info("Starting health checker")

	for {
		// This will ensure that we will run the method as soon as go routine
		// starts instead of waiting for ticker to tick
		go healthCheck(ctx, manager, logger)

		select {
		case <-t.C:
			continue
		case <-ctx.Done():
			logger.Info("Received Interrupt. Stopping health checker")

			return
		}
	}
}

// Set query params into request's context and return new request.
func setQueryParams(r *http.Request, queryParams *QueryParams) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), QueryParamsContextKey{}, queryParams))
}

// Parse query in the request after cloning it and add query params to context.
func parseQueryParams(r *http.Request, logger *slog.Logger) *http.Request {
	var body []byte

	var clusterID string

	var uuids []string

	var queryPeriod time.Duration

	var err error

	// Get cluster id from X-Ceems-Cluster-Id header
	clusterID = r.Header.Get(ceemsClusterIDHeader)

	// // Get id from path parameter.
	// // Requested paths will be of form /{id}/<rest of path>. Here will strip `id`
	// // part and proxy the rest to backend
	// var pathParts []string

	// for _, p := range strings.Split(r.URL.Path, "/") {
	// 	if strings.TrimSpace(p) == "" {
	// 		continue
	// 	}

	// 	pathParts = append(pathParts, p)
	// }

	// // First path part must be resource manager ID and check if it is in the valid IDs
	// if len(pathParts) > 0 {
	// 	if slices.Contains(rmIDs, pathParts[0]) {
	// 		id = pathParts[0]

	// 		// If there is more than 1 pathParts, make URL or set / as URL
	// 		if len(pathParts) > 1 {
	// 			r.URL.Path = "/" + strings.Join(pathParts[1:], "/")
	// 			r.RequestURI = r.URL.Path
	// 		} else {
	// 			r.URL.Path = "/"
	// 			r.RequestURI = "/"
	// 		}
	// 	}
	// }

	// Make a new request and add newReader to that request body
	clonedReq := r.Clone(r.Context())

	// If request has no body go to proxy directly
	if r.Body == nil {
		return setQueryParams(r, &QueryParams{clusterID, uuids, queryPeriod})
	}

	// If failed to read body, skip verification and go to request proxy
	if body, err = io.ReadAll(r.Body); err != nil {
		logger.Error("Failed to read request body", "err", err)

		return setQueryParams(r, &QueryParams{clusterID, uuids, queryPeriod})
	}

	// clone body to existing request and new request
	r.Body = io.NopCloser(bytes.NewReader(body))
	clonedReq.Body = io.NopCloser(bytes.NewReader(body))

	// Get form values
	if err = clonedReq.ParseForm(); err != nil {
		logger.Error("Could not parse request body", "err", err)

		return setQueryParams(r, &QueryParams{clusterID, uuids, queryPeriod})
	}

	// Parse TSDB's query in request query params
	if val := clonedReq.FormValue("query"); val != "" {
		// Extract UUIDs from query
		uuidMatches := regexpUUID.FindAllStringSubmatch(val, -1)
		for _, match := range uuidMatches {
			if len(match) > 1 {
				for _, uuid := range strings.Split(match[1], "|") {
					// Ignore empty strings
					if strings.TrimSpace(uuid) != "" && !slices.Contains(uuids, uuid) {
						uuids = append(uuids, uuid)
					}
				}
			}
		}

		// Extract ceems_lb_id from query. If multiple values are provided, always
		// get the last and most recent one
		idMatches := regexID.FindAllStringSubmatch(val, -1)
		for _, match := range idMatches {
			if len(match) > 1 {
				for _, idMatch := range strings.Split(match[1], "|") {
					// Ignore empty strings
					if strings.TrimSpace(idMatch) != "" {
						clusterID = strings.TrimSpace(idMatch)
					}
				}
			}
		}
	}

	// Except for query API, rest of the load balanced API endpoint have start query param
	var targetQueryParam string
	if strings.HasSuffix(clonedReq.URL.Path, "query") {
		targetQueryParam = "time"
	} else {
		targetQueryParam = "start"
	}

	// Parse TSDB's start query in request query params
	if startTime, err := parseTimeParam(clonedReq, targetQueryParam, time.Now().UTC()); err != nil {
		logger.Error("Could not parse start query param", "err", err)

		queryPeriod = 0 * time.Second
	} else {
		queryPeriod = time.Now().UTC().Sub(startTime)
	}

	// Set query params to request's context
	return setQueryParams(r, &QueryParams{clusterID, uuids, queryPeriod})
}

// Parse time parameter in request.
func parseTimeParam(r *http.Request, paramName string, defaultValue time.Time) (time.Time, error) {
	val := r.FormValue(paramName)
	if val == "" {
		return defaultValue, nil
	}

	result, err := parseTime(val)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid time value for '%s': %w", paramName, err)
	}

	return result, nil
}

// Convert time parameter string into time.Time.
func parseTime(s string) (time.Time, error) {
	if t, err := strconv.ParseFloat(s, 64); err == nil {
		s, ns := math.Modf(t)
		ns = math.Round(ns*1000) / 1000

		return time.Unix(int64(s), int64(ns*float64(time.Second))).UTC(), nil
	}

	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}

	// Stdlib's time parser can only handle 4 digit years. As a workaround until
	// that is fixed we want to at least support our own boundary times.
	// Context: https://github.com/prometheus/client_golang/issues/614
	// Upstream issue: https://github.com/golang/go/issues/20555
	switch s {
	case minTimeFormatted:
		return MinTime, nil
	case maxTimeFormatted:
		return MaxTime, nil
	}

	return time.Time{}, fmt.Errorf("cannot parse %q to a valid timestamp", s)
}

// healthCheck monitors the status of all backend servers.
func healthCheck(ctx context.Context, manager serverpool.Manager, logger *slog.Logger) {
	aliveChannel := make(chan bool, 1)

	for id, backends := range manager.Backends() {
		for _, backend := range backends {
			requestCtx, stop := context.WithTimeout(ctx, 10*time.Second)
			defer stop()

			status := "up"

			go isAlive(requestCtx, aliveChannel, backend.URL(), logger)

			select {
			case <-ctx.Done():
				logger.Info("Gracefully shutting down health check")

				return
			case alive := <-aliveChannel:
				backend.SetAlive(alive)

				if !alive {
					status = "down"
				}
			}
			logger.Debug("Health check", "id", id, "url", backend.URL().Redacted(), "status", status)
		}
	}
}

// isAlive returns the status of backend server with a channel.
func isAlive(ctx context.Context, aliveChannel chan bool, u *url.URL, logger *slog.Logger) {
	var d net.Dialer

	conn, err := d.DialContext(ctx, "tcp", u.Host)
	if err != nil {
		logger.Debug("Backend unreachable", "backend", u.Redacted(), "err", err)
		aliveChannel <- false

		return
	}

	_ = conn.Close()
	aliveChannel <- true
}
