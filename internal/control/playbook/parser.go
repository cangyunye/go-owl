package playbook

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Playbook struct {
	Name      string                 `yaml:"name"`
	Hosts     []string               `yaml:"hosts"`
	Vars      map[string]interface{} `yaml:"vars"`
	PreTasks  []PlaybookTask         `yaml:"pre_tasks"`
	Tasks     []PlaybookTask         `yaml:"tasks"`
	PostTasks []PlaybookTask         `yaml:"post_tasks"`
}

type PlaybookTask struct {
	Name           string                 `yaml:"name"`
	Action         string                 `yaml:"action"`
	Args           map[string]interface{} `yaml:"args"`
	When           string                 `yaml:"when"`
	WithItems      []interface{}          `yaml:"with_items"`
	LoopVar        string                 `yaml:"loop_control"`
	IgnoreErrors   bool                   `yaml:"ignore_errors"`
	AnyErrorsFatal bool                   `yaml:"any_errors_fatal"`
	Tags           []string               `yaml:"tags"`
	Register       string                 `yaml:"register"`
	ChangedWhen    string                 `yaml:"changed_when"`
	FailedWhen     string                 `yaml:"failed_when"`
	Timeout        *TimeoutConfigYAML     `yaml:"timeout"`
	Retry          *RetryConfigYAML        `yaml:"retry"`
}

type TimeoutConfigYAML struct {
	Connect string `yaml:"connect"`
	Command string `yaml:"command"`
}

type RetryConfigYAML struct {
	Max         int    `yaml:"max"`
	Interval    string `yaml:"interval"`
	MaxInterval string `yaml:"max_interval"`
}

type ParsedPlaybook struct {
	Raw       *Playbook
	Variables map[string]interface{}
	Tasks     []*ParsedTask
	PreTasks  []*ParsedTask
	PostTasks []*ParsedTask
}

type ParsedTask struct {
	Raw       *PlaybookTask
	Name      string
	Action    string
	Args      map[string]interface{}
	Condition *Condition
	Loop      *Loop
	Options   TaskOptions
	ActionOpts *ActionOptions
}

type Condition struct {
	Expression string
	Variables  []string
}

type Loop struct {
	Items   []interface{}
	VarName string
}

type TaskOptions struct {
	IgnoreErrors   bool
	AnyErrorsFatal bool
	Tags           []string
	Register       string
	ChangedWhen    string
	FailedWhen     string
}

type Parser struct {
	variablePattern *regexp.Regexp
	functionPattern *regexp.Regexp
}

func NewParser() *Parser {
	return &Parser{
		variablePattern: regexp.MustCompile(`\{\{\s*([^\}]+?)\s*\}\}`),
		functionPattern: regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)\s*\(`),
	}
}

func (p *Parser) Parse(content string) (*ParsedPlaybook, error) {
	var raw Playbook
	if err := yaml.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if err := p.validatePlaybook(&raw); err != nil {
		return nil, fmt.Errorf("invalid playbook: %w", err)
	}

	parsed := &ParsedPlaybook{
		Raw:       &raw,
		Variables: p.processVariables(raw.Vars),
		Tasks:     make([]*ParsedTask, 0),
		PreTasks:  make([]*ParsedTask, 0),
		PostTasks: make([]*ParsedTask, 0),
	}

	for i := range raw.PreTasks {
		task, err := p.parseTask(&raw.PreTasks[i])
		if err != nil {
			return nil, fmt.Errorf("failed to parse pre_task[%d]: %w", i, err)
		}
		parsed.PreTasks = append(parsed.PreTasks, task)
	}

	for i := range raw.Tasks {
		task, err := p.parseTask(&raw.Tasks[i])
		if err != nil {
			return nil, fmt.Errorf("failed to parse task[%d]: %w", i, err)
		}
		parsed.Tasks = append(parsed.Tasks, task)
	}

	for i := range raw.PostTasks {
		task, err := p.parseTask(&raw.PostTasks[i])
		if err != nil {
			return nil, fmt.Errorf("failed to parse post_task[%d]: %w", i, err)
		}
		parsed.PostTasks = append(parsed.PostTasks, task)
	}

	return parsed, nil
}

func (p *Parser) parseTask(raw *PlaybookTask) (*ParsedTask, error) {
	task := &ParsedTask{
		Raw:  raw,
		Name: raw.Name,
		Args: raw.Args,
		Options: TaskOptions{
			IgnoreErrors:   raw.IgnoreErrors,
			AnyErrorsFatal: raw.AnyErrorsFatal,
			Tags:           raw.Tags,
			Register:       raw.Register,
			ChangedWhen:    raw.ChangedWhen,
			FailedWhen:     raw.FailedWhen,
		},
	}

	if raw.Action != "" {
		task.Action = p.extractAction(raw.Action)
	}

	if raw.When != "" {
		condition, err := p.parseCondition(raw.When)
		if err != nil {
			return nil, err
		}
		task.Condition = condition
	}

	if len(raw.WithItems) > 0 {
		task.Loop = &Loop{
			Items:   raw.WithItems,
			VarName: "item",
		}
	}

	task.ActionOpts = p.parseActionOptions(raw)

	return task, nil
}

func (p *Parser) parseActionOptions(raw *PlaybookTask) *ActionOptions {
	opts := &ActionOptions{}

	if raw.Timeout != nil {
		opts.Timeout = &TimeoutOption{}
		if raw.Timeout.Connect != "" {
			if d, err := time.ParseDuration(raw.Timeout.Connect); err == nil {
				opts.Timeout.Connect = d
			}
		}
		if raw.Timeout.Command != "" {
			if d, err := time.ParseDuration(raw.Timeout.Command); err == nil {
				opts.Timeout.Command = d
			}
		}
	}

	if raw.Retry != nil && raw.Retry.Max > 0 {
		opts.Retry = &RetryOption{
			Max: raw.Retry.Max,
		}
		if raw.Retry.Interval != "" {
			if d, err := time.ParseDuration(raw.Retry.Interval); err == nil {
				opts.Retry.Interval = d
			}
		}
		if raw.Retry.MaxInterval != "" {
			if d, err := time.ParseDuration(raw.Retry.MaxInterval); err == nil {
				opts.Retry.MaxInterval = d
			}
		}
	}

	if opts.Timeout == nil && opts.Retry == nil {
		return nil
	}

	return opts
}

func (p *Parser) extractAction(action string) string {
	parts := strings.SplitN(action, " ", 2)
	if len(parts) > 0 {
		return strings.TrimSpace(parts[0])
	}
	return action
}

func (p *Parser) parseCondition(when string) (*Condition, error) {
	variables := p.extractVariables(when)
	return &Condition{
		Expression: when,
		Variables:  variables,
	}, nil
}

func (p *Parser) extractVariables(expr string) []string {
	matches := p.variablePattern.FindAllStringSubmatch(expr, -1)
	vars := make([]string, 0, len(matches))
	seen := make(map[string]bool)

	for _, match := range matches {
		if len(match) > 1 {
			varName := strings.TrimSpace(match[1])
			if !seen[varName] {
				vars = append(vars, varName)
				seen[varName] = true
			}
		}
	}

	return vars
}

func (p *Parser) processVariables(vars map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range vars {
		result[k] = p.processVariableValue(v)
	}
	return result
}

func (p *Parser) processVariableValue(v interface{}) interface{} {
	switch val := v.(type) {
	case string:
		return p.interpolateString(val)
	case map[string]interface{}:
		result := make(map[string]interface{})
		for k, v := range val {
			result[k] = p.processVariableValue(v)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, v := range val {
			result[i] = p.processVariableValue(v)
		}
		return result
	default:
		return val
	}
}

func (p *Parser) interpolateString(s string) string {
	return p.variablePattern.ReplaceAllStringFunc(s, func(match string) string {
		parts := p.variablePattern.FindStringSubmatch(match)
		if len(parts) > 1 {
			return match
		}
		return match
	})
}

func (p *Parser) validatePlaybook(playbook *Playbook) error {
	if len(playbook.Hosts) == 0 {
		return fmt.Errorf("hosts cannot be empty")
	}
	return nil
}

func (p *Parser) Validate(parsed *ParsedPlaybook) []error {
	var errors []error

	if parsed == nil {
		return []error{fmt.Errorf("parsed playbook is nil")}
	}

	if len(parsed.Tasks) == 0 && len(parsed.PreTasks) == 0 && len(parsed.PostTasks) == 0 {
		errors = append(errors, fmt.Errorf("playbook has no tasks"))
	}

	for i, task := range parsed.Tasks {
		if task.Name == "" {
			errors = append(errors, fmt.Errorf("task[%d] has no name", i))
		}
		if task.Action == "" && len(task.Args) == 0 {
			errors = append(errors, fmt.Errorf("task[%d] '%s' has no action or args", i, task.Name))
		}
	}

	return errors
}

type TemplateEngine struct {
	variables       map[string]interface{}
	variablePattern *regexp.Regexp
}

func NewTemplateEngine(vars map[string]interface{}) *TemplateEngine {
	return &TemplateEngine{
		variables:       vars,
		variablePattern: regexp.MustCompile(`\{\{\s*([^\}]+?)\s*\}\}`),
	}
}

func (e *TemplateEngine) Render(template string) (string, error) {
	result := e.variablePattern.ReplaceAllStringFunc(template, func(match string) string {
		parts := e.variablePattern.FindStringSubmatch(match)
		if len(parts) > 1 {
			varName := strings.TrimSpace(parts[1])
			if val, ok := e.variables[varName]; ok {
				return fmt.Sprintf("%v", val)
			}
		}
		return match
	})
	return result, nil
}

func (e *TemplateEngine) SetVariable(name string, value interface{}) {
	if e.variables == nil {
		e.variables = make(map[string]interface{})
	}
	e.variables[name] = value
}

func (e *TemplateEngine) GetVariable(name string) (interface{}, bool) {
	val, ok := e.variables[name]
	return val, ok
}

func (e *TemplateEngine) MergeVariables(vars map[string]interface{}) {
	if e.variables == nil {
		e.variables = make(map[string]interface{})
	}
	for k, v := range vars {
		e.variables[k] = v
	}
}

type ConditionEvaluator struct {
	engine *TemplateEngine
}

func NewConditionEvaluator(vars map[string]interface{}) *ConditionEvaluator {
	return &ConditionEvaluator{
		engine: NewTemplateEngine(vars),
	}
}

func (e *ConditionEvaluator) Evaluate(condition *Condition) (bool, error) {
	if condition == nil {
		return true, nil
	}

	expression, err := e.engine.Render(condition.Expression)
	if err != nil {
		return false, fmt.Errorf("failed to render condition: %w", err)
	}

	return e.evaluateExpression(expression)
}

func (e *ConditionEvaluator) evaluateExpression(expr string) (bool, error) {
	expr = strings.TrimSpace(expr)

	if strings.HasPrefix(expr, "not ") {
		subExpr := strings.TrimPrefix(expr, "not ")
		result, err := e.evaluateExpression(subExpr)
		return !result, err
	}

	if strings.Contains(expr, " and ") {
		parts := strings.Split(expr, " and ")
		for _, part := range parts {
			result, err := e.evaluateExpression(strings.TrimSpace(part))
			if err != nil {
				return false, err
			}
			if !result {
				return false, nil
			}
		}
		return true, nil
	}

	if strings.Contains(expr, " or ") {
		parts := strings.Split(expr, " or ")
		for _, part := range parts {
			result, err := e.evaluateExpression(strings.TrimSpace(part))
			if err != nil {
				return false, err
			}
			if result {
				return true, nil
			}
		}
		return false, nil
	}

	return e.evaluateSimpleCondition(expr)
}

func (e *ConditionEvaluator) evaluateSimpleCondition(cond string) (bool, error) {
	cond = strings.TrimSpace(cond)

	if strings.HasPrefix(cond, "\"") && strings.HasSuffix(cond, "\"") {
		return cond[1:len(cond)-1] != "", nil
	}
	if strings.HasPrefix(cond, "'") && strings.HasSuffix(cond, "'") {
		return cond[1:len(cond)-1] != "", nil
	}

	if val, ok := e.engine.GetVariable(cond); ok {
		if b, ok := val.(bool); ok {
			return b, nil
		}
		return val != nil && val != "" && val != 0, nil
	}

	parts := strings.SplitN(cond, " ", 3)
	if len(parts) >= 3 {
		left := strings.TrimSpace(parts[0])
		operator := strings.TrimSpace(parts[1])
		right := strings.TrimSpace(parts[2])

		leftVal, leftOk := e.engine.GetVariable(left)
		rightVal, rightOk := e.engine.GetVariable(right)
		if !leftOk {
			leftVal = left
		}
		if !rightOk {
			rightVal = right
		}

		return e.compareValues(leftVal, operator, rightVal)
	}

	return false, fmt.Errorf("cannot evaluate condition: %s", cond)
}

func (e *ConditionEvaluator) compareValues(left interface{}, operator string, right interface{}) (bool, error) {
	switch operator {
	case "==", "===":
		return fmt.Sprintf("%v", left) == fmt.Sprintf("%v", right), nil
	case "!=", "!==":
		return fmt.Sprintf("%v", left) != fmt.Sprintf("%v", right), nil
	case ">":
		return e.compareNumeric(left, right) > 0, nil
	case ">=":
		return e.compareNumeric(left, right) >= 0, nil
	case "<":
		return e.compareNumeric(left, right) < 0, nil
	case "<=":
		return e.compareNumeric(left, right) <= 0, nil
	default:
		return false, fmt.Errorf("unknown operator: %s", operator)
	}
}

func (e *ConditionEvaluator) compareNumeric(left, right interface{}) float64 {
	var leftVal, rightVal float64
	if l, ok := toFloat64(left); ok {
		leftVal = l
	}
	if r, ok := toFloat64(right); ok {
		rightVal = r
	}
	return leftVal - rightVal
}

func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case string:
		var f float64
		n, err := fmt.Sscanf(val, "%f", &f)
		if err != nil || n == 0 {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}
