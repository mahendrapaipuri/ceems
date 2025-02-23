package common

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mahendrapaipuri/ceems/pkg/grafana"
	"github.com/prometheus/common/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockConfig struct {
	Field1 string `yaml:"field1"`
	Field2 string `yaml:"field2"`
}

func TestSanitizeFloat(t *testing.T) {
	tests := []struct {
		name  string
		input float64
	}{
		{
			name:  "With +Inf",
			input: math.Inf(0),
		},
		{
			name:  "With -Inf",
			input: math.Inf(-1),
		},
		{
			name:  "With NaN",
			input: math.NaN(),
		},
	}

	for _, test := range tests {
		got := SanitizeFloat(test.input)
		assert.Zero(t, got, test.name)
	}
}

func TestGenerateKey(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected uint64
	}{
		{
			name:     "Regular URL",
			input:    "/foo",
			expected: 0xf9be0e9c9154425e,
		},
		{
			name:     "URL with query params",
			input:    "/foo?q1=bar&q2=bar2",
			expected: 0xe71d223b0fec69f4,
		},
		{
			name:     "URL with special characters",
			input:    "/?^1234567890ßqwertzuiopü+asdfghjklöä#<yxcvbnm",
			expected: 0x19a8532cae702ffa,
		},
	}

	for _, test := range tests {
		got := GenerateKey(test.input)
		assert.Equal(t, test.expected, got, test.name)
	}
}

func TestRound(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		side     string
		expected int64
	}{
		{
			name:     "Default floor",
			input:    400,
			expected: 0,
		},
		{
			name:     "Default ceil",
			input:    897,
			expected: 900,
		},
		{
			name:     "Right round",
			input:    400,
			side:     "right",
			expected: 900,
		},
		{
			name:     "Left round",
			input:    897,
			side:     "left",
			expected: 0,
		},
	}

	for _, test := range tests {
		got := Round(test.input, 900, test.side)
		assert.Equal(t, test.expected, got, test.name)
	}
}

func TestGetUuid(t *testing.T) {
	expected := "d808af89-684c-6f3f-a474-8d22b566dd12"
	got, err := GetUUIDFromString([]string{"foo", "1234", "bar567"})
	require.NoError(t, err)

	// Check if UUIDs match
	assert.Equal(t, expected, got, "mismatched UUIDs")
}

func TestConvertMapI2MapS(t *testing.T) {
	cases := []struct {
		title string      // Title of the test case
		v     interface{} // Input dynamic object
		exp   interface{} // Expected result
	}{
		{
			title: "nil value",
			v:     nil,
			exp:   nil,
		},
		{
			title: "string value",
			v:     "a",
			exp:   "a",
		},
		{
			title: "map[interfac{}]interface{} value",
			v: map[interface{}]interface{}{
				"s": "s",
				1:   1,
			},
			exp: map[string]interface{}{
				"s": "s",
				"1": 1,
			},
		},
		{
			title: "nested maps and slices",
			v: map[interface{}]interface{}{
				"s": "s",
				1:   1,
				float64(0): []interface{}{
					1,
					"x",
					map[interface{}]interface{}{
						"s": "s",
						2.0: 2,
					},
					map[string]interface{}{
						"s": "s",
						"1": 1,
					},
				},
			},
			exp: map[string]interface{}{
				"s": "s",
				"1": 1,
				"0": []interface{}{
					1,
					"x",
					map[string]interface{}{
						"s": "s",
						"2": 2,
					},
					map[string]interface{}{
						"s": "s",
						"1": 1,
					},
				},
			},
		},
	}

	for _, c := range cases {
		v := ConvertMapI2MapS(c.v)
		if !reflect.DeepEqual(v, c.exp) {
			t.Errorf("[title: %s] Expected value: %v, got: %v", c.title, c.exp, c.v)
		}
	}
}

func TestMakeConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := `
---
field1: foo
field2: bar`
	configPath := filepath.Join(tmpDir, "config.yml")
	os.WriteFile(configPath, []byte(configFile), 0o600)

	// Check error when no file path is provided
	_, err := MakeConfig[mockConfig]("")
	require.Error(t, err, "expected error due to missing file path")

	// Check if config file is correctly read
	expected := &mockConfig{Field1: "foo", Field2: "bar"}
	cfg, err := MakeConfig[mockConfig](configPath)
	require.NoError(t, err)
	assert.Equal(t, expected, cfg)
}

func TestGetFreePort(t *testing.T) {
	_, _, err := GetFreePort()
	require.NoError(t, err)
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
	client, err = NewGrafanaClient(config, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err, "failed to create Grafana client")

	teamMembers, err := client.TeamMembers(context.Background(), []string{"1"})
	require.NoError(t, err, "failed to fetch team members")
	assert.Equal(t, teamMembers[0], "Bearer "+expected, "headers do not match")
}

func TestComputeExternalURL(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{
			input: "",
			valid: true,
		},
		{
			input: "http://proxy.com/prometheus",
			valid: true,
		},
		{
			input: "'https://url/prometheus'",
			valid: false,
		},
		{
			input: "'relative/path/with/quotes'",
			valid: false,
		},
		{
			input: "http://alertmanager.company.com",
			valid: true,
		},
		{
			input: "https://double--dash.de",
			valid: true,
		},
		{
			input: "'http://starts/with/quote",
			valid: false,
		},
		{
			input: "ends/with/quote\"",
			valid: false,
		},
	}

	for _, test := range tests {
		_, err := ComputeExternalURL(test.input, "0.0.0.0:9090")
		if test.valid {
			assert.NoError(t, err)
		} else {
			assert.Error(t, err)
		}
	}
}
