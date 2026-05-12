package transfer

import (
	"fmt"
	"sync"
	"time"

	"github.com/cangyunye/go-owl/internal/common/model"
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
	TaskID          string
	FileName        string
	FileSize        int64
	FileHash        string
	SourcePath      string
	DestPath        string
	Tree            *DiffusionTree
	NodeStatuses    map[string]*NodeDiffusionStatus
	Status          DiffusionTransferStatus
	FanOutK         int
	Threshold       int
	Error           string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	CompletedAt     *time.Time
}

func NewDiffusionTransfer(taskID, fileName, sourcePath, destPath string, fileSize int64, fileHash string, tree *DiffusionTree) *DiffusionTransfer {
	return &DiffusionTransfer{
		TaskID:        taskID,
		FileName:      fileName,
		FileSize:      fileSize,
		FileHash:      fileHash,
		SourcePath:    sourcePath,
		DestPath:      destPath,
		Tree:          tree,
		NodeStatuses:  make(map[string]*NodeDiffusionStatus),
		Status:        DiffusionStatusPending,
		FanOutK:       tree.FanOutK,
		Threshold:     tree.Threshold,
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

func (d *DiffusionTransfer) UpdateNodeProgress(nodeID string, chunksSent, chunksTotal int64) {
	if nodeStatus, ok := d.NodeStatuses[nodeID]; ok {
		nodeStatus.ChunksSent = chunksSent
		nodeStatus.ChunksTotal = chunksTotal
		if chunksTotal > 0 {
			nodeStatus.Progress = float64(chunksSent) / float64(chunksTotal) * 100
		}
	}
	d.UpdatedAt = time.Now()
}

func (d *DiffusionTransfer) MarkNodeAsSource(nodeID string) {
	if nodeStatus, ok := d.NodeStatuses[nodeID]; ok {
		nodeStatus.IsSource = true
	}
}

func (d *DiffusionTransfer) GetNodeStatus(nodeID string) (*NodeDiffusionStatus, bool) {
	status, ok := d.NodeStatuses[nodeID]
	return status, ok
}

func (d *DiffusionTransfer) GetPendingNodes() []string {
	var pending []string
	for nodeID, status := range d.NodeStatuses {
		if nodeID != d.Tree.Root && status.Status == DiffusionStatusPending {
			pending = append(pending, nodeID)
		}
	}
	return pending
}

func (d *DiffusionTransfer) GetAvailableSources() []string {
	var sources []string
	for nodeID, status := range d.NodeStatuses {
		if status.IsSource && status.Status == DiffusionStatusCompleted {
			sources = append(sources, nodeID)
		}
	}
	return sources
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

func (d *DiffusionTransfer) GetOverallProgress() float64 {
	if len(d.NodeStatuses) == 0 {
		return 0
	}

	totalProgress := 0.0
	for _, status := range d.NodeStatuses {
		if status.Status == DiffusionStatusCompleted {
			totalProgress += 100.0
		} else {
			totalProgress += status.Progress
		}
	}

	return totalProgress / float64(len(d.NodeStatuses))
}

func (d *DiffusionTransfer) GetSuccessCount() int {
	count := 0
	for _, status := range d.NodeStatuses {
		if status.Status == DiffusionStatusCompleted {
			count++
		}
	}
	return count
}

func (d *DiffusionTransfer) GetFailureCount() int {
	count := 0
	for _, status := range d.NodeStatuses {
		if status.Status == DiffusionStatusFailed {
			count++
		}
	}
	return count
}

func (d *DiffusionTransfer) ShouldUseDiffusion() bool {
	return len(d.Tree.Nodes) >= d.Threshold
}

func (d *DiffusionTransfer) GetSubTaskForNode(sourceNodeID string) (*SubTransferTask, error) {
	sourceStatus, ok := d.NodeStatuses[sourceNodeID]
	if !ok {
		return nil, fmt.Errorf("source node %s not found", sourceNodeID)
	}

	children := sourceStatus.Children
	if len(children) == 0 {
		return nil, fmt.Errorf("source node %s has no children to transfer to", sourceNodeID)
	}

	subTask := &SubTransferTask{
		ParentTaskID:  d.TaskID,
		SourceNodeID:  sourceNodeID,
		TargetNodeIDs: children,
		FileName:      d.FileName,
		FileSize:      d.FileSize,
		FileHash:      d.FileHash,
		DestPath:      d.DestPath,
		ChunkSize:     DefaultChunkSizeTransfer,
		Status:        DiffusionStatusPending,
		CreatedAt:     time.Now(),
	}

	return subTask, nil
}

type SubTransferTask struct {
	ParentTaskID  string
	SourceNodeID  string
	TargetNodeIDs []string
	FileName      string
	FileSize      int64
	FileHash      string
	DestPath      string
	ChunkSize     int64
	Status        DiffusionTransferStatus
	Progress      map[string]float64
	Error         string
	CreatedAt     time.Time
	CompletedAt   *time.Time
}

func NewSubTransferTask(parentTaskID, sourceNodeID string, targetNodeIDs []string, fileName string, fileSize int64, fileHash, destPath string) *SubTransferTask {
	progress := make(map[string]float64)
	for _, nodeID := range targetNodeIDs {
		progress[nodeID] = 0
	}

	return &SubTransferTask{
		ParentTaskID:  parentTaskID,
		SourceNodeID:  sourceNodeID,
		TargetNodeIDs: targetNodeIDs,
		FileName:      fileName,
		FileSize:      fileSize,
		FileHash:      fileHash,
		DestPath:      destPath,
		ChunkSize:     DefaultChunkSizeTransfer,
		Status:        DiffusionStatusPending,
		Progress:      progress,
		CreatedAt:     time.Now(),
	}
}

func (s *SubTransferTask) UpdateTargetProgress(nodeID string, progress float64) {
	s.Progress[nodeID] = progress
}

func (s *SubTransferTask) GetOverallProgress() float64 {
	if len(s.Progress) == 0 {
		return 0
	}

	total := 0.0
	for _, p := range s.Progress {
		total += p
	}
	return total / float64(len(s.Progress))
}

func (s *SubTransferTask) IsCompleted() bool {
	for _, p := range s.Progress {
		if p < 100 {
			return false
		}
	}
	return true
}

type DiffusionScheduler struct {
	mu         sync.RWMutex
	transfers  map[string]*DiffusionTransfer
	subTasks   map[string]*SubTransferTask
	treeBuilder TreeBuilder
}

func NewDiffusionScheduler() *DiffusionScheduler {
	return &DiffusionScheduler{
		transfers:  make(map[string]*DiffusionTransfer),
		subTasks:   make(map[string]*SubTransferTask),
		treeBuilder: NewTreeBuilder(DefaultFanOutK, DefaultMaxDepth, DefaultThreshold),
	}
}

func (s *DiffusionScheduler) CreateTransfer(taskID string, targets []*model.Node, sourcePath, destPath string) (*DiffusionTransfer, error) {
	tree := s.treeBuilder.Build(targets)

	transfer := NewDiffusionTransfer(
		taskID,
		"",
		sourcePath,
		destPath,
		0,
		"",
		tree,
	)

	transfer.InitializeStatuses()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.transfers[taskID] = transfer

	return transfer, nil
}

func (s *DiffusionScheduler) GetTransfer(taskID string) (*DiffusionTransfer, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	transfer, ok := s.transfers[taskID]
	return transfer, ok
}

func (s *DiffusionScheduler) UpdateTransferStatus(taskID string, status DiffusionTransferStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	transfer, ok := s.transfers[taskID]
	if !ok {
		return fmt.Errorf("transfer task %s not found", taskID)
	}

	transfer.Status = status
	transfer.UpdatedAt = time.Now()
	return nil
}

func (s *DiffusionScheduler) UpdateNodeStatus(taskID, nodeID string, status DiffusionTransferStatus, progress float64, errorMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	transfer, ok := s.transfers[taskID]
	if !ok {
		return fmt.Errorf("transfer task %s not found", taskID)
	}

	transfer.UpdateNodeStatus(nodeID, status, progress, errorMsg)
	return nil
}

func (s *DiffusionScheduler) ReassignFailedNode(taskID string, failedNodeID string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	transfer, ok := s.transfers[taskID]
	if !ok {
		return nil, fmt.Errorf("transfer task %s not found", taskID)
	}

	failedStatus, ok := transfer.NodeStatuses[failedNodeID]
	if !ok {
		return nil, fmt.Errorf("failed node %s not found", failedNodeID)
	}

	parentID := failedStatus.ParentID
	if parentID == "" {
		return nil, fmt.Errorf("cannot reassign root node")
	}

	parentStatus, ok := transfer.NodeStatuses[parentID]
	if !ok {
		return nil, fmt.Errorf("parent node %s not found", parentID)
	}

	children := make([]string, len(failedStatus.Children))
	copy(children, failedStatus.Children)

	for _, childID := range children {
		childStatus := transfer.NodeStatuses[childID]
		childStatus.ParentID = parentID
		parentStatus.Children = append(parentStatus.Children, childID)
	}

	parentStatus.Status = DiffusionStatusPending

	return children, nil
}

func (s *DiffusionScheduler) ListTransfers() []*DiffusionTransfer {
	s.mu.RLock()
	defer s.mu.RUnlock()

	transfers := make([]*DiffusionTransfer, 0, len(s.transfers))
	for _, transfer := range s.transfers {
		transfers = append(transfers, transfer)
	}
	return transfers
}

func (s *DiffusionScheduler) GetTransferReport(taskID string) (*TransferReport, error) {
	transfer, ok := s.GetTransfer(taskID)
	if !ok {
		return nil, fmt.Errorf("transfer task %s not found", taskID)
	}

	report := &TransferReport{
		TaskID:           taskID,
		FileName:         transfer.FileName,
		FileSize:         transfer.FileSize,
		TotalNodes:       len(transfer.NodeStatuses),
		SuccessCount:     transfer.GetSuccessCount(),
		FailureCount:     transfer.GetFailureCount(),
		OverallProgress:   transfer.GetOverallProgress(),
		Status:           string(transfer.Status),
		UseDiffusion:     transfer.ShouldUseDiffusion(),
		CreatedAt:        transfer.CreatedAt,
		CompletedAt:       transfer.CompletedAt,
		NodeReports:       make([]NodeReport, 0),
	}

	for nodeID, status := range transfer.NodeStatuses {
		report.NodeReports = append(report.NodeReports, NodeReport{
			NodeID:     nodeID,
			ParentID:   status.ParentID,
			IsSource:   status.IsSource,
			Status:     string(status.Status),
			Progress:   status.Progress,
			Error:      status.Error,
			StartTime:  status.StartTime,
			EndTime:    status.EndTime,
		})
	}

	return report, nil
}

type TransferReport struct {
	TaskID          string
	FileName        string
	FileSize        int64
	TotalNodes      int
	SuccessCount    int
	FailureCount    int
	OverallProgress float64
	Status          string
	UseDiffusion    bool
	CreatedAt       time.Time
	CompletedAt     *time.Time
	NodeReports     []NodeReport
}

type NodeReport struct {
	NodeID     string
	ParentID   string
	IsSource   bool
	Status     string
	Progress   float64
	Error      string
	StartTime  time.Time
	EndTime    *time.Time
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
