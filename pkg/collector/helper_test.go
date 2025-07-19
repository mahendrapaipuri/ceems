package collector

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type logLine struct {
	Level   string `json:"level"`
	Message string `json:"msg"`
	Source  string `json:"source"`
	AAttr   int    `json:"a"`
	PAttr   string `json:"p"`
}

func TestGokitLogger(t *testing.T) {
	// logfmt
	for _, lvl := range []string{"info", "error", "warn", ""} {
		// slog logger
		buf := &bytes.Buffer{}
		slogger := slog.New(slog.NewTextHandler(buf, nil))
		logger := NewGokitLogger(lvl, slogger)

		// When level is empty string, we default to info
		if lvl == "" {
			lvl = "info"
		}

		kvs := []any{"a", 123}
		lc := log.With(logger, kvs...)

		err := lc.Log("msg", "message")
		require.NoError(t, err)
		assert.Equal(
			t, "msg=message source=helper_test.go:40 a=123",
			strings.TrimSpace(strings.Split(buf.String(), "level="+strings.ToUpper(lvl))[1]),
		)

		buf.Reset()

		lc = log.WithPrefix(lc, "p", "first")
		lc.Log("msg", "message")
		require.NoError(t, err)
		assert.Equal(
			t, "msg=message p=first source=helper_test.go:50 a=123",
			strings.TrimSpace(strings.Split(buf.String(), "level="+strings.ToUpper(lvl))[1]),
		)
	}

	// json format
	for _, lvl := range []string{"info", "error", "warn", ""} {
		// slog logger
		buf := &bytes.Buffer{}
		slogger := slog.New(slog.NewJSONHandler(buf, nil))
		logger := NewGokitLogger(lvl, slogger)

		// When level is empty string, we default to info
		if lvl == "" {
			lvl = "info"
		}

		kvs := []any{"a", 123}
		lc := log.With(logger, kvs...)

		err := lc.Log("msg", "message")
		require.NoError(t, err)

		var got logLine
		err = json.Unmarshal(buf.Bytes(), &got)
		require.NoError(t, err)

		assert.Equal(t, logLine{strings.ToUpper(lvl), "message", "helper_test.go:73", 123, ""}, got)

		buf.Reset()

		lc = log.WithPrefix(lc, "p", "first")
		err = lc.Log("msg", "message")
		require.NoError(t, err)

		err = json.Unmarshal(buf.Bytes(), &got)
		require.NoError(t, err)
		assert.Equal(t, logLine{strings.ToUpper(lvl), "message", "helper_test.go:85", 123, "first"}, got)
	}
}

func TestAreEqual(t *testing.T) {
	testCases := []struct {
		inputA   []string
		inputB   []string
		expected bool
	}{
		{
			inputA:   []string{"a", "b", "c"},
			inputB:   []string{"a", "c", "b"},
			expected: true,
		},
		{
			inputA:   []string{"a1", "b", "c2"},
			inputB:   []string{"a1", "c2", "b"},
			expected: true,
		},
		{
			inputA:   []string{"a", "b", "c2"},
			inputB:   []string{"a1", "c2", "b"},
			expected: false,
		},
		{
			inputA:   []string{"a", "b"},
			inputB:   []string{"a1", "c2", "b"},
			expected: false,
		},
	}

	for _, tc := range testCases {
		got := areEqual(tc.inputA, tc.inputB)
		assert.Equal(t, tc.expected, got)
	}
}

func TestElementsCount(t *testing.T) {
	testCases := []struct {
		input    []string
		expected map[string]uint64
	}{
		{
			input: []string{"a", "a", "b", "c", "c"},
			expected: map[string]uint64{
				"a": 2,
				"b": 1,
				"c": 2,
			},
		},
		{
			input: []string{"a", "b", "c", "c", "a"},
			expected: map[string]uint64{
				"a": 2,
				"b": 1,
				"c": 2,
			},
		},
	}

	for _, tc := range testCases {
		got := elementCounts(tc.input)
		for h, v := range got {
			assert.Equal(t, tc.expected[h.Value()], v)
		}
	}
}

func TestParseRange(t *testing.T) {
	testCases := []struct {
		input    string
		expected []string
	}{
		{
			input:    "0-2",
			expected: []string{"0", "1", "2"},
		},
		{
			input:    "0-2,7-9",
			expected: []string{"0", "1", "2", "7", "8", "9"},
		},
		{
			input:    "0-2,5",
			expected: []string{"0", "1", "2", "5"},
		},
		{
			input:    "0,5",
			expected: []string{"0", "5"},
		},
	}

	for _, tc := range testCases {
		got, err := parseRange(tc.input)
		require.NoError(t, err)

		assert.Equal(t, tc.expected, got)
	}
}

func TestSantizeMetricName(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{
			input:    "metric-name",
			expected: "metric_name",
		},
		{
			input:    "metric-name$",
			expected: "metric_name_",
		},
		{
			input:    "ns/metric-name",
			expected: "ns_metric_name",
		},
	}

	for _, tc := range testCases {
		got := SanitizeMetricName(tc.input)
		assert.Equal(t, tc.expected, got)
	}
}

func TestInode(t *testing.T) {
	absPath, err := filepath.Abs("testdata")
	require.NoError(t, err)

	inodeValue, err := inode(absPath)
	require.NoError(t, err)

	assert.Positive(t, inodeValue)
}

func TestLookupCgroupsRootError(t *testing.T) {
	// Look for non existent name
	_, err := lookupCgroupRoots("testdata/sys/fs/cgroup/system.slice", "doesnotexit.scope")
	require.Error(t, err)
}
