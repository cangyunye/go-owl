package monitor

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"
)

// SyntaxGate 语法闸门：AI 生成的脚本/剧本执行前必须通过语法校验。
// 校验失败（或校验器不可用）时失败关闭——绝不执行无法验证的内容。
type SyntaxGate struct {
	bashPath string // 测试可注入
}

// NewSyntaxGate 创建语法闸门（默认使用系统 bash）。
func NewSyntaxGate() *SyntaxGate {
	return &SyntaxGate{bashPath: "bash"}
}

// Validate 按对策类型校验内容：script → bash -n；playbook → YAML 解析；
// sop 等无执行内容 → 直接通过。
func (g *SyntaxGate) Validate(kind, content string) (ok bool, reason string) {
	switch kind {
	case "script":
		return g.validateBash(content)
	case "playbook":
		return g.validateYAML(content)
	default:
		return true, ""
	}
}

// validateBash 用 bash -n 校验脚本语法（stdin 传入，不执行）。
func (g *SyntaxGate) validateBash(content string) (bool, string) {
	cmd := exec.Command(g.bashPath, "-n")
	cmd.Stdin = strings.NewReader(content)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if _, lookupErr := exec.LookPath(g.bashPath); lookupErr != nil {
			return false, fmt.Sprintf("bash 不可用（%s），语法校验失败关闭", g.bashPath)
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return false, "bash -n 校验失败: " + msg
	}
	return true, ""
}

// validateYAML 用 yaml.v3 解析校验剧本结构。
func (g *SyntaxGate) validateYAML(content string) (bool, string) {
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(content), &node); err != nil {
		return false, "YAML 解析失败: " + err.Error()
	}
	if node.Kind == 0 {
		return false, "YAML 内容为空"
	}
	return true, ""
}
