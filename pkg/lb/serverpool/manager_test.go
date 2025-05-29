package serverpool

import (
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/mahendrapaipuri/ceems/pkg/lb/backend"
	"github.com/mahendrapaipuri/ceems/pkg/lb/base"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func SleepHandler(w http.ResponseWriter, r *http.Request) {
	time.Sleep(3 * time.Second)
}

var h = http.HandlerFunc(SleepHandler)

var noOpLogger = slog.New(slog.DiscardHandler)

func TestNew(t *testing.T) {
	for _, strategy := range []base.LBStrategy{base.RoundRobin, base.LeastConnection} {
		m, _ := New(strategy, noOpLogger)
		b, err := backend.NewTSDB(&backend.ServerConfig{Web: &models.WebConfig{URL: "http://localhost:3333"}}, noOpLogger)
		require.NoError(t, err)

		m.Add("default", b)

		assert.Equal(t, 1, m.Size("default"))
	}
}

func TestNewUnknownStrategy(t *testing.T) {
	_, err := New(-1, noOpLogger)
	assert.Error(t, err)
}
