package playbook

import (
	"testing"
)

func TestParser_Parse(t *testing.T) {
	parser := NewParser()

	content := `
name: Test Playbook
hosts:
  - web
  - database
vars:
  port: 8080
  env: production
pre_tasks:
  - name: pre task 1
    action: debug
    args:
      msg: "before tasks"
tasks:
  - name: task 1
    action: command
    args:
      cmd: echo hello
  - name: task 2
    action: shell
    args:
      script: /tmp/script.sh
post_tasks:
  - name: post task 1
    action: cleanup
`

	parsed, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if parsed.Raw.Name != "Test Playbook" {
		t.Errorf("expected name 'Test Playbook', got '%s'", parsed.Raw.Name)
	}

	if len(parsed.Raw.Hosts) != 2 {
		t.Errorf("expected 2 hosts, got %d", len(parsed.Raw.Hosts))
	}

	if len(parsed.PreTasks) != 1 {
		t.Errorf("expected 1 pre_task, got %d", len(parsed.PreTasks))
	}

	if len(parsed.Tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(parsed.Tasks))
	}

	if len(parsed.PostTasks) != 1 {
		t.Errorf("expected 1 post_task, got %d", len(parsed.PostTasks))
	}
}

func TestParser_ParseInvalidYAML(t *testing.T) {
	parser := NewParser()

	invalidContent := `: invalid yaml start`

	_, err := parser.Parse(invalidContent)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestParser_ParseEmptyHosts(t *testing.T) {
	parser := NewParser()

	content := `
name: Test Playbook
hosts: []
tasks:
  - name: task 1
    action: command
`

	parsed, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("unexpected error for empty hosts: %v", err)
	}
	if len(parsed.Raw.Hosts) != 0 {
		t.Errorf("expected empty hosts, got %v", parsed.Raw.Hosts)
	}
}

func TestParser_ParseTaskWithWhen(t *testing.T) {
	parser := NewParser()

	content := `
name: Test Playbook
hosts:
  - web
vars:
  debug: true
tasks:
  - name: conditional task
    action: debug
    args:
      msg: "debug mode"
    when: debug == true
`

	parsed, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(parsed.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(parsed.Tasks))
	}

	task := parsed.Tasks[0]
	if task.Condition == nil {
		t.Fatal("expected condition to be parsed")
	}

	if task.Condition.Expression != "debug == true" {
		t.Errorf("expected condition 'debug == true', got '%s'", task.Condition.Expression)
	}
}

func TestParser_ParseTaskWithWithItems(t *testing.T) {
	parser := NewParser()

	content := `
name: Test Playbook
hosts:
  - web
tasks:
  - name: loop task
    action: debug
    args:
      msg: "item: {{ item }}"
    with_items:
      - one
      - two
      - three
`

	parsed, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	task := parsed.Tasks[0]
	if task.Loop == nil {
		t.Fatal("expected loop to be parsed")
	}

	if len(task.Loop.Items) != 3 {
		t.Errorf("expected 3 loop items, got %d", len(task.Loop.Items))
	}
}

func TestParser_ParseTaskWithIgnoreErrors(t *testing.T) {
	parser := NewParser()

	content := `
name: Test Playbook
hosts:
  - web
tasks:
  - name: task with ignore
    action: command
    args:
      cmd: may fail
    ignore_errors: true
`

	parsed, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	task := parsed.Tasks[0]
	if !task.Options.IgnoreErrors {
		t.Error("expected IgnoreErrors to be true")
	}
}

func TestParser_ParseTaskWithTags(t *testing.T) {
	parser := NewParser()

	content := `
name: Test Playbook
hosts:
  - web
tasks:
  - name: tagged task
    action: command
    args:
      cmd: echo hello
    tags:
      - setup
      - install
`

	parsed, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	task := parsed.Tasks[0]
	if len(task.Options.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(task.Options.Tags))
	}
}

func TestParser_ParseTaskWithTimeout(t *testing.T) {
	parser := NewParser()

	content := `
name: Test Playbook
hosts:
  - web
tasks:
  - name: long running task
    action: command
    args:
      cmd: sleep 300
    timeout:
      connect: 5s
      command: 10m
`

	parsed, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	task := parsed.Tasks[0]
	if task.ActionOpts == nil {
		t.Fatal("expected ActionOpts to be parsed")
	}
	if task.ActionOpts.Timeout == nil {
		t.Fatal("expected Timeout to be parsed")
	}
	if task.ActionOpts.Timeout.Connect != 5*1e9 {
		t.Errorf("expected Connect timeout 5s, got %v", task.ActionOpts.Timeout.Connect)
	}
	if task.ActionOpts.Timeout.Command != 600*1e9 {
		t.Errorf("expected Command timeout 10m, got %v", task.ActionOpts.Timeout.Command)
	}
}

func TestParser_ParseTaskWithRetry(t *testing.T) {
	parser := NewParser()

	content := `
name: Test Playbook
hosts:
  - web
tasks:
  - name: retry task
    action: command
    args:
      cmd: curl http://api.example.com
    retry:
      max: 3
      interval: 2s
      max_interval: 30s
`

	parsed, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	task := parsed.Tasks[0]
	if task.ActionOpts == nil {
		t.Fatal("expected ActionOpts to be parsed")
	}
	if task.ActionOpts.Retry == nil {
		t.Fatal("expected Retry to be parsed")
	}
	if task.ActionOpts.Retry.Max != 3 {
		t.Errorf("expected max retries 3, got %d", task.ActionOpts.Retry.Max)
	}
	if task.ActionOpts.Retry.Interval != 2*1e9 {
		t.Errorf("expected interval 2s, got %v", task.ActionOpts.Retry.Interval)
	}
	if task.ActionOpts.Retry.MaxInterval != 30*1e9 {
		t.Errorf("expected max interval 30s, got %v", task.ActionOpts.Retry.MaxInterval)
	}
}

func TestParser_ParseTaskWithTimeoutAndRetry(t *testing.T) {
	parser := NewParser()

	content := `
name: Test Playbook
hosts:
  - web
tasks:
  - name: complex task
    action: shell
    args:
      command: long-running-script.sh
    timeout:
      command: 5m
    retry:
      max: 5
      interval: 1s
`

	parsed, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	task := parsed.Tasks[0]
	if task.ActionOpts == nil {
		t.Fatal("expected ActionOpts to be parsed")
	}
	if task.ActionOpts.Timeout == nil {
		t.Fatal("expected Timeout to be parsed")
	}
	if task.ActionOpts.Retry == nil {
		t.Fatal("expected Retry to be parsed")
	}
	if task.ActionOpts.Timeout.Command != 300*1e9 {
		t.Errorf("expected Command timeout 5m, got %v", task.ActionOpts.Timeout.Command)
	}
	if task.ActionOpts.Retry.Max != 5 {
		t.Errorf("expected max retries 5, got %d", task.ActionOpts.Retry.Max)
	}
}

func TestParser_ParseTaskWithoutTimeoutOrRetry(t *testing.T) {
	parser := NewParser()

	content := `
name: Test Playbook
hosts:
  - web
tasks:
  - name: simple task
    action: debug
    args:
      msg: hello
`

	parsed, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	task := parsed.Tasks[0]
	if task.ActionOpts != nil {
		t.Error("expected ActionOpts to be nil for simple task")
	}
}

func TestParser_ParseExecutionMode(t *testing.T) {
	parser := NewParser()

	t.Run("pipeline mode", func(t *testing.T) {
		content := `
name: Pipeline Playbook
hosts:
  - web
execution_mode: pipeline
tasks:
  - name: task 1
    action: command
`
		parsed, err := parser.Parse(content)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if parsed.ExecutionMode != ExecutionModePipeline {
			t.Errorf("expected ExecutionModePipeline, got '%s'", parsed.ExecutionMode)
		}
	})

	t.Run("default mode", func(t *testing.T) {
		content := `
name: Default Playbook
hosts:
  - web
tasks:
  - name: task 1
    action: command
`
		parsed, err := parser.Parse(content)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if parsed.ExecutionMode != ExecutionModeFailContinue {
			t.Errorf("expected ExecutionModeFailContinue, got '%s'", parsed.ExecutionMode)
		}
	})
}

func TestParser_ParseInvalidExecutionMode(t *testing.T) {
	parser := NewParser()

	content := `
name: Invalid Playbook
hosts:
  - web
execution_mode: invalid_mode
tasks:
  - name: task 1
    action: command
`
	_, err := parser.Parse(content)
	if err == nil {
		t.Error("expected error for invalid execution_mode")
	}
}
func TestParser_extractAction(t *testing.T) {

	parser := NewParser()

	tests := []struct {
		input    string
		expected string
	}{
		{"shell", "shell"},
		{"shell /bin/bash", "shell"},
		{"command ls -la", "command"},
		{"debug msg='test'", "debug"},
	}

	for _, tt := range tests {
		result := parser.extractAction(tt.input)
		if result != tt.expected {
			t.Errorf("extractAction(%s) = %s, expected %s", tt.input, result, tt.expected)
		}
	}
}

func TestParser_extractVariables(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		input    string
		expected []string
	}{
		{"{{ name }}", []string{"name"}},
		{"{{ name }} and {{ age }}", []string{"name", "age"}},
		{"no variables", nil},
		{"{{ var }} {{ var }}", []string{"var"}},
	}

	for _, tt := range tests {
		result := parser.extractVariables(tt.input)
		if tt.expected == nil {
			if len(result) != 0 {
				t.Errorf("extractVariables(%s) = %v, expected empty", tt.input, result)
			}
		} else {
			if len(result) != len(tt.expected) {
				t.Errorf("extractVariables(%s) = %v, expected %v", tt.input, result, tt.expected)
			}
		}
	}
}

func TestParser_processVariables(t *testing.T) {
	parser := NewParser()

	vars := map[string]interface{}{
		"port": 8080,
		"name": "test",
		"nested": map[string]interface{}{
			"key": "value",
		},
		"list": []interface{}{"a", "b"},
	}

	result := parser.processVariables(vars)

	if result["port"] != 8080 {
		t.Errorf("expected port 8080, got %v", result["port"])
	}
	if result["name"] != "test" {
		t.Errorf("expected name 'test', got %v", result["name"])
	}
}

func TestParser_Validate(t *testing.T) {
	parser := NewParser()

	t.Run("valid playbook", func(t *testing.T) {
		content := `
name: Test
hosts:
  - web
tasks:
  - name: task 1
    action: debug
`
		parsed, _ := parser.Parse(content)
		errors := parser.Validate(parsed)
		if len(errors) != 0 {
			t.Errorf("expected no errors, got %v", errors)
		}
	})

	t.Run("nil playbook", func(t *testing.T) {
		errors := parser.Validate(nil)
		if len(errors) == 0 {
			t.Error("expected error for nil playbook")
		}
	})

	t.Run("playbook with unnamed task", func(t *testing.T) {
		content := `
name: Test
hosts:
  - web
tasks:
  - action: debug
`
		parsed, _ := parser.Parse(content)
		errors := parser.Validate(parsed)
		if len(errors) == 0 {
			t.Error("expected error for unnamed task")
		}
	})
}

func TestTemplateEngine_Render(t *testing.T) {
	engine := NewTemplateEngine(map[string]interface{}{
		"name": "test",
		"port": 8080,
	})

	tests := []struct {
		template string
		expected string
	}{
		{"hello {{ name }}", "hello test"},
		{"port: {{ port }}", "port: 8080"},
		{"no variables", "no variables"},
	}

	for _, tt := range tests {
		result, _ := engine.Render(tt.template)
		if result != tt.expected {
			t.Errorf("Render(%s) = %s, expected %s", tt.template, result, tt.expected)
		}
	}
}

func TestTemplateEngine_SetVariable(t *testing.T) {
	engine := NewTemplateEngine(nil)

	engine.SetVariable("name", "test")
	val, ok := engine.GetVariable("name")
	if !ok {
		t.Error("expected variable to be set")
	}
	if val != "test" {
		t.Errorf("expected 'test', got '%v'", val)
	}
}

func TestTemplateEngine_MergeVariables(t *testing.T) {
	engine := NewTemplateEngine(map[string]interface{}{
		"existing": "value1",
	})

	engine.MergeVariables(map[string]interface{}{
		"new": "value2",
	})

	if _, ok := engine.GetVariable("existing"); !ok {
		t.Error("expected existing variable to remain")
	}
	if _, ok := engine.GetVariable("new"); !ok {
		t.Error("expected new variable to be added")
	}
}

func TestConditionEvaluator_Evaluate(t *testing.T) {
	t.Run("nil condition", func(t *testing.T) {
		evaluator := NewConditionEvaluator(nil)
		result, err := evaluator.Evaluate(nil)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !result {
			t.Error("expected true for nil condition")
		}
	})

	t.Run("simple variable", func(t *testing.T) {
		evaluator := NewConditionEvaluator(map[string]interface{}{
			"debug": true,
		})
		condition := &Condition{Expression: "debug", Variables: []string{"debug"}}
		result, _ := evaluator.Evaluate(condition)
		if !result {
			t.Error("expected true for debug=true")
		}
	})

	t.Run("not condition", func(t *testing.T) {
		evaluator := NewConditionEvaluator(map[string]interface{}{
			"debug": false,
		})
		condition := &Condition{Expression: "not debug", Variables: []string{"debug"}}
		result, _ := evaluator.Evaluate(condition)
		if !result {
			t.Error("expected true for not false")
		}
	})
}

func TestConditionEvaluator_evaluateSimpleCondition(t *testing.T) {
	evaluator := NewConditionEvaluator(map[string]interface{}{
		"value1": "test",
		"value2": 10,
		"value3": 20,
	})

	tests := []struct {
		condition string
		expected  bool
	}{
		{"value1 == test", true},
		{"value1 == wrong", false},
		{"value1 != wrong", true},
		{"value2 > 5", true},
		{"value2 >= 10", true},
		{"value2 < 5", false},
		{"value2 <= 10", true},
		{"value3 == value2", false},
	}

	for _, tt := range tests {
		result, err := evaluator.evaluateSimpleCondition(tt.condition)
		if err != nil {
			t.Errorf("unexpected error for %s: %v", tt.condition, err)
		}
		if result != tt.expected {
			t.Errorf("%s = %v, expected %v", tt.condition, result, tt.expected)
		}
	}
}

func TestNewParser(t *testing.T) {
	parser := NewParser()
	if parser == nil {
		t.Fatal("expected parser to be created")
	}
	if parser.variablePattern == nil {
		t.Error("expected variablePattern to be set")
	}
}

func TestParsedPlaybook_Variables(t *testing.T) {
	parser := NewParser()

	content := `
name: Test
hosts:
  - web
vars:
  app_name: myapp
  version: 1.0
tasks:
  - name: task 1
    action: debug
`

	parsed, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if parsed.Variables["app_name"] != "myapp" {
		t.Errorf("expected app_name 'myapp', got '%v'", parsed.Variables["app_name"])
	}
}

func TestPlaybookTask_AllFields(t *testing.T) {
	parser := NewParser()

	content := `
name: Test
hosts:
  - web
vars:
  always_run: false
tasks:
  - name: complete task
    action: command
    args:
      cmd: echo hello
    when: always_run == true
    with_items:
      - item1
      - item2
    ignore_errors: false
    any_errors_fatal: true
    tags:
      - build
    register: cmd_result
    changed_when: "cmd_result.rc != 0"
    failed_when: "cmd_result.rc == 2"
`

	parsed, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	task := parsed.Tasks[0]

	if task.Name != "complete task" {
		t.Errorf("expected name 'complete task', got '%s'", task.Name)
	}

	if task.Action != "command" {
		t.Errorf("expected action 'command', got '%s'", task.Action)
	}

	if task.Condition == nil {
		t.Fatal("expected condition to be parsed")
	}

	if task.Loop == nil {
		t.Fatal("expected loop to be parsed")
	}

	if !task.Options.AnyErrorsFatal {
		t.Error("expected AnyErrorsFatal to be true")
	}

	if task.Options.Register != "cmd_result" {
		t.Errorf("expected Register 'cmd_result', got '%s'", task.Options.Register)
	}
}

func TestConditionEvaluator_AndOr(t *testing.T) {
	evaluator := NewConditionEvaluator(map[string]interface{}{
		"a": true,
		"b": true,
		"c": false,
	})

	t.Run("and - all true", func(t *testing.T) {
		result, _ := evaluator.evaluateExpression("a and b")
		if !result {
			t.Error("expected true")
		}
	})

	t.Run("and - one false", func(t *testing.T) {
		result, _ := evaluator.evaluateExpression("a and c")
		if result {
			t.Error("expected false")
		}
	})

	t.Run("or - one true", func(t *testing.T) {
		result, _ := evaluator.evaluateExpression("a or c")
		if !result {
			t.Error("expected true")
		}
	})

	t.Run("or - all false", func(t *testing.T) {
		result, _ := evaluator.evaluateExpression("c or c")
		if result {
			t.Error("expected false")
		}
	})
}

func TestConditionEvaluator_StringComparison(t *testing.T) {
	evaluator := NewConditionEvaluator(map[string]interface{}{
		"env": "production",
	})

	t.Run("string equals", func(t *testing.T) {
		result, _ := evaluator.evaluateExpression("env == production")
		if !result {
			t.Error("expected true")
		}
	})

	t.Run("string not equals", func(t *testing.T) {
		result, _ := evaluator.evaluateExpression("env != staging")
		if !result {
			t.Error("expected true")
		}
	})
}

func TestParser_ParseWithDescription(t *testing.T) {
	parser := NewParser()

	content := `
name: Test Playbook
description: 这是一个测试剧本
hosts:
  - web
tasks:
  - name: task 1
    action: command
    args:
      cmd: echo hello
`

	parsed, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if parsed.Raw.Description != "这是一个测试剧本" {
		t.Errorf("expected description '这是一个测试剧本', got '%s'", parsed.Raw.Description)
	}
}

func TestParser_ParseWithDefaultBlock(t *testing.T) {
	parser := NewParser()

	content := `
name: Test Playbook
hosts:
  - web
default:
  groups:
    - web
    - db
  tags:
    - deploy
  skip_tags:
    - debug
tasks:
  - name: task 1
    action: command
    args:
      cmd: echo hello
`

	parsed, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if parsed.DefaultConfig == nil {
		t.Fatal("expected DefaultConfig to be parsed")
	}

	if len(parsed.DefaultConfig.Groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(parsed.DefaultConfig.Groups))
	}
	if parsed.DefaultConfig.Groups[0] != "web" {
		t.Errorf("expected first group 'web', got '%s'", parsed.DefaultConfig.Groups[0])
	}
	if parsed.DefaultConfig.Groups[1] != "db" {
		t.Errorf("expected second group 'db', got '%s'", parsed.DefaultConfig.Groups[1])
	}

	if len(parsed.DefaultConfig.Tags) != 1 {
		t.Errorf("expected 1 tag, got %d", len(parsed.DefaultConfig.Tags))
	}
	if parsed.DefaultConfig.Tags[0] != "deploy" {
		t.Errorf("expected tag 'deploy', got '%s'", parsed.DefaultConfig.Tags[0])
	}

	if len(parsed.DefaultConfig.SkipTags) != 1 {
		t.Errorf("expected 1 skip_tag, got %d", len(parsed.DefaultConfig.SkipTags))
	}
	if parsed.DefaultConfig.SkipTags[0] != "debug" {
		t.Errorf("expected skip_tag 'debug', got '%s'", parsed.DefaultConfig.SkipTags[0])
	}
}

func TestParser_ParseWithDefaultTimeoutRetry(t *testing.T) {
	parser := NewParser()

	content := `
name: Test Playbook
hosts:
  - web
default:
  timeout:
    connect: 10s
    command: 5m
  retry:
    max: 3
    interval: 2s
    max_interval: 30s
tasks:
  - name: task 1
    action: command
    args:
      cmd: echo hello
`

	parsed, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if parsed.DefaultConfig == nil {
		t.Fatal("expected DefaultConfig to be parsed")
	}

	if parsed.DefaultConfig.Timeout == nil {
		t.Fatal("expected Timeout in default block")
	}
	if parsed.DefaultConfig.Timeout.Connect != "10s" {
		t.Errorf("expected connect timeout '10s', got '%s'", parsed.DefaultConfig.Timeout.Connect)
	}
	if parsed.DefaultConfig.Timeout.Command != "5m" {
		t.Errorf("expected command timeout '5m', got '%s'", parsed.DefaultConfig.Timeout.Command)
	}

	if parsed.DefaultConfig.Retry == nil {
		t.Fatal("expected Retry in default block")
	}
	if parsed.DefaultConfig.Retry.Max != 3 {
		t.Errorf("expected max retries 3, got %d", parsed.DefaultConfig.Retry.Max)
	}
	if parsed.DefaultConfig.Retry.Interval != "2s" {
		t.Errorf("expected retry interval '2s', got '%s'", parsed.DefaultConfig.Retry.Interval)
	}
	if parsed.DefaultConfig.Retry.MaxInterval != "30s" {
		t.Errorf("expected max retry interval '30s', got '%s'", parsed.DefaultConfig.Retry.MaxInterval)
	}
}

func TestParser_ParseWithoutDefaultBlock(t *testing.T) {
	parser := NewParser()

	content := `
name: Test Playbook
hosts:
  - web
tasks:
  - name: task 1
    action: command
`

	parsed, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if parsed.DefaultConfig != nil {
		t.Error("expected DefaultConfig to be nil when no default block in YAML")
	}
}

func TestParser_ValidatePipelineMode(t *testing.T) {
	parser := NewParser()

	t.Run("pipeline with post_tasks", func(t *testing.T) {
		content := `
name: Test
hosts:
  - web
execution_mode: pipeline
tasks:
  - name: task 1
    action: debug
post_tasks:
  - name: cleanup
    action: debug
`
		_, err := parser.Parse(content)
		if err == nil {
			t.Error("expected error for pipeline with post_tasks")
		}
	})

	t.Run("pipeline with ignore_errors", func(t *testing.T) {
		content := `
name: Test
hosts:
  - web
execution_mode: pipeline
tasks:
  - name: task 1
    action: debug
    ignore_errors: true
`
		_, err := parser.Parse(content)
		if err == nil {
			t.Error("expected error for pipeline with ignore_errors")
		}
	})

	t.Run("pipeline with any_errors_fatal", func(t *testing.T) {
		content := `
name: Test
hosts:
  - web
execution_mode: pipeline
tasks:
  - name: task 1
    action: debug
    any_errors_fatal: true
`
		_, err := parser.Parse(content)
		if err == nil {
			t.Error("expected error for pipeline with any_errors_fatal")
		}
	})
}
