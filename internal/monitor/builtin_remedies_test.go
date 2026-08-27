package monitor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestStore_SeedBuiltinRemedies 验证首次打开自动写入内置对策且幂等，
// 且每条内置对策都能匹配到对应告警类型。
func TestStore_SeedBuiltinRemedies(t *testing.T) {
	s := newTestStore(t)

	recs, err := s.ListRemedies("")
	require.NoError(t, err)
	require.Len(t, recs, len(BuiltinRemedies()), "内置对策应全部落库")

	require.NoError(t, s.SeedBuiltinRemediesIfEmpty())
	recs, err = s.ListRemedies("")
	require.NoError(t, err)
	require.Len(t, recs, len(BuiltinRemedies()), "再次 seed 不重复")

	// 内置对策引用的告警类型必须存在
	for _, r := range recs {
		_, ok, err := s.GetAlertType(r.AlertTypeID)
		require.NoError(t, err)
		require.True(t, ok, "内置对策 %s 引用的告警类型 %s 不存在", r.ID, r.AlertTypeID)
	}
}

// TestBuiltinRemedies_AllSOP 验证内置对策均为低风险 SOP（安全）。
func TestBuiltinRemedies_AllSOP(t *testing.T) {
	for _, r := range BuiltinRemedies() {
		require.Equal(t, "sop", r.Kind, "内置对策 %s 应为 SOP 类型", r.ID)
		require.Equal(t, "builtin", r.Source)
		require.True(t, r.Reviewed)
		require.Contains(t, []string{"low", "medium"}, r.Risk, "内置对策风险不应为 high")
	}
}
