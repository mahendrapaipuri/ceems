package grafana

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/go-kit/log"
	config_util "github.com/prometheus/common/config"
)

func TestNewGrafanaWithNoURL(t *testing.T) {
	grafana, err := NewGrafana("", config_util.HTTPClientConfig{}, log.NewNopLogger())
	if err != nil {
		t.Errorf("Failed to create Grafana instance")
	}
	if grafana.Available() {
		t.Errorf("Expected Grafana to not available")
	}
}

func TestNewGrafanaWithURL(t *testing.T) {
	// Start test server
	expected := "dummy data"
	t.Setenv("GRAFANA_API_TOKEN", "foo")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(expected))
	}))
	defer server.Close()

	grafana, err := NewGrafana(server.URL, config_util.HTTPClientConfig{}, log.NewNopLogger())
	if err != nil {
		t.Errorf("Failed to create Grafana instance")
	}
	if !grafana.Available() {
		t.Errorf("Expected Grafana to available")
	}

	// Check if Ping is working
	if err := grafana.Ping(); err != nil {
		t.Errorf("Could not ping Grafana")
	}
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

	grafana, err := NewGrafana(server.URL, config_util.HTTPClientConfig{}, log.NewNopLogger())
	if err != nil {
		t.Errorf("Failed to create Grafana instance")
	}
	if !grafana.Available() {
		t.Errorf("Expected Grafana to available")
	}

	if m, err := grafana.TeamMembers([]string{"0"}); err != nil {
		t.Errorf("Expected Grafana query to return value: %s", err)
	} else {
		if !reflect.DeepEqual(m, []string{"foo", "bar"}) {
			t.Errorf("Expected {foo, bar}, got %v", m)
		}
	}
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

	grafana, err := NewGrafana(server.URL, config_util.HTTPClientConfig{}, log.NewNopLogger())
	if err != nil {
		t.Errorf("Failed to create Grafana instance")
	}
	if !grafana.Available() {
		t.Errorf("Expected Grafana to available")
	}

	if _, err := grafana.teamMembers(""); err == nil {
		t.Errorf("Expected Grafana query to return error")
	}
}
