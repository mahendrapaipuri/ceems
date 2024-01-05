package cli

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

func queryServer(address string) error {
	resp, err := http.Get(fmt.Sprintf("http://%s/api/health", address))
	if err != nil {
		return err
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if err := resp.Body.Close(); err != nil {
		return err
	}
	if want, have := http.StatusOK, resp.StatusCode; want != have {
		return fmt.Errorf("want /metrics status code %d, have %d. Body:\n%s", want, have, b)
	}
	return nil
}

func TestBatchStatsServerMain(t *testing.T) {
	tmpDir := t.TempDir()
	// Remove test related args
	os.Args = append([]string{os.Args[0]}, fmt.Sprintf("--data.path=%s", tmpDir))
	os.Args = append(os.Args, "--batch.scheduler.slurm")
	os.Args = append(os.Args, "--slurm.sacct.path=../fixtures/sacct")
	a, _ := NewBatchJobStatsServer()

	// Start Main
	go func() {
		a.Main()
	}()

	// Query exporter
	for i := 0; i < 10; i++ {
		if err := queryServer("localhost:9020"); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
		if i == 9 {
			t.Errorf("Could not start exporter after %d attempts", i)
		}
	}
}
