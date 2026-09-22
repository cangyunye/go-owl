package playbook

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pbexec "github.com/cangyunye/go-owl/internal/control/playbook"
	"github.com/cangyunye/go-owl/internal/i18n"
	pb "github.com/cangyunye/go-owl/pkg/playbook"
)

// goldenCanonicalSkeleton 是规范骨架的唯一期望字节。
// owl playbook 的四个骨架入口（scaffold / new / template create / template export）
// 在相同输入下必须产出与这里逐字节一致的内容。
const goldenCanonicalSkeleton = `# owl playbook 骨架 —— 由 owl playbook scaffold / new / template create / template export 共享生成
# 运行: owl playbook run <本文件> --nodes <节点> --dry-run
description: "TODO: 描述此 Playbook 的用途"
version: "1.0"
tags: []

parameters:
  - name: app_version
    description: "应用版本号"
    default: "latest"
    type: string
    required: false

tasks:
  - name: "TODO: 步骤名称"
    action: command
    args:
      cmd: echo "hello"
    # timeout: 300
    # retries: 3
`

// mustParseSkeleton 断言生成物可被真实执行 Parser 解析并通过语义校验。
func mustParseSkeleton(t *testing.T, content []byte) *pbexec.ParsedPlaybook {
	t.Helper()
	parser := pbexec.NewParser()
	parsed, err := parser.Parse(string(content))
	if err != nil {
		t.Fatalf("生成的骨架必须可被 pbexec Parser 解析: %v\n内容:\n%s", err, content)
	}
	if errs := parser.Validate(parsed); len(errs) > 0 {
		t.Fatalf("生成的骨架必须通过语义校验: %v", errs)
	}
	return parsed
}

// seedCleanHome 将 HOME 指向空临时目录，隔离用户模板目录。
func seedCleanHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

func TestGeneratePlaybookSkeletonCanonicalGolden(t *testing.T) {
	out, err := GeneratePlaybookSkeleton(SkeletonOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != goldenCanonicalSkeleton {
		t.Fatalf("规范骨架与金样不一致\ngot:\n%q\nwant:\n%q", string(out), goldenCanonicalSkeleton)
	}
	mustParseSkeleton(t, out)
}

func TestGeneratePlaybookSkeletonDeterministic(t *testing.T) {
	first, err := GeneratePlaybookSkeleton(SkeletonOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := GeneratePlaybookSkeleton(SkeletonOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("同一输入两次生成结果不一致:\n%q\nvs\n%q", string(first), string(second))
	}
}

func TestGeneratePlaybookSkeletonActionTypes(t *testing.T) {
	for _, tpl := range pb.GetActionTemplates() {
		t.Run(tpl.Name, func(t *testing.T) {
			out, err := GeneratePlaybookSkeleton(SkeletonOptions{ActionType: tpl.Name})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			s := string(out)
			if !strings.Contains(s, "action: "+tpl.Name) {
				t.Errorf("expected action %q in output:\n%s", tpl.Name, s)
			}
			for k := range tpl.Template {
				if !strings.Contains(s, "  "+k+":") && !strings.Contains(s, "      "+k+":") {
					t.Errorf("expected arg key %q in output:\n%s", k, s)
				}
			}
			// include 类型的示例任务引用占位剧本路径，pbexec Parser 会急切展开
			// include 并读取目标文件，因此含占位路径的 include 骨架无法独立解析
			// （与旧版 scaffold --type include 行为一致）；其余类型必须可解析。
			if tpl.Name != "include" {
				mustParseSkeleton(t, out)
			}
		})
	}
}

func TestGeneratePlaybookSkeletonUnknownType(t *testing.T) {
	_, err := GeneratePlaybookSkeleton(SkeletonOptions{ActionType: "nonexistent"})
	if err == nil {
		t.Fatal("unknown action type must fail")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention the unknown type, got: %v", err)
	}
}

func TestGeneratePlaybookSkeletonFullOptions(t *testing.T) {
	opts := SkeletonOptions{
		Description:   "部署示例",
		Version:       "2.1",
		ExecutionMode: "pipeline",
		Default: &pb.TemplateDefaultConfig{
			Groups: []string{"web", "db"},
			Tags:   []string{"deploy"},
		},
		Vars: map[string]interface{}{
			"env":    "prod",
			"region": "cn-1",
		},
		Tasks: []pb.TemplateTask{
			{Name: "打招呼", Action: "command", Args: map[string]interface{}{"cmd": "echo hi"}},
		},
	}
	out, err := GeneratePlaybookSkeleton(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)

	for _, want := range []string{
		"description: 部署示例",
		`version: "2.1"`,
		"execution_mode: pipeline",
		"default:",
		"groups: [web, db]",
		"vars:",
		"env: prod",
		"region: cn-1",
		"action: command",
		"echo hi",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, s)
		}
	}
	// 用户显式提供的任务不应带示例注释提示。
	if strings.Contains(s, "# retries: 3") {
		t.Errorf("user-provided tasks must not carry example hints:\n%s", s)
	}
	// vars 必须按键名排序渲染（env 在 region 之前）。
	if strings.Index(s, "env: prod") > strings.Index(s, "region: cn-1") {
		t.Errorf("vars must be rendered in sorted key order:\n%s", s)
	}

	parsed := mustParseSkeleton(t, out)
	if parsed.ExecutionMode != pbexec.ExecutionModePipeline {
		t.Errorf("expected pipeline mode, got %v", parsed.ExecutionMode)
	}

	// 同一 opts 再次生成必须逐字节一致。
	again, err := GeneratePlaybookSkeleton(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(out, again) {
		t.Fatalf("same options must render identical bytes")
	}
}

func TestGeneratePlaybookSkeletonInvalidExecutionMode(t *testing.T) {
	_, err := GeneratePlaybookSkeleton(SkeletonOptions{ExecutionMode: "bogus"})
	if err == nil {
		t.Fatal("invalid execution_mode must fail")
	}
}

// --- 四入口一致性 ---

func runScaffoldForTest(t *testing.T, args ...string) []byte {
	t.Helper()
	cmd := NewPlaybookScaffoldCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("scaffold execute: %v", err)
	}
	if errb.Len() > 0 {
		t.Fatalf("scaffold stderr: %s", errb.String())
	}
	return out.Bytes()
}

func runNewSkeletonForTest(t *testing.T, outPath string, vars []string) []byte {
	t.Helper()
	cmd := NewPlaybookNewCmd()
	pbNewFrom = CanonicalSkeletonName // flag 构造会重置包级变量，须在其后赋值
	pbNewVars = vars
	pbNewOutput = outPath

	runPlaybookNew(cmd, nil) // 失败路径会 os.Exit(1)，能返回即成功

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("new 未生成文件 %s: %v", outPath, err)
	}
	return data
}

func runTemplateCreateForTest(t *testing.T, outPath, stdinScript string) []byte {
	t.Helper()

	scriptFile := filepath.Join(t.TempDir(), "stdin.txt")
	if err := os.WriteFile(scriptFile, []byte(stdinScript), 0644); err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(scriptFile)
	if err != nil {
		t.Fatal(err)
	}
	swallow, err := os.Create(filepath.Join(t.TempDir(), "stdout.txt"))
	if err != nil {
		t.Fatal(err)
	}

	oldStdin, oldStdout := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = in, swallow
	defer func() {
		os.Stdin, os.Stdout = oldStdin, oldStdout
		_ = in.Close()
		_ = swallow.Close()
	}()

	cmd := NewPlaybookTemplateCreateCmd()
	playbookTemplateOutput = outPath // flag 构造会重置包级变量，须在其后赋值

	runPlaybookTemplate(cmd, nil) // 失败路径会 os.Exit(1)，能返回即成功

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("template create 未生成文件 %s: %v", outPath, err)
	}
	return data
}

func runTemplateExportSkeletonForTest(t *testing.T) []byte {
	t.Helper()
	cmd := NewPlaybookTemplateExportCmd()
	toDir := t.TempDir()
	templateExportTo = toDir // flag 构造会重置包级变量，须在其后赋值

	runTemplateExport(cmd, []string{CanonicalSkeletonName})

	data, err := os.ReadFile(filepath.Join(toDir, CanonicalSkeletonName+".yaml"))
	if err != nil {
		t.Fatalf("template export 未生成文件: %v", err)
	}
	return data
}

func TestSkeletonEntriesProduceIdenticalBytes(t *testing.T) {
	seedCleanHome(t)

	scaffoldBytes := runScaffoldForTest(t)
	newBytes := runNewSkeletonForTest(t, filepath.Join(t.TempDir(), "new.yaml"), nil)
	// 向导输入: 名称 demo，其余全部回车取默认，任务选择直接 q 退出 → 规范示例任务。
	createBytes := runTemplateCreateForTest(t,
		filepath.Join(t.TempDir(), "created.yaml"),
		"demo\n"+strings.Repeat("\n", 5)+"q\n")
	exportBytes := runTemplateExportSkeletonForTest(t)

	entries := map[string][]byte{
		"scaffold":        scaffoldBytes,
		"new":             newBytes,
		"template create": createBytes,
		"template export": exportBytes,
	}
	for name, got := range entries {
		if string(got) != goldenCanonicalSkeleton {
			t.Errorf("%s 产出与金样不一致:\n%q", name, string(got))
		}
		mustParseSkeleton(t, got)
	}
}

func TestNewSkeletonVarFlagsAreAcceptedAndKeptIdentical(t *testing.T) {
	seedCleanHome(t)

	// 规范骨架的 parameters 块为示例占位，无 {{ }} 引用点，
	// --var 仅做格式校验，不改变四入口一致的默认输出。
	got := runNewSkeletonForTest(t,
		filepath.Join(t.TempDir(), "new.yaml"),
		[]string{"app_version=9.9.9"})
	if string(got) != goldenCanonicalSkeleton {
		t.Errorf("expected unchanged canonical bytes with --var, got:\n%q", string(got))
	}
}

func TestTemplateCreateWizardTasksFlowThroughGenerator(t *testing.T) {
	seedCleanHome(t)

	outPath := filepath.Join(t.TempDir(), "created.yaml")
	// 向导输入: 名称 demo，其余默认，选择第 1 个 action（command）后退出。
	got := runTemplateCreateForTest(t, outPath,
		"demo\n"+strings.Repeat("\n", 5)+"1\nq\n")

	actionTemplates := pb.GetActionTemplates()
	argsCopy := make(map[string]interface{})
	for k, v := range actionTemplates[0].Template {
		argsCopy[k] = v
	}
	expected, err := GeneratePlaybookSkeleton(SkeletonOptions{
		Tasks: []pb.TemplateTask{{
			Name:   i18n.T("playbook.template.task_name", i18n.F(1)),
			Action: actionTemplates[0].Name,
			Args:   argsCopy,
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != string(expected) {
		t.Fatalf("wizard output != generator output with same tasks\ngot:\n%q\nwant:\n%q", string(got), string(expected))
	}
	mustParseSkeleton(t, got)
}

func TestTemplateExportSkeletonIsLoadableTemplate(t *testing.T) {
	seedCleanHome(t)

	got := runTemplateExportSkeletonForTest(t)

	// 导出物落回用户模板目录后必须能被模板体系加载（list/info/new 可用）。
	meta, err := pb.ParseTemplateMeta(got)
	if err != nil {
		t.Fatalf("exported skeleton must be a loadable template: %v", err)
	}
	if meta.Description == "" {
		t.Errorf("exported skeleton must carry description")
	}
	if len(meta.Parameters) != 1 || meta.Parameters[0].Name != "app_version" {
		t.Errorf("exported skeleton must carry the example parameter, got %+v", meta.Parameters)
	}
}
