package collector

import (
	"fmt"
	"net/http"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func queryExporter(address string) error {
	resp, err := http.Get(fmt.Sprintf("http://%s/metrics", address)) //nolint:noctx
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if want, have := http.StatusOK, resp.StatusCode; want != have {
		return fmt.Errorf("want /metrics status code %d, have %d.", want, have)
	}

	return nil
}

func TestCEEMSExporterMain(t *testing.T) {
	// Remove test related args and add a dummy arg
	os.Args = append([]string{os.Args[0]}, "--web.max-requests=2")

	// Create new instance of exporter CLI app
	a, err := NewCEEMSExporter()
	require.NoError(t, err)

	// Add procfs path
	_, err = a.App.Parse([]string{"--path.procfs", "testdata/proc"})
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

	// Send INT signal and wait a second to clean up server and DB
	syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	time.Sleep(1 * time.Second)
}
