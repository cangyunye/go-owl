package playbook

import (
	"bufio"
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