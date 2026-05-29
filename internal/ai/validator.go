package ai

import (
	"fmt"
	"path/filepath"
	"strings"
)

type Validator struct{}

func NewValidator() *Validator {
	return &Validator{}
}

func (v *Validator) ValidateQueryNodes(params map[string]interface{}) error {
	if group, ok := params["group"]; ok {
		if _, ok := group.(string); !ok {
			return fmt.Errorf("group must be a string")
		}
	}
	if labels, ok := params["labels"]; ok {
		if _, ok := labels.(map[string]interface{}); !ok {
			return fmt.Errorf("labels must be an object")
		}
	}
	if status, ok := params["status"]; ok {
		statusStr, ok := status.(string)
		if !ok {
			return fmt.Errorf("status must be a string")
		}
		validStatuses := []string{"online", "offline", "unknown"}
		found := false
		for _, s := range validStatuses {
			if strings.EqualFold(statusStr, s) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("invalid status: %s, must be one of: %v", statusStr, validStatuses)
		}
	}
	if format, ok := params["format"]; ok {
		formatStr, ok := format.(string)
		if !ok {
			return fmt.Errorf("format must be a string")
		}
		validFormats := []string{"table", "json", "summary"}
		found := false
		for _, f := range validFormats {
			if strings.EqualFold(formatStr, f) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("invalid format: %s, must be one of: %v", formatStr, validFormats)
		}
	}
	return nil
}

func (v *Validator) ValidateExecuteCommand(params map[string]interface{}) error {
	if nodes, ok := params["nodes"]; ok {
		nodeList, ok := nodes.([]interface{})
		if !ok {
			return fmt.Errorf("nodes must be an array")
		}
		if len(nodeList) == 0 {
			return fmt.Errorf("nodes must be a non-empty array")
		}
		for i, n := range nodeList {
			if _, ok := n.(string); !ok {
				return fmt.Errorf("node at index %d must be a string", i)
			}
		}
	}

	command, ok := params["command"]
	if !ok {
		return fmt.Errorf("command is required")
	}
	cmdStr, ok := command.(string)
	if !ok || cmdStr == "" {
		return fmt.Errorf("command must be a non-empty string")
	}

	if group, ok := params["group"]; ok {
		if _, ok := group.(string); !ok {
			return fmt.Errorf("group must be a string")
		}
	}

	if label, ok := params["label"]; ok {
		if _, ok := label.(string); !ok {
			return fmt.Errorf("label must be a string")
		}
	}

	if timeout, ok := params["timeout"]; ok {
		switch tv := timeout.(type) {
		case float64:
			if tv < 1 || tv > 3600 {
				return fmt.Errorf("timeout must be between 1 and 3600 seconds")
			}
		case int:
			if tv < 1 || tv > 3600 {
				return fmt.Errorf("timeout must be between 1 and 3600 seconds")
			}
		default:
			return fmt.Errorf("timeout must be a number")
		}
	}

	if format, ok := params["format"]; ok {
		formatStr, ok := format.(string)
		if !ok {
			return fmt.Errorf("format must be a string")
		}
		validFormats := []string{"simple", "detail", "json"}
		found := false
		for _, f := range validFormats {
			if strings.EqualFold(formatStr, f) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("invalid format: %s, must be one of: %v", formatStr, validFormats)
		}
	}

	if mode, ok := params["mode"]; ok {
		modeStr, ok := mode.(string)
		if !ok {
			return fmt.Errorf("mode must be a string")
		}
		validModes := []string{"parallel", "serial", "async"}
		found := false
		for _, m := range validModes {
			if strings.EqualFold(modeStr, m) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("invalid mode: %s, must be one of: %v", modeStr, validModes)
		}
	}

	return nil
}

func (v *Validator) ValidateGeneratePlaybook(params map[string]interface{}) error {
	requirement, ok := params["requirement"]
	if !ok {
		return fmt.Errorf("requirement is required")
	}
	reqStr, ok := requirement.(string)
	if !ok || reqStr == "" {
		return fmt.Errorf("requirement must be a non-empty string")
	}

	if vars, ok := params["vars"]; ok {
		if _, ok := vars.(map[string]interface{}); !ok {
			return fmt.Errorf("vars must be an object")
		}
	}

	return nil
}

func (v *Validator) ValidateTransferFile(params map[string]interface{}) error {
	sourceFile, ok := params["source_file"]
	if !ok {
		return fmt.Errorf("source_file is required")
	}
	srcStr, ok := sourceFile.(string)
	if !ok || srcStr == "" {
		return fmt.Errorf("source_file must be a non-empty string")
	}

	nodes, ok := params["nodes"]
	if !ok {
		return fmt.Errorf("nodes is required")
	}
	nodeList, ok := nodes.([]interface{})
	if !ok || len(nodeList) == 0 {
		return fmt.Errorf("nodes must be a non-empty array")
	}
	for i, n := range nodeList {
		if _, ok := n.(string); !ok {
			return fmt.Errorf("node at index %d must be a string", i)
		}
	}

	destDir, ok := params["dest_dir"]
	if !ok {
		return fmt.Errorf("dest_dir is required")
	}
	destStr, ok := destDir.(string)
	if !ok || destStr == "" {
		return fmt.Errorf("dest_dir must be a non-empty string")
	}
	if !filepath.IsAbs(destStr) {
		return fmt.Errorf("dest_dir must be an absolute path")
	}

	if mode, ok := params["mode"]; ok {
		modeStr, ok := mode.(string)
		if !ok {
			return fmt.Errorf("mode must be a string")
		}
		validModes := []string{"direct", "diffusion", "auto"}
		found := false
		for _, m := range validModes {
			if strings.EqualFold(modeStr, m) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("invalid mode: %s, must be one of: %v", modeStr, validModes)
		}
	}

	if permission, ok := params["permission"]; ok {
		permStr, ok := permission.(string)
		if !ok {
			return fmt.Errorf("permission must be a string")
		}
		if !strings.HasPrefix(permStr, "0") || len(permStr) != 4 {
			return fmt.Errorf("permission must be a 4-digit octal string like 0644")
		}
	}

	return nil
}

func (v *Validator) ValidateExecuteScript(params map[string]interface{}) error {
	script, ok := params["script"]
	if !ok {
		return fmt.Errorf("script is required")
	}
	scriptStr, ok := script.(string)
	if !ok || scriptStr == "" {
		return fmt.Errorf("script must be a non-empty string")
	}

	if nodes, ok := params["nodes"]; ok {
		nodeList, ok := nodes.([]interface{})
		if !ok {
			return fmt.Errorf("nodes must be an array")
		}
		if len(nodeList) == 0 {
			return fmt.Errorf("nodes must be a non-empty array")
		}
		for i, t := range nodeList {
			if _, ok := t.(string); !ok {
				return fmt.Errorf("node at index %d must be a string", i)
			}
		}
	}

	if group, ok := params["group"]; ok {
		if _, ok := group.(string); !ok {
			return fmt.Errorf("group must be a string")
		}
	}

	if label, ok := params["label"]; ok {
		if _, ok := label.(string); !ok {
			return fmt.Errorf("label must be a string")
		}
	}

	if dest, ok := params["dest"]; ok {
		destStr, ok := dest.(string)
		if !ok {
			return fmt.Errorf("dest must be a string")
		}
		if !filepath.IsAbs(destStr) {
			return fmt.Errorf("dest must be an absolute path")
		}
	}

	if args, ok := params["args"]; ok {
		if _, ok := args.(string); !ok {
			return fmt.Errorf("args must be a string")
		}
	}

	if timeout, ok := params["timeout"]; ok {
		switch tv := timeout.(type) {
		case float64:
			if tv < 1 || tv > 3600 {
				return fmt.Errorf("timeout must be between 1 and 3600 seconds")
			}
		case int:
			if tv < 1 || tv > 3600 {
				return fmt.Errorf("timeout must be between 1 and 3600 seconds")
			}
		default:
			return fmt.Errorf("timeout must be a number")
		}
	}

	if inline, ok := params["inline"]; ok {
		if _, ok := inline.(bool); !ok {
			return fmt.Errorf("inline must be a boolean")
		}
	}

	if keep, ok := params["keep"]; ok {
		if _, ok := keep.(bool); !ok {
			return fmt.Errorf("keep must be a boolean")
		}
	}

	return nil
}

func (v *Validator) ValidateParams(intent IntentType, params map[string]interface{}) error {
	switch intent {
	case IntentQueryNodes:
		return v.ValidateQueryNodes(params)
	case IntentExecuteCmd:
		return v.ValidateExecuteCommand(params)
	case IntentExecuteScript:
		return v.ValidateExecuteScript(params)
	case IntentGeneratePlaybook:
		return v.ValidateGeneratePlaybook(params)
	case IntentTransferFile:
		return v.ValidateTransferFile(params)
	default:
		return fmt.Errorf("unknown intent type")
	}
}
