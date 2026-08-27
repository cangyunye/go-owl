package monitor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRemedyRunStore_CreateAndGet 验证执行计划落库与整单读取（含步骤）。
func TestRemedyRunStore_CreateAndGet(t *testing.T) {
	s := newTestStore(t)

	run := &RemedyRun{
		ID: "RUN-1", AlertID: "AL-1", NodeID: "node-a",
		Status: RunPending, StopOnError: true, CreatedBy: "admin",
		CreatedAt: 1000, UpdatedAt: 1000,
		Steps: []RemedyStep{
			{Order: 0, RemedyID: "RM-1", Name: "清理临时文件", Kind: "script",
				Content: "find /tmp -type f -mtime +7 -delete", Status: StepPending},
			{Order: 1, RemedyID: "RM-2", Name: "磁盘排查指引", Kind: "sop",
				Content: "1. du -h --max-depth=1 /", Status: StepPending},
		},
	}
	require.NoError(t, s.CreateRemedyRun(run))

	got, exists, err := s.GetRemedyRun("RUN-1")
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, "node-a", got.NodeID)
	require.Equal(t, RunPending, got.Status)
	require.Len(t, got.Steps, 2)
	require.Equal(t, "清理临时文件", got.Steps[0].Name)
	require.Equal(t, "script", got.Steps[0].Kind)
	require.Equal(t, "sop", got.Steps[1].Kind)

	_, exists, err = s.GetRemedyRun("RUN-NOPE")
	require.NoError(t, err)
	require.False(t, exists)
}

// TestRemedyRunStore_UpdateProgress 验证执行过程中的状态推进。
func TestRemedyRunStore_UpdateProgress(t *testing.T) {
	s := newTestStore(t)
	run := &RemedyRun{
		ID: "RUN-1", AlertID: "AL-1", NodeID: "node-a", Status: RunRunning,
		CreatedAt: 1000, UpdatedAt: 1000,
		Steps: []RemedyStep{
			{Order: 0, RemedyID: "RM-1", Name: "清理", Kind: "script", Content: "echo hi", Status: StepPending},
		},
	}
	require.NoError(t, s.CreateRemedyRun(run))

	// 步骤成功推进
	require.NoError(t, s.UpdateRemedyStep("RUN-1", 0, func(st *RemedyStep) {
		st.Status = StepSuccess
		st.ExitCode = 0
		st.Output = "ok"
		st.StartedAt = 1001
		st.FinishedAt = 1002
	}))
	// 整单完成
	require.NoError(t, s.UpdateRemedyRunStatus("RUN-1", RunDone))

	got, _, _ := s.GetRemedyRun("RUN-1")
	require.Equal(t, RunDone, got.Status)
	require.Equal(t, StepSuccess, got.Steps[0].Status)
	require.Equal(t, 0, got.Steps[0].ExitCode)
	require.Equal(t, "ok", got.Steps[0].Output)
	require.Equal(t, int64(1002), got.Steps[0].FinishedAt)
}

// TestRemedyRunStore_ListByAlert 验证按告警查询执行历史。
func TestRemedyRunStore_ListByAlert(t *testing.T) {
	s := newTestStore(t)
	for _, id := range []string{"RUN-1", "RUN-2"} {
		require.NoError(t, s.CreateRemedyRun(&RemedyRun{
			ID: id, AlertID: "AL-1", NodeID: "node-a", Status: RunDone,
			CreatedAt: 1000, UpdatedAt: 1000,
		}))
	}
	runs, err := s.ListRemedyRunsByAlert("AL-1")
	require.NoError(t, err)
	require.Len(t, runs, 2)

	runs, err = s.ListRemedyRunsByAlert("AL-OTHER")
	require.NoError(t, err)
	require.Empty(t, runs)
}
