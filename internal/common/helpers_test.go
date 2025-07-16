package common

import (
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/grafana"
	"github.com/prometheus/common/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockConfig struct {
	Field1 string `yaml:"field1"`
	Field2 string `yaml:"field2"`
}

func TestTimeSpan(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Duration
		expected string
	}{
		{
			name:     "Less than day",
			input:    23 * time.Hour,
			expected: "23:00:00",
		},
		{
			name:     "More than day",
			input:    25 * time.Hour,
			expected: "1-01:00:00",
		},
	}

	for _, test := range tests {
		got := Timespan(test.input).Format("15:04:05")
		assert.Equal(t, test.expected, got, test.name)
	}
}

func TestNodelistParser(t *testing.T) {
	tests := []struct {
		nodelist string
		expected []string
	}{
		{
			"compute-a-0", []string{"compute-a-0"},
		},
		{
			"compute-a-[0000-0001]", []string{"compute-a-0000", "compute-a-0001"},
		},
		{
			"compute-a-[00-01,05-06]", []string{"compute-a-00", "compute-a-01", "compute-a-05", "compute-a-06"},
		},
		{
			"compute-a-[0-1]-b-[3-4]",
			[]string{"compute-a-0-b-3", "compute-a-0-b-4", "compute-a-1-b-3", "compute-a-1-b-4"},
		},
		{
			"compute-a-[0-1,3,5-6]-b-[3-4,5,7-9]",
			[]string{
				"compute-a-0-b-3",
				"compute-a-0-b-4",
				"compute-a-0-b-5",
				"compute-a-0-b-7",
				"compute-a-0-b-8",
				"compute-a-0-b-9",
				"compute-a-1-b-3",
				"compute-a-1-b-4",
				"compute-a-1-b-5",
				"compute-a-1-b-7",
				"compute-a-1-b-8",
				"compute-a-1-b-9",
				"compute-a-3-b-3",
				"compute-a-3-b-4",
				"compute-a-3-b-5",
				"compute-a-3-b-7",
				"compute-a-3-b-8",
				"compute-a-3-b-9",
				"compute-a-5-b-3",
				"compute-a-5-b-4",
				"compute-a-5-b-5",
				"compute-a-5-b-7",
				"compute-a-5-b-8",
				"compute-a-5-b-9",
				"compute-a-6-b-3",
				"compute-a-6-b-4",
				"compute-a-6-b-5",
				"compute-a-6-b-7",
				"compute-a-6-b-8",
				"compute-a-6-b-9",
			},
		},
		{
			"compute-a-[0-1]-b-[3-4],compute-c,compute-d",
			[]string{
				"compute-a-0-b-3", "compute-a-0-b-4",
				"compute-a-1-b-3", "compute-a-1-b-4", "compute-c", "compute-d",
			},
		},
		{
			"compute-a-[0-2,5,7-9]-b-[3-4,7,9-12],compute-c,compute-d",
			[]string{
				"compute-a-0-b-3",
				"compute-a-0-b-4",
				"compute-a-0-b-7",
				"compute-a-0-b-9",
				"compute-a-0-b-10",
				"compute-a-0-b-11",
				"compute-a-0-b-12",
				"compute-a-1-b-3",
				"compute-a-1-b-4",
				"compute-a-1-b-7",
				"compute-a-1-b-9",
				"compute-a-1-b-10",
				"compute-a-1-b-11",
				"compute-a-1-b-12",
				"compute-a-2-b-3",
				"compute-a-2-b-4",
				"compute-a-2-b-7",
				"compute-a-2-b-9",
				"compute-a-2-b-10",
				"compute-a-2-b-11",
				"compute-a-2-b-12",
				"compute-a-5-b-3",
				"compute-a-5-b-4",
				"compute-a-5-b-7",
				"compute-a-5-b-9",
				"compute-a-5-b-10",
				"compute-a-5-b-11",
				"compute-a-5-b-12",
				"compute-a-7-b-3",
				"compute-a-7-b-4",
				"compute-a-7-b-7",
				"compute-a-7-b-9",
				"compute-a-7-b-10",
				"compute-a-7-b-11",
				"compute-a-7-b-12",
				"compute-a-8-b-3",
				"compute-a-8-b-4",
				"compute-a-8-b-7",
				"compute-a-8-b-9",
				"compute-a-8-b-10",
				"compute-a-8-b-11",
				"compute-a-8-b-12",
				"compute-a-9-b-3",
				"compute-a-9-b-4",
				"compute-a-9-b-7",
				"compute-a-9-b-9",
				"compute-a-9-b-10",
				"compute-a-9-b-11",
				"compute-a-9-b-12",
				"compute-c",
				"compute-d",
			},
		},
	}

	for _, test := range tests {
		output := NodelistParser(test.nodelist)
		assert.Equal(t, test.expected, output)
	}
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
		title string // Title of the test case
		v     any    // Input dynamic object
		exp   any    // Expected result
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
			v: map[any]any{
				"s": "s",
				1:   1,
			},
			exp: map[string]any{
				"s": "s",
				"1": 1,
			},
		},
		{
			title: "nested maps and slices",
			v: map[any]any{
				"s": "s",
				1:   1,
				float64(0): []any{
					1,
					"x",
					map[any]any{
						"s": "s",
						2.0: 2,
					},
					map[string]any{
						"s": "s",
						"1": 1,
					},
				},
			},
			exp: map[string]any{
				"s": "s",
				"1": 1,
				"0": []any{
					1,
					"x",
					map[string]any{
						"s": "s",
						"2": 2,
					},
					map[string]any{
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
	// Set some sample env vars
	t.Setenv("samplekey1", "samplevalue1")
	t.Setenv("samplekey2", "samplevalue2")

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	// Check error when no file path is provided
	_, err := MakeConfig[mockConfig]("", false)
	require.Error(t, err, "expected error due to missing file path")

	tests := []struct {
		name          string
		config        string
		expandEnvVars bool
		expected      *mockConfig
	}{
		{
			name: "config with no env vars",
			config: `
---
field1: foo
field2: bar`,
			expected: &mockConfig{Field1: "foo", Field2: "bar"},
		},
		{
			name: "config with env vars reference but no bool flag",
			config: `
---
field1: $samplekay1
field2: bar`,
			expected: &mockConfig{Field1: "$samplekay1", Field2: "bar"},
		},
		{
			name: "config with env vars reference with bool flag",
			config: `
---
field1: $samplekey1
field2: ${samplekey2}`,
			expandEnvVars: true,
			expected:      &mockConfig{Field1: "samplevalue1", Field2: "samplevalue2"},
		},
		{
			name: "config with env vars reference with default value with bool flag",
			config: `
---
field1: $samplekey1
field2: ${samplekey3:-test}`,
			expandEnvVars: true,
			expected:      &mockConfig{Field1: "samplevalue1", Field2: ""},
		},
		{
			name: "config with env vars reference with escpaing $ value with bool flag",
			config: `
---
field1: $samplekey1
field2: somerandom$$stuff`,
			expandEnvVars: true,
			expected:      &mockConfig{Field1: "samplevalue1", Field2: "somerandom$stuff"},
		},
		{
			name: "config with env vars with forbidden template token",
			config: `
---
field1: $samplekey1
field2: ${samplekey3}` + templateToken,
			expandEnvVars: true,
			expected:      nil,
		},
	}

	for _, test := range tests {
		os.WriteFile(configPath, []byte(test.config), 0o600)

		cfg, err := MakeConfig[mockConfig](configPath, test.expandEnvVars)

		if test.expected != nil {
			require.NoError(t, err, test.name)
			assert.Equal(t, test.expected, cfg, test.name)
		} else {
			require.Error(t, err, test.name)
		}
	}
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
	client, err = NewGrafanaClient(config, slog.New(slog.DiscardHandler))
	require.NoError(t, err, "failed to create Grafana client")

	teamMembers, err := client.TeamMembers(t.Context(), []string{"1"})
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
