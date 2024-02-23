package backend

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/tsdb"
)

const testURL = "http://localhost:3333"

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
	defer server.Close()

	url, _ := url.Parse(server.URL)
	b := NewTSDBServer(url, false, httputil.NewSingleHostReverseProxy(url))

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
	b := NewTSDBServer(url, false, httputil.NewSingleHostReverseProxy(url))

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
	b := NewTSDBServer(url, false, httputil.NewSingleHostReverseProxy(url))

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
	b := NewTSDBServer(url, false, httputil.NewSingleHostReverseProxy(url))
	b.SetAlive(b.IsAlive())

	if !b.IsAlive() {
		t.Errorf("expected backend to be alive")
	}
}
