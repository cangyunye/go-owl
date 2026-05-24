package file_test

import (
	"fmt"
	"testing"

	"github.com/cangyunye/go-owl/internal/control/transfer"
)

func TestRelayRoundTrip_ToShellArgsAndParseResults(t *testing.T) {
	subTask := &transfer.RelaySubTask{
		SourceNodeID: "source-1",
		Targets: []transfer.RelayTarget{
			{Host: "root@10.0.0.1:/tmp/file.tar.gz", Password: "s3cret1"},
			{Host: "root@10.0.0.2:/tmp/file.tar.gz", Password: "s3cret2"},
			{Host: "root@10.0.0.3:/tmp/file.tar.gz", Password: "s3cret3"},
		},
		SourceFile: "/data/app.tar.gz",
		TimeoutSec: 300,
	}

	args := subTask.ToShellArgs()

	if len(args) < 6 {
		t.Fatalf("expected at least 6 args, got %d: %v", len(args), args)
	}

	assertFlagPair(t, args, "--source", "/data/app.tar.gz")
	assertFlagPair(t, args, "--targets",
		"root@10.0.0.1:/tmp/file.tar.gz,root@10.0.0.2:/tmp/file.tar.gz,root@10.0.0.3:/tmp/file.tar.gz")
	assertFlagPair(t, args, "--timeout", "300")
	assertFlagPair(t, args, "--passwords", "s3cret1,s3cret2,s3cret3")

	mockCSV := `target,status,error,duration_ms
root@10.0.0.1:/tmp/file.tar.gz,success,,1523
root@10.0.0.2:/tmp/file.tar.gz,failed,Permission denied,3012
root@10.0.0.3:/tmp/file.tar.gz,success,,892
`

	results, err := transfer.ParseRelayResults(mockCSV)
	if err != nil {
		t.Fatalf("unexpected error parsing mock CSV: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	expectedResults := []struct {
		target     string
		status     string
		errMsg     string
		durationMs int64
	}{
		{"root@10.0.0.1:/tmp/file.tar.gz", "success", "", 1523},
		{"root@10.0.0.2:/tmp/file.tar.gz", "failed", "Permission denied", 3012},
		{"root@10.0.0.3:/tmp/file.tar.gz", "success", "", 892},
	}

	for i, expected := range expectedResults {
		r := results[i]
		if r.Target != expected.target {
			t.Errorf("result[%d].Target: expected '%s', got '%s'", i, expected.target, r.Target)
		}
		if r.Status != expected.status {
			t.Errorf("result[%d].Status: expected '%s', got '%s'", i, expected.status, r.Status)
		}
		if r.Error != expected.errMsg {
			t.Errorf("result[%d].Error: expected '%s', got '%s'", i, expected.errMsg, r.Error)
		}
		if r.DurationMs != expected.durationMs {
			t.Errorf("result[%d].DurationMs: expected %d, got %d", i, expected.durationMs, r.DurationMs)
		}
	}
}

func TestRelayRoundTrip_MixedStatusResults(t *testing.T) {
	subTask := &transfer.RelaySubTask{
		SourceNodeID: "source-1",
		Targets: []transfer.RelayTarget{
			{Host: "root@10.0.0.1:/opt/data.zip", Password: "pw1"},
			{Host: "root@10.0.0.2:/opt/data.zip", Password: ""},
		},
		SourceFile: "/data/data.zip",
		TimeoutSec: 120,
	}

	args := subTask.ToShellArgs()

	assertFlagPair(t, args, "--source", "/data/data.zip")
	assertFlagPair(t, args, "--targets", "root@10.0.0.1:/opt/data.zip,root@10.0.0.2:/opt/data.zip")
	assertFlagPair(t, args, "--timeout", "120")
	assertFlagPair(t, args, "--passwords", "pw1,")

	mockCSV := `target,status,error,duration_ms
root@10.0.0.1:/opt/data.zip,timeout,Connection timed out,121000
root@10.0.0.2:/opt/data.zip,auth_failed,Authentication failed,50
`

	results, err := transfer.ParseRelayResults(mockCSV)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if results[0].Status != "timeout" {
		t.Errorf("expected timeout status, got '%s'", results[0].Status)
	}
	if results[0].DurationMs != 121000 {
		t.Errorf("expected 121000ms, got %d", results[0].DurationMs)
	}

	if results[1].Status != "auth_failed" {
		t.Errorf("expected auth_failed status, got '%s'", results[1].Status)
	}
	if results[1].DurationMs != 50 {
		t.Errorf("expected 50ms, got %d", results[1].DurationMs)
	}
}

func TestRelayRoundTrip_NoPasswords(t *testing.T) {
	subTask := &transfer.RelaySubTask{
		SourceNodeID: "source-1",
		Targets: []transfer.RelayTarget{
			{Host: "root@10.0.0.1:/tmp/file.tar.gz", Password: ""},
			{Host: "root@10.0.0.2:/tmp/file.tar.gz", Password: ""},
		},
		SourceFile: "/data/file.tar.gz",
		TimeoutSec: 300,
	}

	args := subTask.ToShellArgs()

	for _, arg := range args {
		if arg == "--passwords" {
			t.Fatalf("--passwords should not be present when no passwords: args=%v", args)
		}
	}

	mockCSV := `target,status,error,duration_ms
root@10.0.0.1:/tmp/file.tar.gz,success,,500
root@10.0.0.2:/tmp/file.tar.gz,success,,620
`

	results, err := transfer.ParseRelayResults(mockCSV)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	for i, r := range results {
		if r.Status != "success" {
			t.Errorf("result[%d]: expected success, got '%s'", i, r.Status)
		}
	}
}

func TestRelayRoundTrip_LargeNumberOfTargets(t *testing.T) {
	targetCount := 50
	targets := make([]transfer.RelayTarget, targetCount)
	expectedTargets := make([]string, targetCount)
	expectedPasswords := make([]string, targetCount)

	for i := 0; i < targetCount; i++ {
		host := fmt.Sprintf("root@10.0.0.%d:/tmp/file.tar.gz", i+1)
		password := fmt.Sprintf("pass%d", i+1)
		targets[i] = transfer.RelayTarget{Host: host, Password: password}
		expectedTargets[i] = host
		expectedPasswords[i] = password
	}

	subTask := &transfer.RelaySubTask{
		SourceNodeID: "source-1",
		Targets:      targets,
		SourceFile:   "/data/file.tar.gz",
		TimeoutSec:   600,
	}

	args := subTask.ToShellArgs()

	assertFlagPair(t, args, "--source", "/data/file.tar.gz")
	assertFlagPair(t, args, "--timeout", "600")

	targetsStr := ""
	passwordsStr := ""
	for i := 0; i < targetCount; i++ {
		if i > 0 {
			targetsStr += ","
			passwordsStr += ","
		}
		targetsStr += expectedTargets[i]
		passwordsStr += expectedPasswords[i]
	}

	assertFlagPair(t, args, "--targets", targetsStr)
	assertFlagPair(t, args, "--passwords", passwordsStr)

	var csvLines string
	csvLines = "target,status,error,duration_ms\n"
	for i := 0; i < targetCount; i++ {
		csvLines += fmt.Sprintf("%s,success,,%d\n", expectedTargets[i], 100+i)
	}

	results, err := transfer.ParseRelayResults(csvLines)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != targetCount {
		t.Fatalf("expected %d results, got %d", targetCount, len(results))
	}

	for i, r := range results {
		if r.Target != expectedTargets[i] {
			t.Errorf("result[%d].Target: expected '%s', got '%s'", i, expectedTargets[i], r.Target)
		}
		if r.DurationMs != int64(100+i) {
			t.Errorf("result[%d].DurationMs: expected %d, got %d", i, 100+i, r.DurationMs)
		}
	}
}

func assertFlagPair(t *testing.T, args []string, flag string, expectedValue string) {
	t.Helper()

	for i, arg := range args {
		if arg == flag {
			if i+1 < len(args) {
				if args[i+1] != expectedValue {
					t.Errorf("flag %s: expected value '%s', got '%s'", flag, expectedValue, args[i+1])
				}
				return
			}
			t.Errorf("flag %s found but no value follows", flag)
			return
		}
	}

	t.Errorf("flag %s not found in args: %v", flag, args)
}
