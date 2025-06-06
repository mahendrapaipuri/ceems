// Package grafana implements Grafana client
package grafana

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"

	config_util "github.com/prometheus/common/config"
)

// Custom errors.
var (
	ErrNoTeamIDs = errors.New("Grafana Teams IDs not set")
)

// GrafanaTeamsReponse is the API response struct from Grafana.
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

// Grafana implements Grafana client.
type Grafana struct {
	logger    *slog.Logger
	URL       *url.URL
	Client    *http.Client
	available bool
}

// New return a new instance of Grafana struct.
func New(webURL string, config config_util.HTTPClientConfig, logger *slog.Logger) (*Grafana, error) {
	// If webURL is empty return empty struct with available set to false
	if webURL == "" {
		logger.Warn("Grafana web URL not found")

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
	if grafanaClient, err = config_util.NewClientFromConfig(config, "grafana"); err != nil {
		return nil, err
	}

	return &Grafana{
		URL:       grafanaURL,
		Client:    grafanaClient,
		logger:    logger,
		available: true,
	}, nil
}

// String receiver for Grafana struct.
func (g *Grafana) String() string {
	return fmt.Sprintf("Grafana URL: %s, Is Grafana Online: %t", g.URL.Redacted(), g.available)
}

// Available returns true if Grafana is available.
func (g *Grafana) Available() bool {
	return g.available
}

// Ping attempts to ping Grafana.
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

// TeamMembers fetches team members from a given slice of Grafana teams IDs.
func (g *Grafana) TeamMembers(ctx context.Context, teamsIDs []string) ([]string, error) {
	// Sanity checks
	// Check if adminTeamID is not an empty string
	if teamsIDs == nil {
		return nil, ErrNoTeamIDs
	}

	var allMembers []string

	for _, teamsID := range teamsIDs {
		teamMembers, err := g.teamMembers(ctx, teamsID)
		if err != nil {
			g.logger.Warn("Failed to fetch team members from Grafana Team", "teams_id", teamsID, "err", err)
		} else {
			allMembers = append(allMembers, teamMembers...)
		}
	}

	return allMembers, nil
}

// teamMembersEndpoint returns the URL for fetching team members.
func (g *Grafana) teamMembersEndpoint(teamID string) string {
	return g.URL.JoinPath(fmt.Sprintf("/api/teams/%s/members", teamID)).String()
}

// teamMembers fetches team members from a given Grafana team.
func (g *Grafana) teamMembers(ctx context.Context, teamsID string) ([]string, error) {
	// Check if adminTeamID is not an empty string
	if teamsID == "" {
		return nil, ErrNoTeamIDs
	}

	// Make API URL
	teamMembersURL := g.teamMembersEndpoint(teamsID)

	// Create a new GET request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, teamMembersURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new HTTP request for Grafana teams API: %w", err)
	}

	// Add necessary headers
	req.Header.Add("Content-Type", "application/json")

	// Make request
	resp, err := g.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make HTTP request for Grafana teams API: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read HTTP response body for Grafana teams API: %w", err)
	}

	// Unpack into data
	var data []GrafanaTeamsReponse

	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal HTTP response body for Grafana teams API: %w", err)
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
