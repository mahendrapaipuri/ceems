package serverpool

import (
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/lb/backend"
)

func SleepHandler(w http.ResponseWriter, r *http.Request) {
	time.Sleep(3 * time.Second)
}

var (
	h   = http.HandlerFunc(SleepHandler)
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	w   = httptest.NewRecorder()
)

func TestNewManager(t *testing.T) {
	for _, strategy := range []string{"round-robin", "least-connection", "resource-based"} {
		m, _ := NewManager(strategy)
		url, _ := url.Parse("http://localhost:3333")
		b := backend.NewTSDBServer(url, false, httputil.NewSingleHostReverseProxy(url))
		m.Add(b)

		if m.Size() != 1 {
			t.Errorf("expected 1 backend TSDB servers, got %d", m.Size())
		}
	}
}

func TestNewManagerUnknownStrategy(t *testing.T) {
	_, err := NewManager("unknown")
	if err == nil {
		t.Errorf("expected error in creating a manager with unknown strategy")
	}
}
