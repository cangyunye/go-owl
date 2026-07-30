package playbook

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type TemplateParameter struct {
	Name        string        `yaml:"name"`
	Description string        `yaml:"description"`
	Type        string        `yaml:"type"`
	Required    bool          `yaml:"required"`
	Default     interface{}   `yaml:"default"`
	Options     []interface{} `yaml:"options"`
	Pattern     string        `yaml:"pattern"`
}

type TemplateMeta struct {
	Description string              `yaml:"description"`
	Tags        []string            `yaml:"tags"`
	Parameters  []TemplateParameter `yaml:"parameters"`
}

func ParseTemplateMeta(data []byte) (*TemplateMeta, error) {
	var meta TemplateMeta
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("解析模板元数据失败: %w", err)
	}
	return &meta, nil
}

func ValidateParams(params []TemplateParameter, provided map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	for _, p := range params {
		val, ok := provided[p.Name]
		if !ok {
			if p.Default != nil {
				val = p.Default
			} else if p.Required {
				return nil, fmt.Errorf("缺少必填参数: %s", p.Name)
			} else {
				continue
			}
		}

		if len(p.Options) > 0 {
			found := false
			valStr := fmt.Sprintf("%v", val)
			for _, opt := range p.Options {
				if fmt.Sprintf("%v", opt) == valStr {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("参数 %s 的值 %v 不在允许选项中", p.Name, val)
			}
		}

		if p.Pattern != "" {
			re, err := regexp.Compile(p.Pattern)
			if err != nil {
				return nil, fmt.Errorf("参数 %s 的正则表达式无效: %w", p.Name, err)
			}
			if !re.MatchString(fmt.Sprintf("%v", val)) {
				return nil, fmt.Errorf("参数 %s 的值 %v 不匹配格式要求", p.Name, val)
			}
		}

		result[p.Name] = val
	}

	return result, nil
}

func Instantiate(templateData []byte, vars map[string]interface{}) ([]byte, error) {
	var raw map[string]interface{}
	if err := yaml.Unmarshal(templateData, &raw); err != nil {
		return nil, fmt.Errorf("解析模板失败: %w", err)
	}

	delete(raw, "parameters")
	delete(raw, "description")
	delete(raw, "tags")

	out, err := yaml.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("序列化模板失败: %w", err)
	}

	content := string(out)
	for k, v := range vars {
		re := regexp.MustCompile(`\{\{\s*` + regexp.QuoteMeta(k) + `\s*\}\}`)
		content = re.ReplaceAllString(content, fmt.Sprintf("%v", v))
	}

	return []byte(strings.TrimSpace(content) + "\n"), nil
}
