//go:build cgo
// +build cgo

package frontend

import (
	"bytes"
	"context"
	"errors"
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

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/mahendrapaipuri/ceems/pkg/lb/backend"
	"github.com/mahendrapaipuri/ceems/pkg/lb/serverpool"
	"google.golang.org/protobuf/proto"
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

// ErrorHandler returns a custom error handler for reverse proxy.
func ErrorHandler(u *url.URL, backendServer backend.Server, lb LoadBalancer, logger *slog.Logger) func(http.ResponseWriter, *http.Request, error) {
	return func(writer http.ResponseWriter, request *http.Request, err error) {
		logger.Error("Failed to handle the request", "host", u.Host, "err", err)
		backendServer.SetAlive(false)

		// If already retried the request, return error
		if !AllowRetry(request) {
			logger.Info("Max retry attempts reached, terminating", "address", request.RemoteAddr, "path", request.URL.Path)
			http.Error(writer, "Service not available", http.StatusServiceUnavailable)

			return
		}

		// Retry request and set context value so that we dont retry for second time
		logger.Info("Attempting retry", "address", request.RemoteAddr, "path", request.URL.Path)
		lb.Serve(
			writer,
			request.WithContext(
				context.WithValue(request.Context(), RetryContextKey{}, true),
			),
		)
	}
}

// Set query params into request's context and return new request.
func setQueryParams(r *http.Request, queryParams *ReqParams) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), ReqParamsContextKey{}, queryParams))
}

// parseTSDBRequest parses TSDB query in the request after cloning it and reads them into request params.
func parseTSDBRequest(p *ReqParams, r *http.Request) error {
	var body []byte

	var err error

	// Make a new request and add newReader to that request body
	clonedReq := r.Clone(r.Context())

	// If request has no body go to proxy directly
	if r.Body == nil {
		return errors.New("no body found in the request")
	}

	// If failed to read body, skip verification and go to request proxy
	if body, err = io.ReadAll(r.Body); err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}

	// clone body to existing request and new request
	r.Body = io.NopCloser(bytes.NewReader(body))
	clonedReq.Body = io.NopCloser(bytes.NewReader(body))

	// Get form values
	if err = clonedReq.ParseForm(); err != nil {
		return fmt.Errorf("failed to parse request form data: %w", err)
	}

	// Except for query API, rest of the load balanced API endpoint have start query param
	var targetTimeParam, targetQueryParam string

	switch {
	case strings.HasSuffix(clonedReq.URL.Path, "query"):
		targetQueryParam = "query"
		targetTimeParam = "time"
	case strings.HasSuffix(clonedReq.URL.Path, "query_range"):
		targetQueryParam = "query"
		targetTimeParam = "start"
	default:
		targetQueryParam = "match[]"
		targetTimeParam = "start"
	}

	// Parse TSDB's query in request query params
	if val := clonedReq.FormValue(targetQueryParam); val != "" {
		parseReqParams(p, val)
	}

	// Parse TSDB's start query in request query params
	if startTime, err := parseTimeParam(clonedReq, targetTimeParam, time.Now().Local()); err != nil {
		p.queryPeriod = 0 * time.Second
		p.time = time.Now().Local().UnixMilli()
	} else {
		p.queryPeriod = time.Now().Local().Sub(startTime)
		p.time = startTime.Local().UnixMilli()
	}

	return nil
}

// parsePyroRequest parses Pyroscope query in the request after cloning it and reads them into request params.
func parsePyroRequest(p *ReqParams, r *http.Request) error {
	var body []byte

	var err error

	// If request has no body go to proxy directly
	if r.Body == nil {
		return errors.New("no body found in the request")
	}

	// If failed to read body, skip verification and go to request proxy
	if body, err = io.ReadAll(r.Body); err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}

	// clone body to existing request
	r.Body = io.NopCloser(bytes.NewReader(body))

	// Read body into request data
	data := querierv1.SelectMergeStacktracesRequest{}
	if err := proto.Unmarshal(body, &data); err != nil {
		return fmt.Errorf("failed to umarshall request body: %w", err)
	}

	// Parse Pyroscope's LabelSelector in request data
	if val := data.GetLabelSelector(); val != "" {
		parseReqParams(p, val)
	}

	// Parse Pyroscope's start query in request query params
	if start := data.GetStart(); start == 0 {
		p.queryPeriod = 0 * time.Second
		p.time = time.Now().Local().UnixMilli()
	} else {
		startTime := time.Unix(start, 0)
		p.queryPeriod = time.Now().Local().Sub(startTime)
		p.time = startTime.Local().UnixMilli()
	}

	return nil
}

// parseRequestParams parses request parameters from `req` and reads them into `p`.
func parseReqParams(p *ReqParams, req string) {
	// Extract UUIDs from query
	for _, match := range regexpUUID.FindAllStringSubmatch(req, -1) {
		if len(match) > 1 {
			for _, uuid := range strings.Split(match[1], "|") {
				// Ignore empty strings
				if strings.TrimSpace(uuid) != "" && !slices.Contains(p.uuids, uuid) {
					p.uuids = append(p.uuids, uuid)
				}
			}
		}
	}

	// Extract ceems_id from query. If multiple values are provided, always
	// get the last and most recent one
	idMatches := regexID.FindAllStringSubmatch(req, -1)
	for _, match := range idMatches {
		if len(match) > 1 {
			for _, idMatch := range strings.Split(match[1], "|") {
				// Ignore empty strings
				if strings.TrimSpace(idMatch) != "" {
					p.clusterID = strings.TrimSpace(idMatch)
				}
			}
		}
	}
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
