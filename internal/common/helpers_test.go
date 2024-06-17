package common

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-kit/log"
	"github.com/mahendrapaipuri/ceems/pkg/grafana"
	"github.com/prometheus/common/config"
)

func TestGetUuid(t *testing.T) {
	expected := "d808af89-684c-6f3f-a474-8d22b566dd12"
	got, err := GetUUIDFromString([]string{"foo", "1234", "bar567"})
	if err != nil {
		t.Errorf("Failed to generate UUID due to %s", err)
	}

	// Check if UUIDs match
	if expected != got {
		t.Errorf("Mismatched UUIDs. Expected %s Got %s", expected, got)
	}
}

func TestGrafanaClient(t *testing.T) {
	// Start mock server
	expected := "dummy"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		teamMembers := []grafana.GrafanaTeamsReponse{
			{
				Login: r.Header.Get("Authorization"),
			},
		}
		if err := json.NewEncoder(w).Encode(&teamMembers); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	// Make config file
	config := &GrafanaWebConfig{
		URL: server.URL,
		HTTPClientConfig: config.HTTPClientConfig{
			Authorization: &config.Authorization{
				Type:        "Bearer",
				Credentials: config.Secret(expected),
			},
		},
	}

	// Create grafana client
	var client *grafana.Grafana
	var err error
	if client, err = CreateGrafanaClient(config, log.NewNopLogger()); err != nil {
		t.Errorf("failed to create Grafana client: %s", err)
	}

	teamMembers, err := client.TeamMembers([]string{"1"})
	if err != nil {
		t.Errorf("failed to fetch team members: %s", err)
	}
	if teamMembers[0] != fmt.Sprintf("Bearer %s", expected) {
		t.Errorf("expected %s, got %s", fmt.Sprintf("Bearer %s", expected), teamMembers[0])
	}
}
