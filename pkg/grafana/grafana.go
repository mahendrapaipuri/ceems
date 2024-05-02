// Package grafana implements Grafana client
package grafana

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

// GrafanaTeamsReponse is the API response struct from Grafana
type GrafanaTeamsReponse struct {
	OrgID      int      `json:"orgId"`
	TeamID     int      `json:"teamId"`
	TeamUID    string   `json:"teamUID"`
	UserID     int      `json:"userId"`
	AuthModule string   `json:"auth_module"`
	Email      string   `json:"email"`
	Name       string   `json:"name"`
	Login      string   `json:"login"`
	AvatarURL  string   `json:"avatarUrl"`
	Labels     []string `json:"labels"`
	Permission int      `json:"permission"`
}

// Grafana struct
type Grafana struct {
	logger    log.Logger
	URL       *url.URL
	Client    *http.Client
	available bool
}

// NewGrafana return a new instance of Grafana struct
func NewGrafana(webURL string, webSkipTLSVerify bool, logger log.Logger) (*Grafana, error) {
	// If webURL is empty return empty struct with available set to false
	if webURL == "" {
		level.Warn(logger).Log("msg", "Grafana web URL not found")
		return &Grafana{
			available: false,
		}, nil
	}

	// Parse Grafana web Url
	var grafanaURL *url.URL
	var grafanaClient *http.Client
	var err error
	if grafanaURL, err = url.Parse(webURL); err != nil {
		return nil, errors.Unwrap(err)
	}

	// If skip verify is set to true for TSDB add it to client
	if webSkipTLSVerify {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		grafanaClient = &http.Client{Transport: tr, Timeout: time.Duration(30 * time.Second)}
	} else {
		grafanaClient = &http.Client{Timeout: time.Duration(30 * time.Second)}
	}
	return &Grafana{
		URL:       grafanaURL,
		Client:    grafanaClient,
		logger:    logger,
		available: true,
	}, nil
}

// teamMembersEndpoint returns the URL for fetching team members
func (g *Grafana) teamMembersEndpoint(teamID string) string {
	return g.URL.JoinPath(fmt.Sprintf("/api/teams/%s/members", teamID)).String()
}

// String receiver for Grafana struct
func (g *Grafana) String() string {
	return fmt.Sprintf("Grafana{URL: %s, available: %t}", g.URL.Redacted(), g.available)
}

// Available returns true if Grafana is available
func (g *Grafana) Available() bool {
	return g.available
}

// Ping attempts to ping Grafana
func (g *Grafana) Ping() error {
	var d net.Dialer
	// Check if Grafana host is reachable
	conn, err := d.Dial("tcp", g.URL.Host)
	if err != nil {
		return err
	}
	defer conn.Close()
	return nil
}

// TeamMembers fetches team members from a Grafana team
func (g *Grafana) TeamMembers(teamID string) ([]string, error) {
	// Sanity checks
	// Check if adminTeamID is not an empty string
	if teamID == "" {
		return nil, fmt.Errorf("Grafana Team ID not set")
	}

	// Check if API Token is provided
	if os.Getenv("GRAFANA_API_TOKEN") == "" {
		return nil, fmt.Errorf("GRAFANA_API_TOKEN environment variable not set")
	}

	// Make API URL
	teamMembersURL := g.teamMembersEndpoint(teamID)

	// Create a new GET request
	req, err := http.NewRequest(http.MethodGet, teamMembersURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new HTTP request for Grafana teams API: %s", err)
	}

	// Add token to auth header
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", os.Getenv("GRAFANA_API_TOKEN")))

	// Add necessary headers
	req.Header.Add("Content-Type", "application/json")

	// Make request
	resp, err := g.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make HTTP request for Grafana teams API: %s", err)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read HTTP response body for Grafana teams API: %s", err)
	}

	// Unpack into data
	var data []GrafanaTeamsReponse
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal HTTP response body for Grafana teams API: %s", err)
	}

	// Get list of all usernames and add them to admin users
	var teamMembers []string
	for _, user := range data {
		if user.Login != "" {
			teamMembers = append(teamMembers, user.Login)
		}
	}
	return teamMembers, nil
}
