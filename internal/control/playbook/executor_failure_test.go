package playbook

import (
	"testing"
	"time"
)

// R2：失败判定曾有两套口径并存——终态判定要求 ExitCode!=0 && Error!=nil，
// FailureCount 只看 ExitCode!=0。这正是 946542b 类「误判成功」的温床。
// 统一为单一谓词：非零退出码即失败（shell 语义，Error 是补充信息）。

func TestApplyTerminalStatus_NonZeroExitIsFailure(t *testing.T) {
	exec := &PlaybookExecution{
		Status: ExecutionStatusRunning,
		Results: map[string][]*TaskResult{
			"t1": {{TaskName: "t1", NodeID: "n1", ExitCode: 1, StartTime: time.Now(), EndTime: time.Now()}},
		},
	}
	exec.applyTerminalStatus()
	if exec.Status != ExecutionStatusFailed {
		t.Fatalf("非零退出码应判失败（统一口径），实际 %s", exec.Status)
	}
}

func TestApplyTerminalStatus_AllZeroExitCompleted(t *testing.T) {
	exec := &PlaybookExecution{
		Status: ExecutionStatusRunning,
		Results: map[string][]*TaskResult{
			"t1": {{TaskName: "t1", NodeID: "n1", ExitCode: 0}},
		},
	}
	exec.applyTerminalStatus()
	if exec.Status != ExecutionStatusCompleted {
		t.Fatalf("全部退出码为 0 应判完成，实际 %s", exec.Status)
	}
}

func TestFailureCount_UsesUnifiedPredicate(t *testing.T) {
	exec := &PlaybookExecution{
		Results: map[string][]*TaskResult{
			"t": {
				{ExitCode: 0},
				{ExitCode: 1},
				{ExitCode: 0, Error: errForTest("x")},
				{ExitCode: 2, Error: errForTest("y")},
			},
		},
	}
	if got := exec.FailureCount(); got != 2 {
		t.Fatalf("FailureCount 应为 2，实际 %d", got)
	}
	if got := exec.SuccessCount(); got != 2 {
		t.Fatalf("SuccessCount 应为 2，实际 %d", got)
	}
}

type testErr string

func (e testErr) Error() string { return string(e) }

func errForTest(s string) error { return testErr(s) }
