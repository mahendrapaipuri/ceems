package backend

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/mahendrapaipuri/ceems/pkg/tsdb"
)

const (
	testURL          = "http://localhost:3333"
	testURLBasicAuth = "http://foo:bar@localhost:3333" // #nosec
)

func TestTSDBConfigSuccess(t *testing.T) {
	// Start test server
	expected := tsdb.Response{
		Status: "success",
		Data: map[string]string{
			"storageRetention": "30d",
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	// defer server.Close()

	url, _ := url.Parse(server.URL)
	b := NewTSDBServer(url, httputil.NewSingleHostReverseProxy(url), log.NewNopLogger())

	if b.URL().String() != server.URL {
		t.Errorf("expected URL %s, got %s", server.URL, b.URL().String())
	}
	if b.RetentionPeriod() != time.Duration(30*24*time.Hour) {
		t.Errorf("expected retention period 30d, got %s", b.RetentionPeriod())
	}
	if !b.IsAlive() {
		t.Errorf("expected backend to be alive")
	}
	if b.ActiveConnections() != 0 {
		t.Errorf("expected zero active connections to backend")
	}

	// Stop dummy server and query for retention period, we should get last updated value
	server.Close()
	if b.RetentionPeriod() != time.Duration(30*24*time.Hour) {
		t.Errorf("expected retention period 30d, got %s", b.RetentionPeriod())
	}
}

func TestTSDBConfigSuccessWithTwoRetentions(t *testing.T) {
	// Start test server
	expected := tsdb.Response{
		Status: "success",
		Data: map[string]string{
			"storageRetention": "30d or 10GiB",
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	url, _ := url.Parse(server.URL)
	b := NewTSDBServer(url, httputil.NewSingleHostReverseProxy(url), log.NewNopLogger())

	if b.URL().String() != server.URL {
		t.Errorf("expected URL %s, got %s", server.URL, b.URL().String())
	}
	if b.RetentionPeriod() != time.Duration(30*24*time.Hour) {
		t.Errorf("expected retention period 30d, got %s", b.RetentionPeriod())
	}
	if !b.IsAlive() {
		t.Errorf("expected backend to be alive")
	}
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
	b := NewTSDBServer(url, httputil.NewSingleHostReverseProxy(url), log.NewNopLogger())

	if b.URL().String() != server.URL {
		t.Errorf("expected URL %s, got %s", server.URL, b.URL().String())
	}
	if b.RetentionPeriod() != time.Duration(0*time.Hour) {
		t.Errorf("expected retention period 0s, got %s", b.RetentionPeriod())
	}
	if !b.IsAlive() {
		t.Errorf("expected backend to be alive")
	}
}

func TestTSDBBackendAlive(t *testing.T) {
	url, _ := url.Parse(testURL)
	b := NewTSDBServer(url, httputil.NewSingleHostReverseProxy(url), log.NewNopLogger())
	b.SetAlive(b.IsAlive())

	if !b.IsAlive() {
		t.Errorf("expected backend to be alive")
	}
}

func TestTSDBBackendAliveWithBasicAuth(t *testing.T) {
	url, _ := url.Parse(testURLBasicAuth)
	b := NewTSDBServer(url, httputil.NewSingleHostReverseProxy(url), log.NewNopLogger())
	b.SetAlive(b.IsAlive())

	if !b.IsAlive() {
		t.Errorf("expected backend to be alive")
	}
}
