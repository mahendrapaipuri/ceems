package frontend

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
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

// AllowRetry checks if a failed request can be retried
func AllowRetry(r *http.Request) bool {
	if _, ok := r.Context().Value(RetryContextKey{}).(bool); ok {
		return false
	}
	return true
}

// Monitor checks the backend servers health
func Monitor(ctx context.Context, manager serverpool.Manager, logger log.Logger) {
	t := time.NewTicker(time.Second * 20)
	level.Info(logger).Log("msg", "Starting health checker")
	for {
		// This will ensure that we will run the method as soon as go routine
		// starts instead of waiting for ticker to tick
		go healthCheck(ctx, manager, logger)

		select {
		case <-t.C:
			continue
		case <-ctx.Done():
			level.Info(logger).Log("msg", "Received Interrupt. Stopping health checker")
			return
		}
	}
}

// // Returns query period based on start time of query
// func parseQueryPeriod(r *http.Request) time.Duration {
// 	// Parse start query string in request
// 	start, err := parseTimeParam(r, "start", MinTime)
// 	if err != nil {
// 		return time.Duration(0 * time.Second)
// 	}
// 	return time.Now().UTC().Sub(start)
// }

// Parse time parameter in request
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

// Convert time parameter string into time.Time
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

// healthCheck monitors the status of all backend servers
func healthCheck(ctx context.Context, manager serverpool.Manager, logger log.Logger) {
	aliveChannel := make(chan bool, 1)

	for _, backend := range manager.Backends() {
		requestCtx, stop := context.WithTimeout(ctx, 10*time.Second)
		defer stop()
		status := "up"
		go isAlive(requestCtx, aliveChannel, backend.URL(), logger)

		select {
		case <-ctx.Done():
			level.Info(logger).Log("msg", "Gracefully shutting down health check")
			return
		case alive := <-aliveChannel:
			backend.SetAlive(alive)
			if !alive {
				status = "down"
			}
		}
		level.Debug(logger).Log("msg", "Health check", "url", backend.URL().String(), "status", status)
	}
}

// isAlive returns the status of backend server with a channel
func isAlive(ctx context.Context, aliveChannel chan bool, u *url.URL, logger log.Logger) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", u.Host)
	if err != nil {
		level.Debug(logger).Log("msg", "Backend unreachable", "backend", u.String(), "err", err)
		aliveChannel <- false
		return
	}
	_ = conn.Close()
	aliveChannel <- true
}
