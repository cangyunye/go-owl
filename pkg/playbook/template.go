package playbook

import (
	"gopkg.in/yaml.v3"
)

type ActionTemplate struct {
	Name        string
	Description string
	Template    map[string]interface{}
}

func GetActionTemplates() []ActionTemplate {
	return actionTemplates
}

var actionTemplates = []ActionTemplate{
	{
		Name:        "command",
		Description: "执行 Shell 命令",
		Template:    map[string]interface{}{"cmd": "<命令内容>"},
	},
	{
		Name:        "script",
		Description: "执行脚本文件",
		Template:    map[string]interface{}{"script": "<脚本路径>", "dest": "/tmp/", "args": ""},
	},
	{
		Name:        "upload",
		Description: "上传文件到节点",
		Template:    map[string]interface{}{"src": "<本地路径>", "dest": "<远程路径>", "overwrite": true},
	},
	{
		Name:        "download",
		Description: "从节点下载文件",
		Template:    map[string]interface{}{"src": "<远程路径>", "dest": "<本地路径>", "subdir": true},
	},
	{
		Name:        "include",
		Description: "包含其他剧本",
		Template:    map[string]interface{}{"playbook": "<剧本路径>"},
	},
}

type TemplateDefaultConfig struct {
	Groups   []string `yaml:"groups,omitempty"`
	Tags     []string `yaml:"tags,omitempty"`
	SkipTags []string `yaml:"skip_tags,omitempty"`
}

type TemplatePlaybook struct {
	Name          string                 `yaml:"name"`
	Description   string                 `yaml:"description,omitempty"`
	Version       string                 `yaml:"version,omitempty"`
	Hosts         []string               `yaml:"hosts"`
	ExecutionMode string                 `yaml:"execution_mode,omitempty"`
	Default       *TemplateDefaultConfig `yaml:"default,omitempty"`
	Vars          map[string]interface{} `yaml:"vars,omitempty"`
	PreTasks      []TemplateTask         `yaml:"pre_tasks"`
	Tasks         []TemplateTask         `yaml:"tasks"`
	PostTasks     []TemplateTask         `yaml:"post_tasks"`
}

type TemplateTask struct {
	Name   string                 `yaml:"name"`
	Action string                 `yaml:"action"`
	Args   map[string]interface{} `yaml:"args"`
}

func RenderTemplateYAML(tpl *TemplatePlaybook) ([]byte, error) {
	return yaml.Marshal(tpl)
}
