package ai

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileDownloadValidate(t *testing.T) {
	tool := NewFileDownloadTool(nil)

	ok := map[string]interface{}{
		"remote_file": "/var/log/nginx/access.log",
		"nodes":       []interface{}{"web-01"},
	}
	if err := tool.Validate(ok); err != nil {
		t.Errorf("expected valid params, got %v", err)
	}

	missingFile := map[string]interface{}{"nodes": []interface{}{"web-01"}}
	if err := tool.Validate(missingFile); err == nil {
		t.Error("expected error for missing remote_file")
	}

	missingTarget := map[string]interface{}{"remote_file": "/tmp/x.log"}
	if err := tool.Validate(missingTarget); err == nil {
		t.Error("expected error for missing target nodes")
	}
}

func TestPlaybookGenerateTool(t *testing.T) {
	dir := t.TempDir()
	origDir := playbookSaveDir
	playbookSaveDir = func() string { return dir }
	defer func() { playbookSaveDir = origDir }()

	tool := NewPlaybookGenerateTool(&mockNodeMgrForAI{})
	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"requirement": "Install nginx on all web nodes and start it",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(out, dir) {
		t.Errorf("expected save path in output, got %q", out)
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 1 {
		t.Fatalf("expected 1 saved playbook, got %d", len(files))
	}
	if !strings.HasSuffix(files[0].Name(), ".yaml") {
		t.Errorf("expected .yaml file, got %s", files[0].Name())
	}
	content, _ := os.ReadFile(filepath.Join(dir, files[0].Name()))
	if !strings.Contains(string(content), "Install nginx") {
		t.Errorf("expected requirement in playbook content, got %s", content)
	}
}

func TestPlaybookGenerateToolValidation(t *testing.T) {
	tool := NewPlaybookGenerateTool(nil)
	if err := tool.Validate(map[string]interface{}{"requirement": "install nginx"}); err != nil {
		t.Errorf("expected valid, got %v", err)
	}
	if err := tool.Validate(map[string]interface{}{}); err == nil {
		t.Error("expected error for missing requirement")
	}
}

func TestPlaybookTemplateToolsValidate(t *testing.T) {
	info := NewPlaybookTemplateInfoTool(nil)
	if err := info.Validate(map[string]interface{}{"name": "deploy-app"}); err != nil {
		t.Errorf("expected valid, got %v", err)
	}
	if err := info.Validate(map[string]interface{}{}); err == nil {
		t.Error("expected error for missing name")
	}

	show := NewPlaybookStateShowTool(nil)
	if err := show.Validate(map[string]interface{}{"run_id": "run-1"}); err != nil {
		t.Errorf("expected valid, got %v", err)
	}
	if err := show.Validate(map[string]interface{}{}); err == nil {
		t.Error("expected error for missing run_id")
	}
}

func TestIntentDownloadNotReverse(t *testing.T) {
	c := NewIntentClassifier()

	// "下载"必须命中 file_download，而不是 transfer_file（修复行为反转）
	dl := c.Classify("把 /var/log/nginx/access.log 从 web-01 下载到本地")
	if dl.Type != IntentFileDownload {
		t.Errorf("expected file_download, got %s", dl.Type)
	}

	// 上传/传输仍命中 transfer_file
	up := c.Classify("上传 ./app.tar.gz 到 web-01")
	if up.Type != IntentTransferFile {
		t.Errorf("expected transfer_file, got %s", up.Type)
	}
}

func TestFileDownloadParamExtraction(t *testing.T) {
	ext := NewParamExtractor([]string{"web-01", "db-01"})
	params := ext.ExtractParams(IntentFileDownload, "把 /var/log/nginx/access.log 从 web-01 下载到本地")
	if params["remote_file"] != "/var/log/nginx/access.log" {
		t.Errorf("expected remote_file, got %v", params["remote_file"])
	}
	if nodes, ok := params["nodes"].([]interface{}); !ok || len(nodes) == 0 || nodes[0] != "web-01" {
		t.Errorf("expected target node web-01, got %v", params["nodes"])
	}
}

func TestNewToolsRegistered(t *testing.T) {
	agent := newTestAgentForRoute(nil)
	want := []string{
		"file_download",
		"playbook_template_list", "playbook_template_info", "playbook_template_export",
		"playbook_scaffold", "playbook_state_list", "playbook_state_show", "playbook_generate",
	}
	for _, name := range want {
		if _, ok := agent.registry.Get(name); !ok {
			t.Errorf("tool %s not registered", name)
		}
	}
	if _, ok := agent.registry.Get("playbook_info"); ok {
		t.Error("obsolete playbook_info tool should be removed from registry")
	}
}
