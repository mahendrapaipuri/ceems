package serverpool

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"testing"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/lb/backend"
	"github.com/stretchr/testify/assert"
)

func SleepHandler(w http.ResponseWriter, r *http.Request) {
	time.Sleep(3 * time.Second)
}

var h = http.HandlerFunc(SleepHandler)

// req = httptest.NewRequest(http.MethodGet, "/test", nil)
// w   = httptest.NewRecorder()

func TestNew(t *testing.T) {
	for _, strategy := range []string{"round-robin", "least-connection", "resource-based"} {
		m, _ := New(strategy, slog.New(slog.NewTextHandler(io.Discard, nil)))
		url, _ := url.Parse("http://localhost:3333")
		b := backend.New(url, httputil.NewSingleHostReverseProxy(url), slog.New(slog.NewTextHandler(io.Discard, nil)))
		m.Add("default", b)

		assert.Equal(t, 1, m.Size("default"))
	}
}

func TestNewUnknownStrategy(t *testing.T) {
	_, err := New("unknown", slog.New(slog.NewTextHandler(io.Discard, nil)))
	assert.Error(t, err)
}
