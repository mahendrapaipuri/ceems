//go:build amd64 || arm64
// +build amd64 arm64

package collector

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	ebpfspy "github.com/grafana/pyroscope/ebpf"
	"github.com/grafana/pyroscope/ebpf/pprof"
	"github.com/grafana/pyroscope/ebpf/sd"
	"github.com/grafana/pyroscope/ebpf/symtab"
	"github.com/grafana/pyroscope/ebpf/symtab/elf"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProfilingConfig(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		config      string
		errExpected bool
	}{
		{
			name: "invalid demangle",
			config: `
---
ceems_profiler:
  ebpf:
    demangle: foo`,
			errExpected: true,
		},
		{
			name: "Upper case demangle",
			config: `
---
ceems_profiler:
  ebpf:
    demangle: FULL
  pyroscope:
    external_labels:
      mylabel: value`,
			errExpected: false,
		},
		{
			name: "Mixed case demangle",
			config: `
---
ceems_profiler:
  ebpf:
    demangle: TemPLAtes`,
			errExpected: false,
		},
		{
			name: "Invalid URL",
			config: `
---
ceems_profiler:
  pyroscope:
    url: http://192.168.0.%31:4040/`,
			errExpected: true,
		},
	}

	// Enable slurm collector on CLI
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--collector.slurm",
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
		},
	)
	require.NoError(t, err)

	for _, test := range tests {
		configPath := filepath.Join(tmpDir, "config.yml")
		os.WriteFile(configPath, []byte(test.config), 0o600)

		// Profiler config
		c := &profilerConfig{
			logger:     noOpLogger,
			configFile: configPath,
			enabled:    true,
		}

		// New instance of profiler
		_, err := NewProfiler(c)
		if test.errExpected {
			require.Error(t, err, test.name)
		} else {
			require.NoError(t, err, test.name)
		}
	}
}

func TestNewProfiler(t *testing.T) {
	skipUnprivileged(t)

	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
			"--collector.cgroups.force-version", "v2",
			"--collector.slurm",
		},
	)
	require.NoError(t, err)

	// Test Pyroscope server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	tmpDir := t.TempDir()

	// Config file
	configContent := `
---
ceems_profiler:
  ebpf:
    collect_interval: 500ms
    discover_interval: 500ms
  pyroscope:
    url: %s
    external_labels:
      test: mylabel`

	configPath := filepath.Join(tmpDir, "config.yml")
	config := fmt.Sprintf(configContent, server.URL)
	os.WriteFile(configPath, []byte(config), 0o600)

	// Profiler config
	c := &profilerConfig{
		logger:      noOpLogger,
		configFile:  configPath,
		enabled:     true,
		selfProfile: true,
	}

	// New instance of profiler
	profiler, err := NewProfiler(c)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())

	// Start profiling in go routine
	go profiler.Start(ctx)

	// Sleep for a while
	time.Sleep(2 * time.Second)

	// Stop profiling
	cancel()
	profiler.Stop()
}

// Nicked from Grafana Alloy
// Ref: https://github.com/grafana/alloy/blob/main/internal/component/pyroscope/ebpf/ebpf_linux_test.go
type mockSession struct {
	options         ebpfspy.SessionOptions
	collectCallback func() error
	collected       int
	data            [][]string
	dataTarget      *sd.Target
	mtx             sync.Mutex
}

func (m *mockSession) Start() error {
	return nil
}

func (m *mockSession) Stop() {
}

func (m *mockSession) Update(options ebpfspy.SessionOptions) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.options = options

	return nil
}

func (m *mockSession) UpdateTargets(_ sd.TargetsOptions) {
}

func (m *mockSession) CollectProfiles(f pprof.CollectProfilesCallback) error {
	m.collected++
	if m.collectCallback != nil {
		return m.collectCallback()
	}

	for _, stack := range m.data {
		f(
			pprof.ProfileSample{
				Target:      m.dataTarget,
				Pid:         0,
				SampleType:  pprof.SampleTypeCpu,
				Aggregation: pprof.SampleAggregation(false),
				Stack:       stack,
				Value:       1,
				Value2:      0,
			})
	}

	return nil
}

func (m *mockSession) DebugInfo() any {
	return ebpfspy.SessionDebugInfo{
		ElfCache: symtab.ElfCacheDebugInfo{
			BuildIDCache: symtab.GCacheDebugInfo[elf.SymTabDebugInfo]{},
			SameFileCache: symtab.GCacheDebugInfo[elf.SymTabDebugInfo]{
				LRUSize:      10,
				RoundSize:    10,
				CurrentRound: 1,
				LRUDump: []elf.SymTabDebugInfo{
					{
						Name:          "X",
						Size:          123,
						MiniDebugInfo: false,
						LastUsedRound: 1,
					},
				},
			},
		},
		PidCache: symtab.GCacheDebugInfo[symtab.ProcTableDebugInfo]{
			LRUSize:      10,
			RoundSize:    10,
			CurrentRound: 1,
			LRUDump: []symtab.ProcTableDebugInfo{
				{
					Pid:  666,
					Size: 123,
				},
			},
		},
		Arch:   "my-arch",
		Kernel: "my-kernel",
	}
}

type mockDiscoverer struct {
	logger  *slog.Logger
	updated int
}

func (m *mockDiscoverer) Discover() ([]Target, error) {
	m.updated++

	return []Target{{Labels: sd.DiscoveryTarget{"__process_pid__": "0", "service_name": "root"}}}, nil
}

func (m *mockDiscoverer) Enabled() bool {
	return true
}

func TestTargetUpdatesAndCollection(t *testing.T) {
	// Mock session
	session := &mockSession{
		data: [][]string{
			{"a", "b", "c"},
			{"q", "w", "e"},
		},
		dataTarget: sd.NewTarget("cid", 0, map[string]string{"service_name": "foo"}),
	}
	discoverer := &mockDiscoverer{logger: noOpLogger}

	// Make a new config
	config := &ProfilerConfig{}
	config.Profiler.Session = defaultSessionConfig
	config.Profiler.Session.CollectInterval = model.Duration(100 * time.Millisecond)
	config.Profiler.Session.DiscoverInterval = model.Duration(200 * time.Millisecond)

	// Create an instance of profiler
	p := &eBPFProfiler{
		logger:         noOpLogger,
		session:        session,
		config:         config,
		sessionOptions: convertSessionOptions(config),
		targetsFinder:  discoverer,
		enabled:        true,
	}

	var wg sync.WaitGroup

	wg.Add(1)

	ctx, cancel := context.WithCancel(t.Context())

	go func() {
		defer wg.Done()

		err := p.Start(ctx)
		require.NoError(t, err) //nolint:testifylint
	}()

	time.Sleep(time.Second)

	// Stop collection
	cancel()
	p.Stop()
	wg.Wait()

	// wait for the session to be updated
	assert.GreaterOrEqual(t, discoverer.updated, 4)
	assert.GreaterOrEqual(t, session.collected, 9)
}
