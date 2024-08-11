package grafana

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-kit/log"
	config_util "github.com/prometheus/common/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGrafanaWithNoURL(t *testing.T) {
	grafana, err := New("", config_util.HTTPClientConfig{}, log.NewNopLogger())
	require.NoError(t, err)
	assert.False(t, grafana.Available())
}

func TestNewGrafanaWithURL(t *testing.T) {
	// Start test server
	expected := "dummy data"

	t.Setenv("GRAFANA_API_TOKEN", "foo")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(expected))
	}))
	defer server.Close()

	grafana, err := New(server.URL, config_util.HTTPClientConfig{}, log.NewNopLogger())
	require.NoError(t, err)
	assert.True(t, grafana.Available())

	// Check if Ping is working
	assert.NoError(t, grafana.Ping())
}

func TestGrafanaTeamMembersQuerySuccess(t *testing.T) {
	// Start test server
	expected := []GrafanaTeamsReponse{
		{Login: "foo"}, {Login: "bar"},
	}

	t.Setenv("GRAFANA_API_TOKEN", "foo")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	grafana, err := New(server.URL, config_util.HTTPClientConfig{}, log.NewNopLogger())
	require.NoError(t, err)
	assert.True(t, grafana.Available())

	m, err := grafana.TeamMembers(context.Background(), []string{"0"})
	require.NoError(t, err)
	assert.Equal(t, []string{"foo", "bar"}, m)
}

func TestGrafanaTeamMembersQueryFailNoTeamID(t *testing.T) {
	// Start test server
	expected := []GrafanaTeamsReponse{
		{Login: "foo"}, {Login: "bar"},
	}

	t.Setenv("GRAFANA_API_TOKEN", "foo")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	grafana, err := New(server.URL, config_util.HTTPClientConfig{}, log.NewNopLogger())
	require.NoError(t, err)
	assert.True(t, grafana.Available())

	_, err = grafana.teamMembers(context.Background(), "")
	assert.Error(t, err)
}
