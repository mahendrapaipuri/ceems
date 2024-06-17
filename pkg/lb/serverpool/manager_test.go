package serverpool

import (
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"
	"time"

	"github.com/go-kit/log"
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
		m, _ := NewManager(strategy, log.NewNopLogger())
		url, _ := url.Parse("http://localhost:3333")
		b := backend.NewTSDBServer(url, httputil.NewSingleHostReverseProxy(url), log.NewNopLogger())
		m.Add("default", b)

		if m.Size("default") != 1 {
			t.Errorf("expected 1 backend TSDB servers, got %d", m.Size("default"))
		}
	}
}

func TestNewManagerUnknownStrategy(t *testing.T) {
	_, err := NewManager("unknown", log.NewNopLogger())
	if err == nil {
		t.Errorf("expected error in creating a manager with unknown strategy")
	}
}
