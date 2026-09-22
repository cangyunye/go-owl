package monitor

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestSplitCompositeOutput_Protocol 验证复合输出按分隔符协议切分：
// 每段以 __OWL_SECTION__<name>__ 开始、以 __OWL_RC__<name>__<rc> 结束，
// 段内容原样保留，rc 之外的多余尾部（如整段命令的 stderr 追加）被忽略。
func TestSplitCompositeOutput_Protocol(t *testing.T) {
	raw := strings.Join([]string{
		"__OWL_SECTION__loadavg__",
		"0.52 0.47 0.41 2/345 12345",
		"__OWL_RC__loadavg__0",
		"__OWL_SECTION__nproc__",
		"8",
		"__OWL_RC__nproc__0",
		"__OWL_SECTION__oom__",
		"0",
		"__OWL_RC__oom__0",
	}, "\n")

	sections := splitCompositeOutput(raw, collectSteps)
	require.Len(t, sections, 3)
	require.Equal(t, "0.52 0.47 0.41 2/345 12345", sections["loadavg"].output)
	require.Equal(t, 0, sections["loadavg"].rc)
	require.True(t, sections["loadavg"].rcPresent)
	require.Equal(t, "8", sections["nproc"].output)
	require.Equal(t, "0", sections["oom"].output)
}

// TestSplitCompositeOutput_MissingSection 验证段缺失/无 rc 标记时的降级：
// 缺失段不出现在结果中，出现但被截断（无 rc 行）的段 rcPresent=false。
func TestSplitCompositeOutput_MissingSection(t *testing.T) {
	// oom 段整体缺失（sh 中途被杀）
	raw := strings.Join([]string{
		"__OWL_SECTION__loadavg__",
		"0.1 0.2 0.3 1/1 1",
		"__OWL_RC__loadavg__0",
	}, "\n")
	sections := splitCompositeOutput(raw, collectSteps)
	require.Len(t, sections, 1)
	require.Contains(t, sections, "loadavg")
	require.NotContains(t, sections, "oom")

	// nproc 段有内容但 rc 行缺失（输出被截断）
	raw = strings.Join([]string{
		"__OWL_SECTION__loadavg__",
		"0.1 0.2 0.3 1/1 1",
		"__OWL_RC__loadavg__0",
		"__OWL_SECTION__nproc__",
		"8",
	}, "\n")
	sections = splitCompositeOutput(raw, collectSteps)
	require.True(t, sections["loadavg"].rcPresent)
	require.False(t, sections["nproc"].rcPresent, "rc 行缺失应标记为 rc 未知")
	require.Equal(t, "8", sections["nproc"].output)
}

// TestSplitCompositeOutput_GluedRCMarker 验证命令输出无换行结尾时
// rc 标记与内容粘在同一行（POSIX echo 直接续写）也能正确切分。
func TestSplitCompositeOutput_GluedRCMarker(t *testing.T) {
	raw := "__OWL_SECTION__nproc__\n8__OWL_RC__nproc__0"
	sections := splitCompositeOutput(raw, collectSteps)
	require.Equal(t, "8", sections["nproc"].output)
	require.Equal(t, 0, sections["nproc"].rc)
}

// TestSplitCompositeOutput_TrailingStderrIgnored 验证整个复合命令的 stderr
// （native executor 追加在全部输出末尾）不会污染任何段。
func TestSplitCompositeOutput_TrailingStderrIgnored(t *testing.T) {
	raw := strings.Join([]string{
		"__OWL_SECTION__loadavg__",
		"0.52 0.47 0.41 2/345 12345",
		"__OWL_RC__loadavg__0",
		"__OWL_SECTION__nproc__",
		"",
		"__OWL_RC__nproc__127",
		"sh: nproc: command not found", // 复合命令整体 stderr 尾巴
	}, "\n")

	sections := splitCompositeOutput(raw, collectSteps)
	require.Equal(t, "0.52 0.47 0.41 2/345 12345", sections["loadavg"].output)
	require.True(t, sections["nproc"].rcPresent, "rc 标记 127 存在，应正常识别为失败退出码")
	require.Equal(t, 127, sections["nproc"].rc)
}

// TestBuildCompositeCommand_ShCompatible 验证复合命令在 /bin/sh（POSIX）下
// 真实可执行且分段正确：不用 bash-ism，marker 行与 rc 行按协议输出。
func TestBuildCompositeCommand_ShCompatible(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell 不可用")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh 不可用")
	}

	steps := []collectStep{
		{name: "s1", command: "echo hello"},
		{name: "s2", command: "printf 'no-newline'"},
		{name: "s3", command: "false"},
	}
	cmd := buildCompositeCommand(steps)

	out, err := exec.Command("sh", "-c", cmd).Output()
	require.NoError(t, err) // 段内命令失败不退出 shell，复合命令整体 rc = 末尾 echo 的 0

	sections := splitCompositeOutput(string(out), steps)
	require.Equal(t, "hello", sections["s1"].output)
	require.Equal(t, "no-newline", sections["s2"].output, "无换行结尾的输出应与 rc 标记正确分离")
	require.Equal(t, 1, sections["s3"].rc, "段内命令退出码应经 $? 透传")
	require.True(t, sections["s3"].rcPresent)
}

// TestCollector_CompositeParityWithSequential 核心等价性断言：
// 「复合命令切分后喂解析器」与「逐条执行后喂解析器」产出完全相同的 Metrics。
func TestCollector_CompositeParityWithSequential(t *testing.T) {
	ts := int64(1750000000)
	outputs := sampleOutputs()

	// 参照实现：逐条执行（原 Collect 路径）
	var want []Sample
	for _, step := range collectSteps {
		raw := outputs[step.command]
		parsed, err := step.parse(raw, "node-a", ts)
		require.NoError(t, err, "step %s", step.name)
		want = append(want, parsed...)
	}

	// 复合路径：一次执行 + 切分 + 同一解析器
	exec := &fakeExecer{outputs: outputs}
	c := NewCollector(&fakeFactory{exec: exec})
	c.now = func() int64 { return ts }
	got, err := c.Collect(context.Background(), &Target{ID: "node-a"})
	require.NoError(t, err)

	require.Len(t, got, len(want))
	require.ElementsMatch(t, want, got, "复合采集与逐条采集的指标必须完全一致")
}

// TestCollector_CompositeSingleExecution 验证固定命令表只产生一次 SSH 执行
// （svc 为 opt-in 保持单独执行）：这是本轮优化的核心目标。
func TestCollector_CompositeSingleExecution(t *testing.T) {
	exec := &fakeExecer{outputs: sampleOutputs()}
	c := NewCollector(&fakeFactory{exec: exec})
	c.now = func() int64 { return 1750000000 }

	_, err := c.Collect(context.Background(), &Target{ID: "node-a"})
	require.NoError(t, err)
	require.Len(t, exec.executed, 1, "无 svc 时应只执行 1 条复合命令, got %v", exec.executed)

	// 有 svc：复合命令 1 条 + systemctl 1 条
	exec2 := &fakeExecer{outputs: sampleOutputs()}
	c2 := NewCollector(&fakeFactory{exec: exec2})
	c2.now = func() int64 { return 1750000000 }
	_, err = c2.Collect(context.Background(), &Target{ID: "node-a", Services: []string{"cron"}})
	require.NoError(t, err)
	require.Len(t, exec2.executed, 2, "有 svc 时应为复合命令 + systemctl 共 2 次执行")
}

// TestCollector_CompositeStepFailureParity 验证段内失败语义与逐条执行一致：
// 非必选命令失败 → 整体返回错误但其余指标照常产出（错误含失败命令）；
// 必选命令（loadavg）失败 → 整体失败；optional 命令失败 → 静默跳过。
func TestCollector_CompositeStepFailureParity(t *testing.T) {
	// ss（非必选）失败
	exec := &fakeExecer{outputs: sampleOutputs(), fail: map[string]bool{"LC_ALL=C ss -s": true}}
	c := NewCollector(&fakeFactory{exec: exec})
	c.now = func() int64 { return 1750000000 }
	samples, err := c.Collect(context.Background(), &Target{ID: "node-a"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "ss -s", "错误应携带失败命令名")
	byMetric := map[string]float64{}
	for _, s := range samples {
		byMetric[s.Metric] = s.Value
	}
	require.InDelta(t, 45, byMetric["disk.usage./"], 0.001)
	_, hasTCP := byMetric["net.tcp_estab"]
	require.False(t, hasTCP, "ss 段失败时不应产出 tcp 指标")

	// loadavg（必选）失败 → 整体失败
	exec2 := &fakeExecer{outputs: sampleOutputs(), fail: map[string]bool{"cat /proc/loadavg": true}}
	c2 := NewCollector(&fakeFactory{exec: exec2})
	c2.now = func() int64 { return 1750000000 }
	_, err = c2.Collect(context.Background(), &Target{ID: "node-a"})
	require.Error(t, err)

	// oom（optional）失败 → 静默，无错误
	exec3 := &fakeExecer{
		outputs: sampleOutputs(),
		fail:    map[string]bool{journalOOMCmd: true, journalErrorsCmd: true},
	}
	c3 := NewCollector(&fakeFactory{exec: exec3})
	c3.now = func() int64 { return 1750000000 }
	samples3, err := c3.Collect(context.Background(), &Target{ID: "node-a"})
	require.NoError(t, err, "仅 optional 段失败不应返回错误")
	for _, s := range samples3 {
		require.False(t, strings.HasPrefix(s.Metric, "err."), "optional 段失败不应产出 err 指标")
	}
}

// TestCollector_CompositeSectionMissing 验证复合输出被截断（段缺失）时：
// 缺失段按执行失败处理；必选段缺失 → 整体失败。
func TestCollector_CompositeSectionMissing(t *testing.T) {
	exec := &truncatedExecer{}
	c := NewCollector(&fakeFactory{exec: exec})
	c.now = func() int64 { return 1750000000 }

	_, err := c.Collect(context.Background(), &Target{ID: "node-a"})
	require.Error(t, err, "必选段 loadavg 缺失应整体失败")
	require.Contains(t, err.Error(), "cat /proc/loadavg", "错误应携带必选命令名")
}

// truncatedExecer 返回只有 nproc 段的截断复合输出。
type truncatedExecer struct{}

func (t *truncatedExecer) Execute(command string, timeout time.Duration) (int, string, error) {
	if strings.Contains(command, sectionMarkerPrefix) {
		return 0, "__OWL_SECTION__nproc__\n8\n__OWL_RC__nproc__0\n", nil
	}
	return 0, "", nil
}
