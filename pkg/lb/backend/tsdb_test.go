package backend

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/tsdb"
	"github.com/stretchr/testify/require"
)

const (
	testURL          = "http://localhost:3333"
	testURLBasicAuth = "http://foo:bar@localhost:3333" // #nosec
)

func TestTSDBConfigSuccess(t *testing.T) {
	// Start test server
	expectedRuntime := tsdb.Response[any]{
		Status: "success",
		Data: map[string]string{
			"storageRetention": "30d",
		},
	}
	expectedRange := tsdb.Response[any]{
		Status: "success",
		Data: map[string]interface{}{
			"resultType": "matrix",
			"result": []interface{}{
				map[string]interface{}{
					"metric": map[string]string{
						"__name__": "up",
						"instance": "localhost:9090",
					},
					"values": []interface{}{
						[]interface{}{time.Now().Add(-15 * 24 * time.Hour).Unix(), "1"},
						[]interface{}{time.Now().Add(-15 * 23 * time.Hour).Unix(), "1"},
					},
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "runtimeinfo") {
			if err := json.NewEncoder(w).Encode(&expectedRuntime); err != nil {
				w.Write([]byte("KO"))
			}
		} else {
			if err := json.NewEncoder(w).Encode(&expectedRange); err != nil {
				w.Write([]byte("KO"))
			}
		}
	}))
	// defer server.Close()

	url, _ := url.Parse(server.URL)
	b := NewTSDB(url, httputil.NewSingleHostReverseProxy(url), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.Equal(t, server.URL, b.URL().String())
	require.Equal(t, 720*time.Hour, b.RetentionPeriod())
	require.True(t, b.IsAlive())
	require.Equal(t, 0, b.ActiveConnections())

	// Stop dummy server and query for retention period, we should get last updated value
	server.Close()
	require.Equal(t, 720*time.Hour, b.RetentionPeriod())
}

func TestTSDBConfigSuccessWithTwoRetentions(t *testing.T) {
	// Start test server
	expectedRuntime := tsdb.Response[any]{
		Status: "success",
		Data: map[string]string{
			"storageRetention": "30d or 10GiB",
		},
	}

	expectedRange := tsdb.Response[any]{
		Status: "success",
		Data: map[string]interface{}{
			"resultType": "matrix",
			"result": []interface{}{
				map[string]interface{}{
					"metric": map[string]string{
						"__name__": "up",
						"instance": "localhost:9090",
					},
					"values": []interface{}{
						[]interface{}{time.Now().Add(-30 * 24 * time.Hour).Unix(), "1"},
						[]interface{}{time.Now().Add(-30 * 23 * time.Hour).Unix(), "1"},
					},
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "runtimeinfo") {
			if err := json.NewEncoder(w).Encode(&expectedRuntime); err != nil {
				w.Write([]byte("KO"))
			}
		} else {
			if err := json.NewEncoder(w).Encode(&expectedRange); err != nil {
				w.Write([]byte("KO"))
			}
		}
	}))
	defer server.Close()

	url, _ := url.Parse(server.URL)
	b := NewTSDB(url, httputil.NewSingleHostReverseProxy(url), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.Equal(t, server.URL, b.URL().String())
	require.Equal(t, 714*time.Hour, b.RetentionPeriod())
	require.True(t, b.IsAlive())
}

func TestTSDBConfigSuccessWithRetentionSize(t *testing.T) {
	// Start test server
	expectedRuntime := tsdb.Response[any]{
		Status: "success",
		Data: map[string]string{
			"storageRetention": "10GiB",
		},
	}

	expectedRange := tsdb.Response[any]{
		Status: "success",
		Data: map[string]interface{}{
			"resultType": "matrix",
			"result": []interface{}{
				map[string]interface{}{
					"metric": map[string]string{
						"__name__": "up",
						"instance": "localhost:9090",
					},
					"values": []interface{}{
						[]interface{}{time.Now().Add(-30 * 24 * time.Hour).Unix(), "1"},
						[]interface{}{time.Now().Add(-30 * 23 * time.Hour).Unix(), "1"},
					},
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "runtimeinfo") {
			if err := json.NewEncoder(w).Encode(&expectedRuntime); err != nil {
				w.Write([]byte("KO"))
			}
		} else {
			if err := json.NewEncoder(w).Encode(&expectedRange); err != nil {
				w.Write([]byte("KO"))
			}
		}
	}))
	defer server.Close()

	url, _ := url.Parse(server.URL)
	b := NewTSDB(url, httputil.NewSingleHostReverseProxy(url), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.Equal(t, server.URL, b.URL().String())
	require.Equal(t, 714*time.Hour, b.RetentionPeriod())
	require.True(t, b.IsAlive())
}

func TestTSDBConfigFail(t *testing.T) {
	// Start test server
	expected := "dummy"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	url, _ := url.Parse(server.URL)
	b := NewTSDB(url, httputil.NewSingleHostReverseProxy(url), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.Equal(t, server.URL, b.URL().String())
	require.Equal(t, 0*time.Hour, b.RetentionPeriod())
	require.True(t, b.IsAlive())
}

func TestTSDBBackendAlive(t *testing.T) {
	url, _ := url.Parse(testURL)
	b := NewTSDB(url, httputil.NewSingleHostReverseProxy(url), slog.New(slog.NewTextHandler(io.Discard, nil)))
	b.SetAlive(b.IsAlive())

	require.True(t, b.IsAlive())
}

func TestTSDBBackendAliveWithBasicAuth(t *testing.T) {
	url, _ := url.Parse(testURLBasicAuth)
	b := NewTSDB(url, httputil.NewSingleHostReverseProxy(url), slog.New(slog.NewTextHandler(io.Discard, nil)))
	b.SetAlive(b.IsAlive())

	require.True(t, b.IsAlive())
}
