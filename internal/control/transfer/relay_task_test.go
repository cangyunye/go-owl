package transfer

import (
	"strings"
	"testing"
)

func TestRelaySubTask_ToShellArgs_AllPasswords(t *testing.T) {
	task := &RelaySubTask{
		SourceNodeID: "source-1",
		Targets: []RelayTarget{
			{Host: "root@10.0.0.1:/tmp/file.tar.gz", Password: "pass1"},
			{Host: "root@10.0.0.2:/tmp/file.tar.gz", Password: "pass2"},
			{Host: "root@10.0.0.3:/tmp/file.tar.gz", Password: "pass3"},
		},
		SourceFile: "/data/file.tar.gz",
		TimeoutSec: 300,
	}

	args := task.ToShellArgs()

	assertArgValue(t, args, "--source", "/data/file.tar.gz")
	assertArgValue(t, args, "--targets", "root@10.0.0.1:/tmp/file.tar.gz,root@10.0.0.2:/tmp/file.tar.gz,root@10.0.0.3:/tmp/file.tar.gz")
	assertArgValue(t, args, "--timeout", "300")
	assertArgValue(t, args, "--passwords", "pass1,pass2,pass3")
}

func TestRelaySubTask_ToShellArgs_NoPasswords(t *testing.T) {
	task := &RelaySubTask{
		SourceNodeID: "source-1",
		Targets: []RelayTarget{
			{Host: "root@10.0.0.1:/tmp/file.tar.gz", Password: ""},
			{Host: "root@10.0.0.2:/tmp/file.tar.gz", Password: ""},
		},
		SourceFile: "/data/file.tar.gz",
		TimeoutSec: 120,
	}

	args := task.ToShellArgs()

	for _, arg := range args {
		if strings.HasPrefix(arg, "--passwords") {
			t.Fatalf("--passwords should not appear when no targets have passwords, but found: %s", arg)
		}
	}

	assertArgValue(t, args, "--source", "/data/file.tar.gz")
	assertArgValue(t, args, "--targets", "root@10.0.0.1:/tmp/file.tar.gz,root@10.0.0.2:/tmp/file.tar.gz")
	assertArgValue(t, args, "--timeout", "120")
}

func TestRelaySubTask_ToShellArgs_MixedPasswords(t *testing.T) {
	task := &RelaySubTask{
		SourceNodeID: "source-1",
		Targets: []RelayTarget{
			{Host: "root@10.0.0.1:/tmp/file.tar.gz", Password: "pass1"},
			{Host: "root@10.0.0.2:/tmp/file.tar.gz", Password: ""},
			{Host: "root@10.0.0.3:/tmp/file.tar.gz", Password: "pass3"},
		},
		SourceFile: "/data/file.tar.gz",
		TimeoutSec: 300,
	}

	args := task.ToShellArgs()

	assertArgValue(t, args, "--source", "/data/file.tar.gz")
	assertArgValue(t, args, "--targets", "root@10.0.0.1:/tmp/file.tar.gz,root@10.0.0.2:/tmp/file.tar.gz,root@10.0.0.3:/tmp/file.tar.gz")
	assertArgValue(t, args, "--timeout", "300")
	assertArgValue(t, args, "--passwords", "pass1,,pass3")
}

func TestRelaySubTask_ToShellArgs_SingleTarget(t *testing.T) {
	task := &RelaySubTask{
		SourceNodeID: "source-1",
		Targets: []RelayTarget{
			{Host: "root@10.0.0.1:/tmp/file.tar.gz", Password: "secret"},
		},
		SourceFile: "/data/file.tar.gz",
		TimeoutSec: 60,
	}

	args := task.ToShellArgs()

	assertArgValue(t, args, "--source", "/data/file.tar.gz")
	assertArgValue(t, args, "--targets", "root@10.0.0.1:/tmp/file.tar.gz")
	assertArgValue(t, args, "--timeout", "60")
	assertArgValue(t, args, "--passwords", "secret")
}

func assertArgValue(t *testing.T, args []string, flag string, expectedValue string) {
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

func TestParseRelayResults_Success(t *testing.T) {
	csvData := `target,status,error,duration_ms
root@10.0.0.1:/tmp/file.tar.gz,success,,1523
root@10.0.0.2:/tmp/file.tar.gz,failed,Permission denied,3000
`

	results, err := ParseRelayResults(csvData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	r1 := results[0]
	if r1.Target != "root@10.0.0.1:/tmp/file.tar.gz" {
		t.Errorf("target mismatch: got '%s'", r1.Target)
	}
	if r1.Status != "success" {
		t.Errorf("status mismatch: got '%s'", r1.Status)
	}
	if r1.Error != "" {
		t.Errorf("error mismatch: got '%s'", r1.Error)
	}
	if r1.DurationMs != 1523 {
		t.Errorf("duration_ms mismatch: got %d", r1.DurationMs)
	}

	r2 := results[1]
	if r2.Target != "root@10.0.0.2:/tmp/file.tar.gz" {
		t.Errorf("target mismatch: got '%s'", r2.Target)
	}
	if r2.Status != "failed" {
		t.Errorf("status mismatch: got '%s'", r2.Status)
	}
	if r2.Error != "Permission denied" {
		t.Errorf("error mismatch: got '%s'", r2.Error)
	}
	if r2.DurationMs != 3000 {
		t.Errorf("duration_ms mismatch: got %d", r2.DurationMs)
	}
}

func TestParseRelayResults_Empty(t *testing.T) {
	results, err := ParseRelayResults("")
	if err != nil {
		t.Fatalf("unexpected error for empty string: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil result for empty string, got %v", results)
	}
}

func TestParseRelayResults_WhitespaceOnly(t *testing.T) {
	_, err := ParseRelayResults("   \n  \n  ")
	if err == nil {
		t.Fatal("expected error for whitespace-only input (CSV reader treats whitespace lines as single-column records)")
	}
}

func TestParseRelayResults_InvalidHeader_WrongCount(t *testing.T) {
	csvData := `target,status,error
row1,success,,100
`

	_, err := ParseRelayResults(csvData)
	if err == nil {
		t.Fatal("expected error for wrong header column count, got nil")
	}
}

func TestParseRelayResults_InvalidHeader_WrongNames(t *testing.T) {
	csvData := `target,statuss,error,duration_ms
root@10.0.0.1,success,,100
`

	_, err := ParseRelayResults(csvData)
	if err == nil {
		t.Fatal("expected error for wrong header column names, got nil")
	}
}

func TestParseRelayResults_SkipMalformedRows_WrongColumnCount(t *testing.T) {
	csvData := `target,status,error,duration_ms
root@10.0.0.1:/tmp/f1,success,,100
root@10.0.0.2:/tmp/f2,failed
root@10.0.0.3:/tmp/f3,success,,300
`

	results, err := ParseRelayResults(csvData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result (csv.Reader with default FieldsPerRecord stops on mismatched field count), got %d", len(results))
	}

	if results[0].Target != "root@10.0.0.1:/tmp/f1" {
		t.Errorf("target[0] mismatch: got '%s'", results[0].Target)
	}
	if results[0].DurationMs != 100 {
		t.Errorf("duration[0] mismatch: got %d", results[0].DurationMs)
	}
}

func TestParseRelayResults_SkipMalformedRows_UnparseableDuration(t *testing.T) {
	csvData := `target,status,error,duration_ms
root@10.0.0.1:/tmp/f1,success,,100
root@10.0.0.2:/tmp/f2,failed,timeout,abc
root@10.0.0.3:/tmp/f3,success,,300
`

	results, err := ParseRelayResults(csvData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results (1 unparseable row skipped), got %d", len(results))
	}

	if results[0].DurationMs != 100 {
		t.Errorf("duration[0] mismatch: got %d", results[0].DurationMs)
	}
	if results[1].DurationMs != 300 {
		t.Errorf("duration[1] mismatch: got %d", results[1].DurationMs)
	}
}

func TestParseRelayResults_SpecialCharacters_Commas(t *testing.T) {
	csvData := `target,status,error,duration_ms
"root@10.0.0.1,user:/tmp/file",success,"error, with comma",100
`

	results, err := ParseRelayResults(csvData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.Target != "root@10.0.0.1,user:/tmp/file" {
		t.Errorf("target mismatch: got '%s'", r.Target)
	}
	if r.Error != "error, with comma" {
		t.Errorf("error mismatch: got '%s'", r.Error)
	}
	if r.DurationMs != 100 {
		t.Errorf("duration_ms mismatch: got %d", r.DurationMs)
	}
}

func TestParseRelayResults_SpecialCharacters_DoubleQuotes(t *testing.T) {
	csvData := `target,status,error,duration_ms
root@10.0.0.1:/tmp/file,"""success""",,100
`

	results, err := ParseRelayResults(csvData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.Status != `"success"` {
		t.Errorf("status mismatch: got '%s'", r.Status)
	}
}

func TestParseRelayResults_EmptyHeader(t *testing.T) {
	_, err := ParseRelayResults("")
	if err != nil {
		t.Fatalf("unexpected error for empty input: %v", err)
	}

	_, err = ParseRelayResults("\n")
	if err == nil {
		t.Fatal("expected error for newline-only input (no valid header)")
	}
}

func TestParseRelayResults_HeaderOnly(t *testing.T) {
	csvData := `target,status,error,duration_ms
`

	results, err := ParseRelayResults(csvData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results for header-only input, got %d", len(results))
	}
}

func TestParseRelayResults_EmptyFields(t *testing.T) {
	csvData := `target,status,error,duration_ms
root@10.0.0.1,,,0
`

	results, err := ParseRelayResults(csvData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.Target != "root@10.0.0.1" {
		t.Errorf("target mismatch: got '%s'", r.Target)
	}
	if r.Status != "" {
		t.Errorf("status should be empty, got '%s'", r.Status)
	}
	if r.Error != "" {
		t.Errorf("error should be empty, got '%s'", r.Error)
	}
	if r.DurationMs != 0 {
		t.Errorf("duration_ms mismatch: got %d", r.DurationMs)
	}
}

func TestParseRelayResults_DurationMsInt64(t *testing.T) {
	csvData := `target,status,error,duration_ms
root@10.0.0.1:/tmp/f,success,,9223372036854775807
`

	results, err := ParseRelayResults(csvData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].DurationMs != 9223372036854775807 {
		t.Errorf("expected max int64, got %d", results[0].DurationMs)
	}
}
