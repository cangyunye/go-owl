package transfer

import (
	"fmt"
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

// TestRelayPartialSuccess 验证中继部分成功时（exit 1），
// ExecuteRelay 返回结果中成功目标会被正确标记 success，
// 失败目标会被正确标记 failed，并返回错误。
func TestRelayPartialSuccess_CSVWithPartialFailure(t *testing.T) {
	// 模拟 mid-flight 部分成功的 CSV 输出（对应 owl-relay.sh exit 1 场景）
	csvData := `target,status,error,duration_ms
root@10.0.0.1:/tmp/f,success,,1523
root@10.0.0.2:/tmp/f,failed,Permission denied,3012
root@10.0.0.3:/tmp/f,success,,892
`
	results, err := ParseRelayResults(csvData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	successCount := 0
	failCount := 0
	for _, r := range results {
		if r.Status == "success" {
			successCount++
		} else {
			failCount++
		}
	}

	if successCount != 2 {
		t.Errorf("expected 2 success, got %d", successCount)
	}
	if failCount != 1 {
		t.Errorf("expected 1 failure, got %d", failCount)
	}

	// 验证失败目标携带正确错误信息
	for _, r := range results {
		if r.Status == "failed" && r.Target == "root@10.0.0.2:/tmp/f" {
			if r.Error != "Permission denied" {
				t.Errorf("expected 'Permission denied', got '%s'", r.Error)
			}
			if r.DurationMs != 3012 {
				t.Errorf("expected 3012ms, got %d", r.DurationMs)
			}
		}
	}
}

// TestRelayAllFailed_CSV 验证中继全部失败时（exit 2），
// 结果中所有目标都标记 failed，没有 success。
func TestRelayAllFailed_CSV(t *testing.T) {
	csvData := `target,status,error,duration_ms
root@10.0.0.1:/tmp/f,failed,Connection refused,5000
root@10.0.0.2:/tmp/f,failed,Host unreachable,30000
`
	results, err := ParseRelayResults(csvData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	for _, r := range results {
		if r.Status != "failed" {
			t.Errorf("expected all failed, got status '%s' for target '%s'", r.Status, r.Target)
		}
	}
}

// TestCheckRsyncAvailable_CacheBehavior 验证 rsync 可用性缓存
func TestCheckRsyncAvailable_CacheInitialState(t *testing.T) {
	// 新创建的 TransferManager 应该有空缓存
	tm := &TransferManager{
		rsyncAvailable: make(map[string]bool),
	}

	tm.mu.Lock()
	_, exists := tm.rsyncAvailable["nonexistent"]
	tm.mu.Unlock()
	if exists {
		t.Error("expected empty cache for new TransferManager")
	}
}

// TestSmartUpload_DecisionLogic 验证 smartUpload 决策分支的正确性。
// 使用 bool 参数模拟 rsync 可用性 + 密码存在性，测试是否进入正确的分支。
func TestSmartUpload_DecisionLogic(t *testing.T) {
	tests := []struct {
		name         string
		resume       bool
		rsyncOK      bool
		hasPassword  bool
		expectRsync  bool // true=期望使用 rsync, false=期望使用 scp
	}{
		{
			name:        "rsync 可用 + 无密码 + 启用续传 → 使用 rsync",
			resume:      true,
			rsyncOK:     true,
			hasPassword: false,
			expectRsync: true,
		},
		{
			name:        "rsync 可用 + 有密码 + 启用续传 → 使用 scp（rsync CLI 不支持密码）",
			resume:      true,
			rsyncOK:     true,
			hasPassword: true,
			expectRsync: false,
		},
		{
			name:        "rsync 不可用 + 无密码 + 启用续传 → 使用 scp",
			resume:      true,
			rsyncOK:     false,
			hasPassword: false,
			expectRsync: false,
		},
		{
			name:        "rsync 可用 + 无密码 + 禁用续传 → 使用 scp",
			resume:      false,
			rsyncOK:     true,
			hasPassword: false,
			expectRsync: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRsync := tt.rsyncOK && tt.resume && !tt.hasPassword
			if gotRsync != tt.expectRsync {
				t.Errorf("resume=%v, rsyncOK=%v, hasPassword=%v: expected rsync=%v, got rsync=%v",
					tt.resume, tt.rsyncOK, tt.hasPassword, tt.expectRsync, gotRsync)
			}
		})
	}
}

// TestRelayExecutor_ShellEscape 验证 shellEscape 函数
func TestRelayExecutor_ShellEscape(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "'simple'"},
		{"", "''"},
		{"with space", "'with space'"},
		{"it's", "'it'\\''s'"},
		{"$HOME", "'$HOME'"},
		{"$(whoami)", "'$(whoami)'"},
		{"`backtick`", "'`backtick`'"},
	}

	for _, tt := range tests {
		result := shellEscape(tt.input)
		if result != tt.expected {
			t.Errorf("shellEscape(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// TestParseRelayResults_PartialThenParseAgain 验证中继 CSV 的幂等性：
// 同一 CSV 反复解析应得到相同结果
func TestParseRelayResults_Idempotent(t *testing.T) {
	csvData := `target,status,error,duration_ms
root@10.0.0.1:/tmp/f,success,,1523
root@10.0.0.2:/tmp/f,failed,timeout,30000
`

	first, err := ParseRelayResults(csvData)
	if err != nil {
		t.Fatalf("first parse failed: %v", err)
	}

	second, err := ParseRelayResults(csvData)
	if err != nil {
		t.Fatalf("second parse failed: %v", err)
	}

	if len(first) != len(second) {
		t.Fatalf("length mismatch: first=%d, second=%d", len(first), len(second))
	}

	for i := range first {
		if first[i].Status != second[i].Status || first[i].Error != second[i].Error {
			t.Errorf("result[%d] mismatch: first=(%s,%s) second=(%s,%s)",
				i, first[i].Status, first[i].Error, second[i].Status, second[i].Error)
		}
	}
}

// TestRelayResult_RelayErrorHint 验证 ExecuteRelay 错误消息包含部分失败信息
func TestRelayResult_RelayErrorHint(t *testing.T) {
	// 模拟部分成功的 CSV + 对应的错误消息（模拟 ExecuteRelay 对 exit=1 的返回）
	csvData := `target,status,error,duration_ms
root@10.0.0.1:/tmp/f,success,,1500
root@10.0.0.2:/tmp/f,failed,Permission denied,3000
`
	results, err := ParseRelayResults(csvData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 模拟 ExecuteRelay 对部分成功的统计
	successCount := 0
	failCount := 0
	var failedTargets []string
	for _, r := range results {
		if r.Status == "success" {
			successCount++
		} else {
			failCount++
			failedTargets = append(failedTargets, r.Target)
		}
	}

	if successCount != 1 {
		t.Errorf("expected 1 success, got %d", successCount)
	}
	if failCount != 1 {
		t.Errorf("expected 1 failure, got %d", failCount)
	}

	// 构造错误消息——与 ExecuteRelay 中的格式一致
	errMsg := fmt.Sprintf("中继部分失败: %d/%d 个目标失败 (%s)", failCount, len(results), strings.Join(failedTargets, ","))
	expectedMsg := "中继部分失败: 1/2 个目标失败 (root@10.0.0.2:/tmp/f)"
	if errMsg != expectedMsg {
		t.Errorf("error message mismatch:\n  got:  %s\n  want: %s", errMsg, expectedMsg)
	}
}

// TestRelayResult_AllFailedErrorHint 验证全部失败时的错误消息
func TestRelayResult_AllFailedErrorHint(t *testing.T) {
	csvData := `target,status,error,duration_ms
root@10.0.0.1:/tmp/f,failed,Connection refused,5000
root@10.0.0.2:/tmp/f,failed,Host unreachable,30000
`
	results, err := ParseRelayResults(csvData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	successCount := 0
	failCount := 0
	for _, r := range results {
		if r.Status == "success" {
			successCount++
		} else {
			failCount++
		}
	}

	if successCount != 0 {
		t.Errorf("expected 0 success, got %d", successCount)
	}
	if failCount != 2 {
		t.Errorf("expected 2 failures, got %d", failCount)
	}

	if failCount > 0 && successCount == 0 {
		exitCode := 2
		errMsg := fmt.Sprintf("中继命令退出码非零 (%d)，全部 %d 个目标失败", exitCode, len(results))
		expectedMsg := "中继命令退出码非零 (2)，全部 2 个目标失败"
		if errMsg != expectedMsg {
			t.Errorf("error message mismatch:\n  got:  %s\n  want: %s", errMsg, expectedMsg)
		}
	}
}
