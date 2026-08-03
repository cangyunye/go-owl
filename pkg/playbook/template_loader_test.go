package playbook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateParams_Defaults(t *testing.T) {
	params := []TemplateParameter{
		{Name: "version", Description: "版本号", Default: "1.0", Type: "string"},
		{Name: "port", Description: "端口", Default: 80, Type: "number"},
	}
	vals, err := ValidateParams(params, map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	if vals["version"] != "1.0" {
		t.Errorf("expected default version 1.0, got %v", vals["version"])
	}
}

func TestValidateParams_Required(t *testing.T) {
	params := []TemplateParameter{
		{Name: "app_name", Description: "应用名", Required: true},
	}
	_, err := ValidateParams(params, map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for missing required param")
	}
}

func TestValidateParams_Options(t *testing.T) {
	params := []TemplateParameter{
		{Name: "port", Description: "端口", Options: []interface{}{80, 443}},
	}
	_, err := ValidateParams(params, map[string]interface{}{"port": 8080})
	if err == nil {
		t.Fatal("expected error for invalid option")
	}
}

func TestValidateParams_Pattern(t *testing.T) {
	params := []TemplateParameter{
		{Name: "version", Description: "版本", Pattern: `^\d+\.\d+\.\d+$`},
	}
	_, err := ValidateParams(params, map[string]interface{}{"version": "abc"})
	if err == nil {
		t.Fatal("expected error for pattern mismatch")
	}
	vals, err := ValidateParams(params, map[string]interface{}{"version": "1.2.3"})
	if err != nil {
		t.Fatal(err)
	}
	if vals["version"] != "1.2.3" {
		t.Errorf("expected 1.2.3, got %v", vals["version"])
	}
}

func TestInstantiate(t *testing.T) {
	yamlContent := `description: test
parameters:
  - name: app_name
    description: "应用名"
    default: myapp
tasks:
  - name: 部署
    action: command
    args:
      cmd: echo {{ app_name }}
`
	result, err := Instantiate([]byte(yamlContent), map[string]interface{}{"app_name": "prod-app"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result), "prod-app") {
		t.Errorf("expected instantiated content to contain prod-app")
	}
	if strings.Contains(string(result), "parameters:") {
		t.Errorf("instantiated content should not contain parameters block")
	}
}

func TestParseTemplateMeta(t *testing.T) {
	yamlContent := `description: Nginx 部署模板
tags: [nginx, deploy]
parameters:
  - name: nginx_version
    description: "Nginx 版本"
    default: "1.24.0"
tasks:
  - name: install
    action: command
    args:
      cmd: echo hello
`
	meta, err := ParseTemplateMeta([]byte(yamlContent))
	if err != nil {
		t.Fatal(err)
	}
	if meta.Description != "Nginx 部署模板" {
		t.Errorf("unexpected description: %s", meta.Description)
	}
	if len(meta.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(meta.Tags))
	}
	if len(meta.Parameters) != 1 {
		t.Errorf("expected 1 parameter, got %d", len(meta.Parameters))
	}
	if meta.Parameters[0].Name != "nginx_version" {
		t.Errorf("unexpected param name: %s", meta.Parameters[0].Name)
	}
}

func TestLoadTemplates_BuiltinOnly(t *testing.T) {
	entries, err := LoadTemplates("/nonexistent/user/path")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected builtin templates to be loaded")
	}
	found := false
	for _, e := range entries {
		if e.Name == "utility/healthcheck/http" {
			found = true
			if e.Source != "builtin" {
				t.Errorf("expected source builtin, got %s", e.Source)
			}
			if e.Category != "utility" {
				t.Errorf("expected category utility, got %s", e.Category)
			}
		}
	}
	if !found {
		t.Error("expected to find utility/healthcheck/http template")
	}
}

func TestGetTemplate(t *testing.T) {
	entry, err := GetTemplate("utility/healthcheck/http", "/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Meta.Description == "" {
		t.Error("expected non-empty description")
	}
	if len(entry.Meta.Parameters) == 0 {
		t.Error("expected parameters")
	}
	if entry.Content == nil {
		t.Error("expected non-nil content")
	}
}

func TestGetTemplate_NotFound(t *testing.T) {
	_, err := GetTemplate("nonexistent/template", "/nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}
}

func TestLoadTemplates_UserOverride(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "utility", "healthcheck"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "utility", "healthcheck", "http.yaml"), []byte("description: user override\ntasks: []\n"), 0644)

	entries, err := LoadTemplates(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name == "utility/healthcheck/http" {
			if e.Source != "user" {
				t.Errorf("expected user source to override builtin, got %s", e.Source)
			}
			if e.Meta.Description != "user override" {
				t.Errorf("expected user description, got %s", e.Meta.Description)
			}
			return
		}
	}
	t.Error("template not found")
}

func TestInstantiate_DollarSignInValue(t *testing.T) {
	yamlContent := "tasks:\n  - name: test\n    action: command\n    args:\n      cmd: echo {{ path }}\n"
	result, err := Instantiate([]byte(yamlContent), map[string]interface{}{"path": "/data/$HOME/${USER}"})
	if err != nil {
		t.Fatal(err)
	}
	output := string(result)
	if !strings.Contains(output, "/data/$HOME/${USER}") {
		t.Errorf("dollar signs corrupted, got: %s", output)
	}
}

func TestInstantiate_LeftoverPlaceholders(t *testing.T) {
	yamlContent := "tasks:\n  - name: test\n    action: command\n    args:\n      cmd: echo {{ a }} {{ b }}\n"
	result, err := Instantiate([]byte(yamlContent), map[string]interface{}{"a": "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result), "{{ b }}") {
		t.Log("Note: leftover placeholders are silently preserved (no error)")
	}
}

func TestValidateParams_UnknownKeys(t *testing.T) {
	params := []TemplateParameter{
		{Name: "a", Description: "param a", Default: "1"},
	}
	vals, err := ValidateParams(params, map[string]interface{}{"a": "x", "typo_key": "y"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := vals["typo_key"]; ok {
		t.Log("Note: unknown keys are included (no filtering)")
	} else {
		t.Log("Note: unknown keys are silently dropped")
	}
}

func TestValidateParams_TypeNotEnforced(t *testing.T) {
	params := []TemplateParameter{
		{Name: "port", Description: "port", Type: "number"},
	}
	vals, err := ValidateParams(params, map[string]interface{}{"port": "not-a-number"})
	if err != nil {
		t.Log("Type validation is enforced")
	} else {
		t.Logf("Note: type 'number' accepted string value %q without error", vals["port"])
	}
}

func TestLoadTemplates_MalformedYAML(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "good.yaml"), []byte("description: good\ntasks: []\n"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "bad.yaml"), []byte("{{{{invalid yaml::::"), 0644)

	entries, err := LoadTemplates(tmpDir)
	if err != nil {
		t.Logf("Note: malformed YAML aborts entire load: %v", err)
	} else {
		t.Logf("Loaded %d entries despite malformed file", len(entries))
	}
}

func TestLoadTemplates_YmlExtension(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "test.yml"), []byte("description: yml test\ntasks: []\n"), 0644)
	entries, err := LoadTemplates(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if e.Name == "test" && e.Source == "user" {
			found = true
		}
	}
	if !found {
		t.Error(".yml extension not loaded")
	}
}

func TestInstantiate_NewlineInjection(t *testing.T) {
	yamlContent := "tasks:\n  - name: test\n    action: command\n    args:\n      cmd: echo {{ url }}\n"
	malicious := "hello\ntasks:\n  - name: injected\n    action: command\n    args:\n      cmd: rm -rf /"
	result, err := Instantiate([]byte(yamlContent), map[string]interface{}{"url": malicious})
	if err != nil {
		t.Fatal(err)
	}
	output := string(result)
	if strings.Contains(output, "injected") {
		t.Log("WARNING: newline in value can inject YAML structure")
	}
}

func TestAllBuiltinTemplates_Parseable(t *testing.T) {
	entries, err := LoadTemplates("/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Source != "builtin" {
			continue
		}
		meta, err := ParseTemplateMeta(e.Content)
		if err != nil {
			t.Errorf("builtin template %s failed to parse: %v", e.Name, err)
			continue
		}
		if meta.Description == "" {
			t.Errorf("builtin template %s has empty description", e.Name)
		}
	}
	if len(entries) < 3 {
		t.Errorf("expected at least 3 builtin templates, got %d", len(entries))
	}
}
