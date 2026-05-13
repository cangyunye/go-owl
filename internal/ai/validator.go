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
	targets, ok := params["targets"]
	if !ok {
		return fmt.Errorf("targets is required")
	}
	targetList, ok := targets.([]interface{})
	if !ok || len(targetList) == 0 {
		return fmt.Errorf("targets must be a non-empty array")
	}
	for i, t := range targetList {
		if _, ok := t.(string); !ok {
			return fmt.Errorf("target at index %d must be a string", i)
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

	if timeout, ok := params["timeout"]; ok {
		switch t := timeout.(type) {
		case float64:
			if t < 1 || t > 3600 {
				return fmt.Errorf("timeout must be between 1 and 3600 seconds")
			}
		case int:
			if t < 1 || t > 3600 {
				return fmt.Errorf("timeout must be between 1 and 3600 seconds")
			}
		default:
			return fmt.Errorf("timeout must be a number")
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

	targets, ok := params["targets"]
	if !ok {
		return fmt.Errorf("targets is required")
	}
	targetList, ok := targets.([]interface{})
	if !ok || len(targetList) == 0 {
		return fmt.Errorf("targets must be a non-empty array")
	}
	for i, t := range targetList {
		if _, ok := t.(string); !ok {
			return fmt.Errorf("target at index %d must be a string", i)
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

func (v *Validator) ValidateParams(intent IntentType, params map[string]interface{}) error {
	switch intent {
	case IntentQueryNodes:
		return v.ValidateQueryNodes(params)
	case IntentExecuteCmd:
		return v.ValidateExecuteCommand(params)
	case IntentGeneratePlaybook:
		return v.ValidateGeneratePlaybook(params)
	case IntentTransferFile:
		return v.ValidateTransferFile(params)
	default:
		return fmt.Errorf("unknown intent type")
	}
}
