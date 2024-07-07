package serverpool

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/mahendrapaipuri/ceems/pkg/lb/backend"
	"github.com/stretchr/testify/assert"
)

func SleepHandler(w http.ResponseWriter, r *http.Request) {
	time.Sleep(3 * time.Second)
}

var (
	h = http.HandlerFunc(SleepHandler)
	// req = httptest.NewRequest(http.MethodGet, "/test", nil)
	// w   = httptest.NewRecorder()
)

func TestNewManager(t *testing.T) {
	for _, strategy := range []string{"round-robin", "least-connection", "resource-based"} {
		m, _ := NewManager(strategy, log.NewNopLogger())
		url, _ := url.Parse("http://localhost:3333")
		b := backend.NewTSDBServer(url, httputil.NewSingleHostReverseProxy(url), log.NewNopLogger())
		m.Add("default", b)

		assert.Equal(t, 1, m.Size("default"))
	}
}

func TestNewManagerUnknownStrategy(t *testing.T) {
	_, err := NewManager("unknown", log.NewNopLogger())
	assert.Error(t, err)
}
