package monitor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func sampleRemedy(id, alertTypeID string, source string, reviewed bool) Remedy {
	return Remedy{
		ID: id, AlertTypeID: alertTypeID, Name: "对策-" + id,
		Kind: "sop", Content: "人工排查步骤...", Risk: "low",
		Source: source, Reviewed: reviewed,
		CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix(),
	}
}

// TestRemedy_CRUD 验证对策库增删改查。
func TestRemedy_CRUD(t *testing.T) {
	s := newTestStore(t)

	r := sampleRemedy("RM-1", "OWL-DSK-001", "user", true)
	r.Kind = "script"
	r.Content = "rm -rf /var/tmp/old/*"
	r.Risk = "medium"
	r.Rollback = "echo rollback"
	require.NoError(t, s.UpsertRemedy(r))

	got, exists, err := s.GetRemedy("RM-1")
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, "script", got.Kind)
	require.Equal(t, "medium", got.Risk)
	require.Equal(t, "echo rollback", got.Rollback)

	// 更新
	r.Name = "新名称"
	require.NoError(t, s.UpsertRemedy(r))
	got, _, _ = s.GetRemedy("RM-1")
	require.Equal(t, "新名称", got.Name)

	// 删除
	require.NoError(t, s.DeleteRemedy("RM-1"))
	_, exists, err = s.GetRemedy("RM-1")
	require.NoError(t, err)
	require.False(t, exists)
}

// TestRemedy_BuiltinNotDeletable 验证内置对策不可删除。
func TestRemedy_BuiltinNotDeletable(t *testing.T) {
	s := newTestStore(t)
	r := sampleRemedy("RM-B", "OWL-OSS-001", "builtin", true)
	require.NoError(t, s.UpsertRemedy(r))
	require.Error(t, s.DeleteRemedy("RM-B"), "内置对策不可删除")

	require.NoError(t, s.DeleteRemedy("RM-不存在"))
}

// TestRemedy_Recommended_UserPriority 验证推荐排序：user 优先于 builtin/AI，
// AI 未审核不入推荐。
func TestRemedy_Recommended_UserPriority(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().Unix()

	// 选用无内置对策的告警类型 OWL-NET-002，避免种子干扰断言
	builtin := sampleRemedy("RM-B", "OWL-NET-002", "builtin", true)
	user := sampleRemedy("RM-U", "OWL-NET-002", "user", true)
	aiReviewed := sampleRemedy("RM-A1", "OWL-NET-002", "ai", true)
	aiPending := sampleRemedy("RM-A2", "OWL-NET-002", "ai", false) // 未审核
	for _, r := range []Remedy{builtin, user, aiReviewed, aiPending} {
		r.CreatedAt = now
		r.UpdatedAt = now
		require.NoError(t, s.UpsertRemedy(r))
	}

	recs, err := s.RecommendedRemedies("OWL-NET-002")
	require.NoError(t, err)
	require.Equal(t, []string{"RM-U", "RM-B", "RM-A1"}, ids(recs), "user 优先，AI 未审核排除")
}

// TestRemedy_Recommended_RiskOrder 验证同级来源内低风险优先。
func TestRemedy_Recommended_RiskOrder(t *testing.T) {
	s := newTestStore(t)

	high := sampleRemedy("RM-H", "OWL-ERR-001", "builtin", true)
	high.Risk = "high"
	low := sampleRemedy("RM-L", "OWL-ERR-001", "builtin", true)
	low.Risk = "low"
	require.NoError(t, s.UpsertRemedy(high))
	require.NoError(t, s.UpsertRemedy(low))

	recs, err := s.RecommendedRemedies("OWL-ERR-001")
	require.NoError(t, err)
	require.Equal(t, "RM-L", recs[0].ID, "低风险对策优先")
}

// TestRemedy_RecordExecution 验证执行反馈闭环：计数累加。
func TestRemedy_RecordExecution(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.UpsertRemedy(sampleRemedy("RM-1", "OWL-DSK-001", "user", true)))

	require.NoError(t, s.RecordExecution("RM-1", true))
	require.NoError(t, s.RecordExecution("RM-1", false))
	require.NoError(t, s.RecordExecution("RM-1", true))

	got, _, _ := s.GetRemedy("RM-1")
	require.Equal(t, 3, got.ExecCount)
	require.Equal(t, 2, got.SuccessCount)
}

// TestCanAutoExecute 验证自动执行门槛：类型放行 AND 对策放行，
// AI 来源必须已审核。默认全关。
func TestCanAutoExecute(t *testing.T) {
	at := AlertType{ID: "OWL-DSK-001", AutoApprove: false}
	r := Remedy{ID: "RM-1", Source: "builtin", AutoApprove: true, Reviewed: true}
	require.False(t, CanAutoExecute(at, r), "类型未放行时禁止自动执行")

	at.AutoApprove = true
	require.True(t, CanAutoExecute(at, r))

	r.AutoApprove = false
	require.False(t, CanAutoExecute(at, r), "对策未放行时禁止自动执行")

	r.AutoApprove = true
	r.Source = "ai"
	r.Reviewed = false
	require.False(t, CanAutoExecute(at, r), "AI 对策未审核禁止自动执行")

	r.Reviewed = true
	require.True(t, CanAutoExecute(at, r))
}

func ids(recs []Remedy) []string {
	out := make([]string, len(recs))
	for i, r := range recs {
		out[i] = r.ID
	}
	return out
}
