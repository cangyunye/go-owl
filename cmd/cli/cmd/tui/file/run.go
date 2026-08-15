package file

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"

	"github.com/cangyunye/go-owl/internal/control/transfer"
	"github.com/cangyunye/go-owl/internal/history"
	"github.com/cangyunye/go-owl/internal/node"
)

type UploadDoneMsg struct {
	Results []transfer.TransferResult
}

// uploadRun 可注入的上传执行器(测试替换);默认走 TransferManager + NodeResolver
var uploadRun = func(ctx context.Context, ids []string, localFile, remotePath string, opts *transfer.UploadOptions) []transfer.TransferResult {
	manager := transfer.NewTransferManager(node.NewNodeResolver())
	defer manager.Close()
	return manager.Upload(ctx, ids, localFile, remotePath, opts)
}

// history 记录可注入(测试替换);默认走 internal/history
var recordOperation = history.RecordOperation
var recordFileTransfer = history.RecordFileTransfer

type uploadRunState struct {
	ids        []string
	localFile  string
	remotePath string
	opts       *transfer.UploadOptions
}

func (m *FileModel) startUpload() (tea.Cmd, error) {
	local := strings.TrimSpace(m.fileInput.Value())
	if local == "" {
		return nil, errors.New("文件路径不能为空")
	}
	if _, err := os.Stat(local); err != nil {
		return nil, errors.New("本地文件不存在: " + local)
	}
	nodes, err := m.resolveTargets()
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, errors.New("没有目标节点")
	}
	f := m.advanced
	if f == nil {
		f = newAdvancedForm()
	}
	ids := make([]string, 0, len(nodes))
	for _, n := range nodes {
		ids = append(ids, n.ID)
	}
	dest := strings.TrimSpace(m.destInput.Value())
	if dest == "" {
		dest = "/tmp"
	}
	remotePath := dest
	if remotePath[len(remotePath)-1] != '/' {
		remotePath += "/"
	}
	remotePath += fileNameFromPath(local)

	m.lastUpload = &uploadRunState{ids: ids, localFile: local, remotePath: remotePath, opts: f.uploadOpts()}
	m.results = nil
	m.loading = true
	return m.launchUpload(), nil
}

func (m *FileModel) launchUpload() tea.Cmd {
	s := m.lastUpload
	return func() tea.Msg {
		taskID := uuid.New().String()
		start := time.Now()
		meta, _ := json.Marshal(map[string]string{
			"local_path":  s.localFile,
			"remote_path": s.remotePath,
		})
		recordOperation(&history.Operation{
			TaskID:    taskID,
			OpType:    "file_transfer",
			Command:   string(meta),
			Targets:   s.ids,
			Status:    "running",
			CreatedAt: start,
		})
		results := uploadRun(context.Background(), s.ids, s.localFile, s.remotePath, s.opts)
		fileInfo, _ := os.Stat(s.localFile)
		fileSize := int64(0)
		if fileInfo != nil {
			fileSize = fileInfo.Size()
		}
		success, failed := 0, 0
		for _, r := range results {
			status := "completed"
			errMsg := ""
			if r.Error != nil {
				status = "failed"
				errMsg = r.Error.Error()
				failed++
			} else {
				success++
			}
			method := r.Method
			if method == "" {
				method = "scp"
			}
			recordFileTransfer(&history.FileTransfer{
				TaskID:       taskID,
				NodeID:       r.NodeID,
				FileName:     fileNameFromPath(s.localFile),
				FileSize:     fileSize,
				TransferType: method,
				Status:       status,
				Progress:     100.0,
				Error:        errMsg,
				CreatedAt:    time.Now(),
			})
		}
		finalStatus := "completed"
		if failed > 0 {
			if success == 0 {
				finalStatus = "failed"
			} else {
				finalStatus = "partial_failure"
			}
		}
		recordOperation(&history.Operation{
			TaskID:    taskID,
			OpType:    "file_transfer",
			Command:   string(meta),
			Targets:   s.ids,
			Status:    finalStatus,
			CreatedAt: start,
		})
		return UploadDoneMsg{Results: results}
	}
}

func fileNameFromPath(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[i+1:]
		}
	}
	return path
}
