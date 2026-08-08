package ai

import (
	"context"
	"fmt"
	"strings"
)

type FileDownloadTool struct {
	executor Executor
}

func NewFileDownloadTool(executor Executor) *FileDownloadTool {
	return &FileDownloadTool{executor: executor}
}

func (t *FileDownloadTool) Name() string        { return "file_download" }
func (t *FileDownloadTool) Description() string { return "Download files from remote nodes to the local machine." }
func (t *FileDownloadTool) Parameters() string  { return fileDownloadParamsSchema }
func (t *FileDownloadTool) Validate(p map[string]interface{}) error {
	if strings.TrimSpace(strOf(p["remote_file"])) == "" {
		return fmt.Errorf("remote_file is required")
	}
	nodes := strSliceOf(p["nodes"])
	if len(nodes) == 0 && strOf(p["group"]) == "" && strOf(p["label"]) == "" && strOf(p["node"]) == "" {
		return fmt.Errorf("must provide nodes, group, label or node")
	}
	return nil
}

const fileDownloadParamsSchema = `{
	"type": "object",
	"properties": {
		"remote_file": {"type": "string", "description": "Remote file path on target nodes"},
		"nodes": {"type": "array", "items": {"type": "string"}, "description": "Target node name list"},
		"node": {"type": "string", "description": "Single target node"},
		"group": {"type": "string", "description": "Target group"},
		"label": {"type": "string", "description": "Target label, e.g. env=prod"},
		"dest": {"type": "string", "description": "Local destination directory, default ."},
		"subdir": {"type": "boolean", "description": "Save into per-node subdirectories"},
		"resume": {"type": "boolean", "description": "Resume interrupted downloads, default true"}
	},
	"required": ["remote_file"]
}`

func (t *FileDownloadTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor != nil {
		p := FileDownloadParams{
			RemoteFile: strOf(params["remote_file"]),
			Group:      strOf(params["group"]),
			Label:      strOf(params["label"]),
			Dest:       strOf(params["dest"]),
			Subdir:     boolOf(params["subdir"]),
			Resume:     boolOf(params["resume"]),
		}
		p.Nodes = strSliceOf(params["nodes"])
		if len(p.Nodes) == 0 && strOf(params["node"]) != "" {
			p.Nodes = []string{strOf(params["node"])}
		}
		result, err := t.executor.FileDownload(ctx, p)
		if err == nil {
			return result.Text, nil
		}
	}
	return "", fmt.Errorf("file_download failed")
}
