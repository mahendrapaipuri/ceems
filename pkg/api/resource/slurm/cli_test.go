package slurm

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/go-kit/log"
)

func TestPreflightsCLI(t *testing.T) {
	manager := slurmScheduler{
		logger: log.NewNopLogger(),
	}
	if err := preflightsCLI(&manager); err == nil {
		t.Errorf("expected error due to missing CLI config, got none")
	}

	// Add sacct command to PATH
	sacctPath, _ := filepath.Abs("../../testdata")
	t.Setenv("PATH", fmt.Sprintf("%s:%s", os.Getenv("PATH"), sacctPath))
	if err := preflightsCLI(&manager); err != nil {
		t.Errorf("error in preflightsCLI: %s", err)
	}
	if manager.cluster.CLI.Path != sacctPath {
		t.Errorf("expected path %s, got %s", sacctPath, manager.cluster.CLI.Path)
	}
}

func TestParseSacctCmdOutput(t *testing.T) {
	units, numUnits := parseSacctCmdOutput(sacctCmdOutput, start, end)
	if !reflect.DeepEqual(units, expectedBatchJobs) {
		t.Errorf("Expected batch jobs %#v. \n\nGot %#v", expectedBatchJobs, units)
	}
	if numUnits != 2 {
		t.Errorf("expected 2 units, got %d", numUnits)
	}

	// Job finished in past
	sacctCmdOutput1 := `1479763|part1|qos1|acc1|grp|1000|usr|1000|2023-02-20T14:37:02+0100|2023-02-20T14:37:07+0100|2023-02-20T15:37:07+0100|01:49:22|3000|0:0|RUNNING|billing=80,cpu=160,energy=1439089,gres/gpu=8,mem=320G,node=2|compute-0|test_script1|/home/usr`
	units, _ = parseSacctCmdOutput(sacctCmdOutput1, start, end)
	// Check if elapsed time corresponds to real elapsed time of job
	if units[0].TotalWallTime != 3600 {
		t.Errorf("expected walltime 3600, got %d", units[0].TotalWallTime)
	}

	// Job created but not started
	sacctCmdOutput2 := `1479763|part1|qos1|acc1|grp|1000|usr|1000|2023-02-21T14:37:02+0100|NA|NA|01:49:22|3000|0:0|PENDING|billing=80,cpu=160,energy=1439089,gres/gpu=8,mem=320G,node=2|compute-0|test_script1|/home/usr`
	units, _ = parseSacctCmdOutput(sacctCmdOutput2, start, end)
	// Check if elapsed time corresponds to real elapsed time of job
	if units[0].TotalWallTime != 0 {
		t.Errorf("expected walltime 0, got %d", units[0].TotalWallTime)
	}

	// Job started inside current interval
	sacctCmdOutput3 := `1479763|part1|qos1|acc1|grp|1000|usr|1000|2023-02-21T15:10:00+0100|2023-02-21T15:10:00+0100|NA|01:49:22|3000|0:0|RUNNING|billing=80,cpu=160,energy=1439089,gres/gpu=8,mem=320G,node=2|compute-0|test_script1|/home/usr`
	units, _ = parseSacctCmdOutput(sacctCmdOutput3, start, end)
	// Check if elapsed time corresponds to real elapsed time of job
	if units[0].TotalWallTime != 300 {
		t.Errorf("expected walltime 300, got %d", units[0].TotalWallTime)
	}

	// Job ended inside current interval
	sacctCmdOutput4 := `1479763|part1|qos1|acc1|grp|1000|usr|1000|2023-02-21T14:10:00+0100|2023-02-21T14:10:00+0100|2023-02-21T15:10:00+0100|01:49:22|3000|0:0|COMPLETED|billing=80,cpu=160,energy=1439089,gres/gpu=8,mem=320G,node=2|compute-0|test_script1|/home/usr`
	units, _ = parseSacctCmdOutput(sacctCmdOutput4, start, end)
	// Check if elapsed time corresponds to real elapsed time of job
	if units[0].TotalWallTime != 600 {
		t.Errorf("expected walltime 600, got %d", units[0].TotalWallTime)
	}

	// Job started and ended inside current interval
	sacctCmdOutput5 := `1479763|part1|qos1|acc1|grp|1000|usr|1000|2023-02-21T15:10:00+0100|2023-02-21T15:10:00+0100|2023-02-21T15:12:00+0100|01:49:22|3000|0:0|COMPLETED|billing=80,cpu=160,energy=1439089,gres/gpu=8,mem=320G,node=2|compute-0|test_script1|/home/usr`
	units, _ = parseSacctCmdOutput(sacctCmdOutput5, start, end)
	// Check if elapsed time corresponds to real elapsed time of job
	if units[0].TotalWallTime != 120 {
		t.Errorf("expected walltime 120, got %d", units[0].TotalWallTime)
	}
}

func TestParseSacctMgrCmdOutput(t *testing.T) {
	users, projects := parseSacctMgrCmdOutput(sacctMgrCmdOutput, current.Format(slurmTimeFormat))
	if !reflect.DeepEqual(users, expectedUsers) {
		t.Errorf("Expected users %#v. \n\nGot %#v", expectedUsers, users)
	}
	if !reflect.DeepEqual(projects, expectedProjects) {
		t.Errorf("Expected users %#v. \n\nGot %#v", expectedProjects, projects)
	}
}
