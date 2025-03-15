//go:build !emissions
// +build !emissions

package emissions

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	wtAPIBaseURL        = "https://api.watttime.org"
	wtEmissionsProvider = "wt"
	wtTimeFormat        = "2006-01-02T15:04:05+07:00"
	lbMWhTogmkWh        = 0.453592
)

// like this.
var tokenLifeDuration = 25 * time.Minute

type auth struct {
	username        string
	password        string
	apiToken        string
	tokenDuration   time.Duration
	tokenExpiryTime int64
}

type wtProvider struct {
	logger             *slog.Logger
	auth               *auth
	region             string
	updatePeriod       time.Duration
	stopTicker         chan bool
	lastEmissionFactor EmissionFactors
	fetch              func(baseURL string, auth *auth, region string) (EmissionFactors, error)
}

func init() {
	// Register emissions provider
	Register(wtEmissionsProvider, "Watt Time", NewWattTimeProvider)
}

// NewWattTimeProvider returns a new Provider that returns emission factor from WattTime data.
func NewWattTimeProvider(logger *slog.Logger) (Provider, error) {
	// Check if WT_USERNAME and WT_PASSWORD are set
	var wtUsername, wtPassword, wtRegion string

	var ok bool

	if wtUsername, ok = os.LookupEnv("WT_USERNAME"); ok {
		if wtPassword, ok = os.LookupEnv("WT_PASSWORD"); ok {
			if wtRegion, ok = os.LookupEnv("WT_REGION"); ok {
				logger.Info("Emission factor from WattTime will be reported.")
			}
		}
	} else {
		return nil, ErrMissingInput
	}

	// Make a instance of struct
	w := &wtProvider{
		logger: logger,
		auth: &auth{
			username:      wtUsername,
			password:      wtPassword,
			tokenDuration: tokenLifeDuration,
		},
		region:       wtRegion,
		updatePeriod: 2 * time.Minute,
		fetch:        fetchWTEmissionFactor,
	}

	// To override baseURL in tests
	url := wtAPIBaseURL
	if baseURL, present := os.LookupEnv("__WT_BASE_URL"); present {
		url = baseURL
	}

	// Update API token
	var err error

	// Try few times before giving up
	for range 5 {
		if err = updateToken(url, w.auth); err == nil {
			break
		}

		time.Sleep(time.Second)
	}

	if err != nil {
		logger.Error("Failed to fetch API token for Watt time API", "err", err)

		return nil, fmt.Errorf("failed to fetch api token for watt time: %w", err)
	}

	// Start update ticker
	w.update()

	return w, nil
}

// Update updates the emission factor.
func (s *wtProvider) Update() (EmissionFactors, error) {
	// If data is present, return it
	if len(s.lastEmissionFactor) > 0 {
		return s.lastEmissionFactor, nil
	}

	return nil, fmt.Errorf("failed to fetch emission factor from %s", wtEmissionsProvider)
}

// Stop updaters and release all resources.
func (s *wtProvider) Stop() error {
	// Stop ticker
	close(s.stopTicker)

	return nil
}

// update fetches the emission factors from Watt Time in a ticker.
func (s *wtProvider) update() {
	// Channel to signal closing ticker
	s.stopTicker = make(chan bool, 1)

	// Run ticker in a go routine
	go func() {
		ticker := time.NewTicker(s.updatePeriod)
		defer ticker.Stop()

		for {
			s.logger.Debug("Updating Watt Time emission factor")

			// Fetch factor
			currentEmissionFactor, err := s.fetch(wtAPIBaseURL, s.auth, s.region)
			if err != nil {
				s.logger.Error("Failed to retrieve emission factor from Watt Time provider", "err", err)
			} else {
				rteFactorMu.Lock()
				s.lastEmissionFactor = currentEmissionFactor
				rteFactorMu.Unlock()
			}

			select {
			case <-ticker.C:
				continue
			case <-s.stopTicker:
				s.logger.Info("Stopping Watt Time emission factor update")

				return
			}
		}
	}()
}

// fetchWTEmissionFactor makes request to Watt time API to fetch factor for the given region.
func fetchWTEmissionFactor(baseURL string, auth *auth, region string) (EmissionFactors, error) {
	// Update token if necessary
	if err := updateToken(baseURL, auth); err != nil {
		return nil, fmt.Errorf("failed to update api token of watt time provider: %w", err)
	}

	// Make URL
	reqURL := baseURL + "/v3/historical"

	// Attempt to get last 15 min data and always take the latest one from response
	currentTime := time.Now()
	endTime := currentTime.Format(wtTimeFormat)
	startTime := currentTime.Add(-15 * time.Minute).Format(wtTimeFormat)

	// Set up params
	params := url.Values{
		"start":       []string{startTime},
		"end":         []string{endTime},
		"region":      []string{region},
		"signal_type": []string{"co2_moer"},
	}

	// Make request
	response, err := wtAPIRequest[wtSignalDataResponse](reqURL, auth, params)
	if err != nil {
		return nil, err
	}

	// Ensure that data has been returned
	if len(response.Data) > 0 {
		return EmissionFactors{region: EmissionFactor{region, response.Data[len(response.Data)-1].Value * lbMWhTogmkWh}}, nil
	}

	// Get any warnings returned
	var warns []string
	for _, warn := range response.Meta.Warnings {
		warns = append(warns, warn.Message)
	}

	return nil, fmt.Errorf("missing data in watt time response. warnings: %s", strings.Join(warns, ","))
}

// updateToken fetches new Watt time API token by authenticating using username and password.
func updateToken(baseURL string, auth *auth) error {
	// Check if token is still valid
	if auth.tokenExpiryTime-time.Now().UnixMilli() > 0 {
		return nil
	}

	// Make URL
	url := baseURL + "/login"

	// Make request
	response, err := wtAPIRequest[wtTokenResponse](url, auth, nil)
	if err != nil {
		return err
	}

	// Update auth with token
	auth.apiToken = response.Token
	auth.tokenExpiryTime = time.Now().Add(auth.tokenDuration).UnixMilli()

	return nil
}

// Make a single request to Watt time API
// Returning nil for generics: https://stackoverflow.com/questions/70585852/return-default-value-for-generic-type
func wtAPIRequest[T any](url string, auth *auth, params url.Values) (T, error) {
	// Create a context with timeout. As we are updating in a separate ticker
	// we can use longer timeouts to wait for the response
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return *new(T), fmt.Errorf("failed to create HTTP request for url %s: %w", url, err)
	}

	// Add params to request
	if params != nil {
		req.URL.RawQuery = params.Encode()
	}

	// For login endpoint use basic auth
	if strings.HasSuffix(url, "login") {
		req.Header.Add("Authorization", "Basic "+basicAuth(auth.username, auth.password))
	} else {
		req.Header.Add("Authorization", "Bearer "+auth.apiToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return *new(T), fmt.Errorf("failed to make HTTP request for url %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return *new(T), fmt.Errorf("failed to read HTTP response body for url %s: %w", url, err)
	}

	var data T

	err = json.Unmarshal(body, &data)
	if err != nil {
		return *new(T), fmt.Errorf("failed to unmarshal HTTP response body for url %s: %w", url, err)
	}

	return data, nil
}

func basicAuth(username, password string) string {
	auth := username + ":" + password

	return base64.StdEncoding.EncodeToString([]byte(auth))
}
