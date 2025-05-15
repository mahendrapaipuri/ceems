package collector

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Logger to be used in uni tests.
var (
	noOpLogger = slog.New(slog.DiscardHandler)
)

func queryExporter(address string) error {
	for _, path := range []string{"metrics", "alloy-targets"} {
		resp, err := http.Get(fmt.Sprintf("http://%s/%s", address, path)) //nolint:noctx
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if want, have := http.StatusOK, resp.StatusCode; want != have {
			return fmt.Errorf("want /%s status code %d, have %d.", path, want, have)
		}
	}

	return nil
}

func TestCEEMSExporterMain(t *testing.T) {
	// Add IPMI command to PATH
	absPath, err := filepath.Abs("testdata/ipmi/ipmiutils/ipmiutil")
	require.NoError(t, err)
	// t.Setenv("PATH", absPath+":"+os.Getenv("PATH"))

	// Remove test related args and add a dummy arg
	os.Args = append([]string{os.Args[0]},
		"--web.max-requests=2",
		"--no-security.drop-privileges",
		"--collector.ipmi_dcmi.cmd", absPath,
		"--path.procfs", "testdata/proc",
		"--path.cgroupfs", "testdata/sys/fs/cgroup",
	)

	// Create new instance of exporter CLI app
	a, err := NewCEEMSExporter()
	require.NoError(t, err)

	// Start Main
	go func() {
		a.Main()
	}()

	// Query exporter
	for i := range 10 {
		if err := queryExporter("localhost:9010"); err == nil {
			break
		}

		time.Sleep(500 * time.Millisecond)

		if i == 9 {
			t.Errorf("Could not start exporter after %d attempts", i)
		}
	}

	// Send INT signal and wait a second to clean up server
	syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	time.Sleep(1 * time.Second)
}
