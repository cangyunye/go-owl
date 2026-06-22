package playbook_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/playbook"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/testutil"
)

func TestPlaybookCmdExists(t *testing.T) {
	parent := playbook.NewPlaybookCmd()

	if parent.Use != "playbook" {
		t.Errorf("expected Use 'playbook', got '%s'", parent.Use)
	}

	expected := []string{"list", "run", "validate", "template"}
	testutil.AssertSubCommands(t, parent, expected)
}

func TestPlaybookListFlags(t *testing.T) {
	cmd := playbook.NewPlaybookListCmd()

	testutil.AssertFlagExists(t, cmd, "library")
	testutil.AssertFlagDefault(t, cmd, "library", "./playbooks")

	testutil.AssertFlagExists(t, cmd, "output")
	testutil.AssertFlagShorthand(t, cmd, "output", "o")
	testutil.AssertFlagDefault(t, cmd, "output", "table")
}

func TestPlaybookRunFlags(t *testing.T) {
	cmd := playbook.NewPlaybookRunCmd()

	testutil.AssertFlagExists(t, cmd, "nodes")
	testutil.AssertFlagDefault(t, cmd, "nodes", "")

	testutil.AssertFlagExists(t, cmd, "group")
	testutil.AssertFlagDefault(t, cmd, "group", "")

	testutil.AssertFlagExists(t, cmd, "label")
	testutil.AssertFlagShorthand(t, cmd, "label", "l")

	testutil.AssertFlagExists(t, cmd, "tags")
	testutil.AssertFlagDefault(t, cmd, "tags", "")

	testutil.AssertFlagExists(t, cmd, "skip-tags")
	testutil.AssertFlagDefault(t, cmd, "skip-tags", "")

	testutil.AssertFlagExists(t, cmd, "extra-vars")
	testutil.AssertFlagExists(t, cmd, "check")
	testutil.AssertFlagDefault(t, cmd, "check", "false")

	testutil.AssertFlagExists(t, cmd, "default-connect-timeout")
	testutil.AssertFlagDefault(t, cmd, "default-connect-timeout", "10s")

	testutil.AssertFlagExists(t, cmd, "default-command-timeout")
	testutil.AssertFlagDefault(t, cmd, "default-command-timeout", "5m0s")

	testutil.AssertFlagExists(t, cmd, "default-retry")
	testutil.AssertFlagDefault(t, cmd, "default-retry", "0")

	testutil.AssertFlagExists(t, cmd, "default-retry-interval")
	testutil.AssertFlagDefault(t, cmd, "default-retry-interval", "1s")

	testutil.AssertFlagExists(t, cmd, "default-retry-max-interval")
	testutil.AssertFlagDefault(t, cmd, "default-retry-max-interval", "30s")
}

func TestPlaybookValidateCmd(t *testing.T) {
	cmd := playbook.NewPlaybookValidateCmd()

	if cmd.Use == "" {
		t.Error("expected non-empty Use for playbook validate")
	}
}

func TestPlaybookHelpContainsSubcommands(t *testing.T) {
	parent := playbook.NewPlaybookCmd()

	testutil.AssertHelpContains(t, parent, "list")
	testutil.AssertHelpContains(t, parent, "run")
	testutil.AssertHelpContains(t, parent, "validate")
	testutil.AssertHelpContains(t, parent, "template")
}

func TestPlaybookTemplateCmd(t *testing.T) {
	cmd := playbook.NewPlaybookTemplateCmd()

	if cmd.Use != "template" {
		t.Errorf("expected Use 'template', got '%s'", cmd.Use)
	}

	testutil.AssertFlagExists(t, cmd, "output")
	testutil.AssertFlagShorthand(t, cmd, "output", "o")
	testutil.AssertFlagDefault(t, cmd, "output", "")
}

func TestActionTemplatesCount(t *testing.T) {
	templates := playbook.GetActionTemplates()
	if len(templates) != 5 {
		t.Errorf("expected 5 action templates, got %d", len(templates))
	}

	expectedActions := []string{"command", "script", "upload", "download", "include"}
	for i, expected := range expectedActions {
		if templates[i].Name != expected {
			t.Errorf("expected action template[%d] name '%s', got '%s'", i, expected, templates[i].Name)
		}
	}
}

func TestPlaybookListInfoStruct(t *testing.T) {
	pb := playbook.PlaybookInfo{
		Name: "test.yml",
		Path: "/tmp/test.yml",
		Size: 100,
		Description: "测试描述",
		TasksCount:  5,
	}

	if pb.Name != "test.yml" {
		t.Errorf("expected Name 'test.yml', got '%s'", pb.Name)
	}
	if pb.Description != "测试描述" {
		t.Errorf("expected Description '测试描述', got '%s'", pb.Description)
	}
	if pb.TasksCount != 5 {
		t.Errorf("expected TasksCount 5, got %d", pb.TasksCount)
	}
}

func TestPlaybookListParsesMeta(t *testing.T) {
	tmpDir := t.TempDir()

	yamlContent := `name: test-playbook
description: 这是一个测试
hosts:
  - web
tasks:
  - name: task 1
    action: command
    args:
      cmd: echo hello
  - name: task 2
    action: command
    args:
      cmd: echo world
`
	yamlPath := filepath.Join(tmpDir, "test-playbook.yaml")
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	info := playbook.ReadPlaybookMeta(yamlPath)

	if info.Name != "test-playbook.yaml" {
		t.Errorf("expected Name 'test-playbook.yaml', got '%s'", info.Name)
	}
	if info.Description != "这是一个测试" {
		t.Errorf("expected Description '这是一个测试', got '%s'", info.Description)
	}
	if info.TasksCount != 2 {
		t.Errorf("expected TasksCount 2, got %d", info.TasksCount)
	}
	if info.Size <= 0 {
		t.Errorf("expected positive Size, got %d", info.Size)
	}
}

func TestPlaybookValidateWithGlob(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建有效文件
	validContent := `name: valid
hosts:
  - web
tasks:
  - name: task 1
    action: command
    args:
      cmd: echo hello
`
	validPath := filepath.Join(tmpDir, "valid.yaml")
	if err := os.WriteFile(validPath, []byte(validContent), 0644); err != nil {
		t.Fatalf("failed to write valid file: %v", err)
	}

	// 创建无效文件（执行模式错误）
	invalidContent := `name: invalid
hosts:
  - web
execution_mode: invalid_mode
tasks:
  - name: task 1
    action: command
`
	invalidPath := filepath.Join(tmpDir, "invalid.yaml")
	if err := os.WriteFile(invalidPath, []byte(invalidContent), 0644); err != nil {
		t.Fatalf("failed to write invalid file: %v", err)
	}

	// 测试单文件验证 - 有效
	results := playbook.ValidatePlaybookFiles([]string{validPath})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Valid {
		t.Errorf("expected valid file to be valid, got error: %v", results[0].Error)
	}

	// 测试单文件验证 - 无效
	results = playbook.ValidatePlaybookFiles([]string{invalidPath})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Valid {
		t.Error("expected invalid file to be invalid")
	}
	if results[0].Error == nil {
		t.Error("expected non-nil error for invalid file")
	}

	// 测试多文件验证
	results = playbook.ValidatePlaybookFiles([]string{validPath, invalidPath})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if !results[0].Valid {
		t.Errorf("expected first file (valid) to be valid")
	}
	if results[1].Valid {
		t.Errorf("expected second file (invalid) to be invalid")
	}

	// 测试不存在的文件
	results = playbook.ValidatePlaybookFiles([]string{"/nonexistent/path.yaml"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Valid {
		t.Error("expected nonexistent file to be invalid")
	}
	if results[0].Error == nil {
		t.Error("expected error for nonexistent file")
	}

	// 测试空参数
	results = playbook.ValidatePlaybookFiles([]string{})
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty input, got %d", len(results))
	}
}

func TestApplyDefaultConfig(t *testing.T) {
	t.Run("CLI group not set, default has groups", func(t *testing.T) {
		group, _, _ := playbook.ApplyDefaultConfig("", "", "",
			[]string{"web", "db"}, nil, nil)
		if group != "web,db" {
			t.Errorf("expected group 'web,db', got '%s'", group)
		}
	})

	t.Run("CLI group set, default ignored", func(t *testing.T) {
		group, _, _ := playbook.ApplyDefaultConfig("db", "", "",
			[]string{"web"}, nil, nil)
		if group != "db" {
			t.Errorf("expected group 'db', got '%s'", group)
		}
	})

	t.Run("CLI tags not set, default has tags", func(t *testing.T) {
		_, tags, _ := playbook.ApplyDefaultConfig("", "", "",
			nil, []string{"deploy", "verify"}, nil)
		if tags != "deploy,verify" {
			t.Errorf("expected tags 'deploy,verify', got '%s'", tags)
		}
	})

	t.Run("CLI tags set, default ignored", func(t *testing.T) {
		_, tags, _ := playbook.ApplyDefaultConfig("", "custom-tag", "",
			nil, []string{"deploy"}, nil)
		if tags != "custom-tag" {
			t.Errorf("expected tags 'custom-tag', got '%s'", tags)
		}
	})

	t.Run("CLI skip-tags not set, default has skip_tags", func(t *testing.T) {
		_, _, skipTags := playbook.ApplyDefaultConfig("", "", "",
			nil, nil, []string{"debug"})
		if skipTags != "debug" {
			t.Errorf("expected skipTags 'debug', got '%s'", skipTags)
		}
	})

	t.Run("CLI values preserved with no defaults", func(t *testing.T) {
		group, tags, _ := playbook.ApplyDefaultConfig("my-group", "my-tags", "", nil, nil, nil)
		if group != "my-group" {
			t.Errorf("expected group 'my-group', got '%s'", group)
		}
		if tags != "my-tags" {
			t.Errorf("expected tags 'my-tags', got '%s'", tags)
		}
	})

	t.Run("CLI group set, default also set", func(t *testing.T) {
		group, _, _ := playbook.ApplyDefaultConfig("cli-group", "", "",
			[]string{"default-group"}, nil, nil)
		if group != "cli-group" {
			t.Errorf("expected group 'cli-group' (CLI wins), got '%s'", group)
		}
	})
}
