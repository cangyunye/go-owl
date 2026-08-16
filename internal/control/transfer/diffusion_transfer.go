package transfer

import (
	"time"
)

type DiffusionTransferStatus string

const (
	DiffusionStatusPending     DiffusionTransferStatus = "pending"
	DiffusionStatusInProgress  DiffusionTransferStatus = "in_progress"
	DiffusionStatusCompleted   DiffusionTransferStatus = "completed"
	DiffusionStatusPartialFail DiffusionTransferStatus = "partial_failure"
	DiffusionStatusFailed      DiffusionTransferStatus = "failed"
)

type NodeDiffusionStatus struct {
	NodeID      string
	ParentID    string
	Children    []string
	Status      DiffusionTransferStatus
	IsSource    bool
	Progress    float64
	Error       string
	StartTime   time.Time
	EndTime     *time.Time
	ChunksSent  int64
	ChunksTotal int64
}

type DiffusionTransfer struct {
	TaskID       string
	FileName     string
	FileSize     int64
	FileHash     string
	SourcePath   string
	DestPath     string
	Tree         *DiffusionTree
	NodeStatuses map[string]*NodeDiffusionStatus
	Status       DiffusionTransferStatus
	FanOutK      int
	Threshold    int
	Error        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	CompletedAt  *time.Time
}

func NewDiffusionTransfer(taskID, fileName, sourcePath, destPath string, fileSize int64, fileHash string, tree *DiffusionTree) *DiffusionTransfer {
	return &DiffusionTransfer{
		TaskID:       taskID,
		FileName:     fileName,
		FileSize:     fileSize,
		FileHash:     fileHash,
		SourcePath:   sourcePath,
		DestPath:     destPath,
		Tree:         tree,
		NodeStatuses: make(map[string]*NodeDiffusionStatus),
		Status:       DiffusionStatusPending,
		FanOutK:      tree.FanOutK,
		Threshold:    tree.Threshold,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
}

func (d *DiffusionTransfer) InitializeStatuses() {
	for nodeID, treeNode := range d.Tree.Nodes {
		status := &NodeDiffusionStatus{
			NodeID:      nodeID,
			ParentID:    treeNode.ParentID,
			Children:    treeNode.Children,
			Status:      DiffusionStatusPending,
			IsSource:    treeNode.ParentID == "" && nodeID != d.Tree.Root,
			Progress:    0,
			ChunksTotal: calculateChunks(d.FileSize, DefaultChunkSizeTransfer),
			StartTime:   time.Now(),
		}
		if nodeID == d.Tree.Root {
			status.IsSource = true
		}
		d.NodeStatuses[nodeID] = status
	}
}

func (d *DiffusionTransfer) UpdateNodeStatus(nodeID string, status DiffusionTransferStatus, progress float64, errorMsg string) {
	if nodeStatus, ok := d.NodeStatuses[nodeID]; ok {
		nodeStatus.Status = status
		nodeStatus.Progress = progress
		nodeStatus.Error = errorMsg
		if status == DiffusionStatusCompleted || status == DiffusionStatusFailed {
			now := time.Now()
			nodeStatus.EndTime = &now
		}
	}
	d.UpdatedAt = time.Now()
	d.recalculateStatus()
}

func (d *DiffusionTransfer) recalculateStatus() {
	completed := 0
	failed := 0
	total := len(d.NodeStatuses)

	for _, status := range d.NodeStatuses {
		if status.Status == DiffusionStatusCompleted {
			completed++
		} else if status.Status == DiffusionStatusFailed {
			failed++
		}
	}

	if completed+failed == total {
		if failed == 0 {
			d.Status = DiffusionStatusCompleted
			now := time.Now()
			d.CompletedAt = &now
		} else if completed == 0 {
			d.Status = DiffusionStatusFailed
		} else {
			d.Status = DiffusionStatusPartialFail
			now := time.Now()
			d.CompletedAt = &now
		}
	} else if completed > 0 || failed > 0 {
		d.Status = DiffusionStatusInProgress
	}
}

func (d *DiffusionTransfer) ShouldUseDiffusion() bool {
	return len(d.Tree.Nodes) >= d.Threshold
}

func calculateChunks(fileSize, chunkSize int64) int64 {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSizeTransfer
	}
	if fileSize <= 0 {
		return 1
	}
	return (fileSize + chunkSize - 1) / chunkSize
}
