package playbook

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pb "github.com/cangyunye/go-owl/pkg/playbook"
)

func promptTasksInput(input string) []pb.TemplateTask {
	return promptForTasks(bufio.NewReader(strings.NewReader(input)))
}

func TestPromptForTasks_SelectOneThenQuit(t *testing.T) {
	tasks := promptTasksInput("1\nq\n")

	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Action != "command" {
		t.Errorf("expected action 'command', got %q", tasks[0].Action)
	}
}

func TestPromptForTasks_MultipleThenQuit(t *testing.T) {
	tasks := promptTasksInput("1\n3\nq\n")

	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0].Action != "command" {
		t.Errorf("expected first action 'command', got %q", tasks[0].Action)
	}
	if tasks[1].Action != "upload" {
		t.Errorf("expected second action 'upload', got %q", tasks[1].Action)
	}
}

func TestPromptForTasks_QuitOnly(t *testing.T) {
	tasks := promptTasksInput("q\n")

	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestPromptForTasks_InvalidChoiceThenValid(t *testing.T) {
	tasks := promptTasksInput("999\n0\n2\nq\n")

	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	actionTemplates := pb.GetActionTemplates()
	if tasks[0].Action != actionTemplates[1].Name {
		t.Errorf("expected action %q, got %q", actionTemplates[1].Name, tasks[0].Action)
	}
}

func TestPromptForTasks_QuitAliases(t *testing.T) {
	tasks := promptTasksInput("1\nquit\n")

	if len(tasks) != 1 {
		t.Fatalf("expected 1 task via 'quit' alias, got %d", len(tasks))
	}
}

const e2eUserTemplateYAML = `description: e2e template
parameters:
  - name: who
    description: "who"
    default: "world"
tasks:
  - name: hi
    action: command
    args:
      cmd: echo hello
`

// 设置 HOME 指向临时目录并写入一个用户模板,供 CLI 命令测试读回。
func seedUserTemplate(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	dir := filepath.Join(tmp, ".owl", "templates")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "e2e-tpl.yaml"), []byte(e2eUserTemplateYAML), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRunTemplateList_ShowsUserTemplates(t *testing.T) {
	seedUserTemplate(t)
	cmd := NewPlaybookTemplateListCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)

	runTemplateList(cmd, nil)

	if !strings.Contains(out.String(), "e2e-tpl") {
		t.Fatalf("user template not listed:\n%s", out.String())
	}
	if strings.Contains(errb.String(), "err") {
		t.Fatalf("unexpected stderr: %s", errb.String())
	}
}

func TestRunTemplateInfo_UserTemplate(t *testing.T) {
	seedUserTemplate(t)
	cmd := NewPlaybookTemplateInfoCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)

	runTemplateInfo(cmd, []string{"e2e-tpl"})

	if !strings.Contains(out.String(), "e2e template") {
		t.Fatalf("user template info missing description:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "who") {
		t.Fatalf("user template info missing parameter:\n%s", out.String())
	}
}

func TestRunTemplateExport_UserTemplate(t *testing.T) {
	seedUserTemplate(t)
	cmd := NewPlaybookTemplateExportCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	toDir := t.TempDir()
	templateExportTo = toDir // 需在 NewPlaybookTemplateExportCmd() 之后赋值(flag 构造会重置该包级变量)

	runTemplateExport(cmd, []string{"e2e-tpl"})

	if _, err := os.Stat(filepath.Join(toDir, "e2e-tpl.yaml")); err != nil {
		t.Fatalf("export not written: %v; stderr=%s out=%s", err, errb.String(), out.String())
	}
}

func TestRunPlaybookNew_UserTemplateNonInteractive(t *testing.T) {
	seedUserTemplate(t)
	cmd := NewPlaybookNewCmd()
	pbNewFrom = "e2e-tpl" // 需在 NewPlaybookNewCmd() 之后赋值(flag 构造会重置这些包级变量)
	pbNewVars = nil
	pbNewOutput = filepath.Join(t.TempDir(), "out.yaml")

	runPlaybookNew(cmd, nil)

	data, err := os.ReadFile(pbNewOutput)
	if err != nil {
		t.Fatalf("output file not created: %v", err)
	}
	if !strings.Contains(string(data), "echo hello") {
		t.Errorf("unexpected instantiated content:\n%s", data)
	}
}

func TestPromptForMissingParams_NonInteractiveSkipsPrompt(t *testing.T) {
	params := []pb.TemplateParameter{
		{Name: "who", Description: "who", Default: "world"},
		{Name: "required", Description: "req", Required: true},
	}
	provided := map[string]interface{}{}

	// 非交互模式:即使 reader 立即 EOF 也不读 stdin、不报错
	promptForMissingParams(params, provided, strings.NewReader(""), false)

	if len(provided) != 0 {
		t.Errorf("non-interactive mode should not prompt, got %v", provided)
	}
}

func TestPromptForMissingParams_InteractiveAcceptsInput(t *testing.T) {
	params := []pb.TemplateParameter{{Name: "who", Description: "who"}}
	provided := map[string]interface{}{}

	promptForMissingParams(params, provided, strings.NewReader("owl\n"), true)

	if provided["who"] != "owl" {
		t.Errorf("expected who=owl, got %v", provided["who"])
	}
}

func TestPromptForMissingParams_InteractiveSkipsProvided(t *testing.T) {
	params := []pb.TemplateParameter{{Name: "who", Description: "who"}}
	provided := map[string]interface{}{"who": "already"}

	promptForMissingParams(params, provided, strings.NewReader(""), true)

	if provided["who"] != "already" {
		t.Errorf("provided value overwritten: %v", provided["who"])
	}
}

func TestPromptForMissingParams_InteractiveEmptyKeepsDefault(t *testing.T) {
	params := []pb.TemplateParameter{{Name: "who", Description: "who", Default: "world"}}
	provided := map[string]interface{}{}

	promptForMissingParams(params, provided, strings.NewReader("\n"), true)

	if len(provided) != 0 {
		t.Errorf("empty input should leave default application to ValidateParams, got %v", provided)
	}
}