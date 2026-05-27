package common

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	internalhistory "github.com/cangyunye/go-owl/internal/history"
	"github.com/cangyunye/go-owl/internal/logger"
	"golang.org/x/term"
)

type ConflictType string

const (
	ConflictDuplicateNameInDB    ConflictType = "duplicate_name_db"
	ConflictDuplicateNameInJSON  ConflictType = "duplicate_name_json"
	ConflictCrossSourceName      ConflictType = "cross_source_name"
	ConflictCrossSourceIDFields  ConflictType = "cross_source_id_fields"
)

type NodeConflict struct {
	Type        ConflictType
	Description string
	DBNode      *NodeInfo
	JSONNode    *NodeInfo
}

func ReadNodesFromJSON(jsonPath string) ([]*NodeInfo, error) {
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		return nil, nil
	}

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, err
	}

	var nodes []*NodeInfo
	if err := json.Unmarshal(data, &nodes); err != nil {
		return nil, err
	}

	return nodes, nil
}

var NodeJSONPath = defaultNodeJSONPath

func defaultNodeJSONPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("/tmp", ".owl", "nodes.json")
	}
	return filepath.Join(homeDir, ".owl", "nodes.json")
}

func DetectConflicts(dbNodes, jsonNodes []*NodeInfo) []NodeConflict {
	var conflicts []NodeConflict

	dbByName := make(map[string][]*NodeInfo)
	dbByID := make(map[string]*NodeInfo)
	jsonByName := make(map[string][]*NodeInfo)
	jsonByID := make(map[string]*NodeInfo)

	for _, n := range dbNodes {
		dbByID[n.ID] = n
		dbByName[n.Name] = append(dbByName[n.Name], n)
	}
	for _, n := range jsonNodes {
		jsonByID[n.ID] = n
		jsonByName[n.Name] = append(jsonByName[n.Name], n)
	}

	conflicts = append(conflicts, detectSameSourceDuplicates(dbByName, "db")...)
	conflicts = append(conflicts, detectSameSourceDuplicates(jsonByName, "json")...)
	conflicts = append(conflicts, detectCrossSourceNameConflicts(dbByName, jsonByName)...)
	conflicts = append(conflicts, detectCrossSourceIDFieldConflicts(dbByID, jsonByID)...)

	return conflicts
}

func detectSameSourceDuplicates(byName map[string][]*NodeInfo, source string) []NodeConflict {
	var conflicts []NodeConflict
	var cType ConflictType
	if source == "db" {
		cType = ConflictDuplicateNameInDB
	} else {
		cType = ConflictDuplicateNameInJSON
	}

	for name, nodes := range byName {
		if len(nodes) > 1 {
			ids := make([]string, len(nodes))
			for i, n := range nodes {
				ids[i] = n.ID
			}
			conflicts = append(conflicts, NodeConflict{
				Type:        cType,
				Description: fmt.Sprintf("Same name '%s' found in %s for IDs: %s", name, source, strings.Join(ids, ", ")),
				DBNode:      nodes[0],
			})
		}
	}
	return conflicts
}

func detectCrossSourceNameConflicts(dbByName, jsonByName map[string][]*NodeInfo) []NodeConflict {
	var conflicts []NodeConflict

	for name, dbNodes := range dbByName {
		jsonNodes, ok := jsonByName[name]
		if !ok {
			continue
		}
		for _, dbNode := range dbNodes {
			for _, jsonNode := range jsonNodes {
				if dbNode.ID != jsonNode.ID {
					conflicts = append(conflicts, NodeConflict{
						Type: ConflictCrossSourceName,
						Description: fmt.Sprintf("Same name '%s' but different IDs: DB=%s, JSON=%s",
							name, dbNode.ID, jsonNode.ID),
						DBNode:   dbNode,
						JSONNode: jsonNode,
					})
				}
			}
		}
	}

	return conflicts
}

func detectCrossSourceIDFieldConflicts(dbByID, jsonByID map[string]*NodeInfo) []NodeConflict {
	var conflicts []NodeConflict

	for id, dbNode := range dbByID {
		jsonNode, ok := jsonByID[id]
		if !ok {
			continue
		}

		diffs := compareNodeFields(dbNode, jsonNode)
		if len(diffs) > 0 {
			conflicts = append(conflicts, NodeConflict{
				Type: ConflictCrossSourceIDFields,
				Description: fmt.Sprintf("Same ID '%s' has different fields: %s",
					id, strings.Join(diffs, ", ")),
				DBNode:   dbNode,
				JSONNode: jsonNode,
			})
		}
	}

	return conflicts
}

func compareNodeFields(dbNode, jsonNode *NodeInfo) []string {
	var diffs []string

	if dbNode.Name != jsonNode.Name {
		diffs = append(diffs, fmt.Sprintf("name(%s⇔%s)", dbNode.Name, jsonNode.Name))
	}
	if dbNode.Address != jsonNode.Address {
		diffs = append(diffs, fmt.Sprintf("address(%s⇔%s)", dbNode.Address, jsonNode.Address))
	}
	if dbNode.Port != jsonNode.Port {
		diffs = append(diffs, fmt.Sprintf("port(%d⇔%d)", dbNode.Port, jsonNode.Port))
	}
	if dbNode.User != jsonNode.User {
		diffs = append(diffs, fmt.Sprintf("user(%s⇔%s)", dbNode.User, jsonNode.User))
	}
	if dbNode.Password != jsonNode.Password {
		diffs = append(diffs, "password(different)")
	}
	if dbNode.SSHKey != jsonNode.SSHKey {
		diffs = append(diffs, fmt.Sprintf("ssh_key(%s⇔%s)", dbNode.SSHKey, jsonNode.SSHKey))
	}
	if dbNode.Status != jsonNode.Status {
		diffs = append(diffs, fmt.Sprintf("status(%s⇔%s)", dbNode.Status, jsonNode.Status))
	}
	if !reflect.DeepEqual(dbNode.Groups, jsonNode.Groups) {
		diffs = append(diffs, fmt.Sprintf("groups(%v⇔%v)", dbNode.Groups, jsonNode.Groups))
	}
	if !reflect.DeepEqual(dbNode.Labels, jsonNode.Labels) {
		diffs = append(diffs, fmt.Sprintf("labels(%v⇔%v)", dbNode.Labels, jsonNode.Labels))
	}
	if dbNode.ProxyJump != jsonNode.ProxyJump {
		diffs = append(diffs, fmt.Sprintf("proxy_jump(%s⇔%s)", dbNode.ProxyJump, jsonNode.ProxyJump))
	}

	return diffs
}

func SyncNodesJSONToDB(db *sql.DB) error {
	jsonPath := NodeJSONPath()
	return syncNodesJSONToDBAt(db, jsonPath)
}

func syncNodesJSONToDBAt(db *sql.DB, jsonPath string) error {
	nodes, err := ReadNodesFromJSON(jsonPath)
	if err != nil {
		return fmt.Errorf("read nodes.json: %w", err)
	}
	if nodes == nil {
		logger.Info("nodes.json not found, skipping sync")
		return nil
	}

	store := NewNodeStoreDB(db)
	if err := store.BulkUpsert(nodes); err != nil {
		return fmt.Errorf("bulk upsert nodes: %w", err)
	}

	logger.Info("Synced nodes.json to database", logger.WithField("count", len(nodes)))
	return nil
}

func EnsureNodesConsistent(db *sql.DB) error {
	jsonNodes, err := ReadNodesFromJSON(NodeJSONPath())
	if err != nil || jsonNodes == nil {
		return nil
	}

	store := NewNodeStoreDB(db)
	dbNodes, err := store.listInternal()
	if err != nil {
		return nil
	}

	return resolveNodeConflicts(db, dbNodes, jsonNodes)
}

func resolveNodeConflicts(db *sql.DB, dbNodes, jsonNodes []*NodeInfo) error {
	conflicts := DetectConflicts(dbNodes, jsonNodes)
	if len(conflicts) == 0 {
		return nil
	}

	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("检测到节点数据冲突: 共 %d 个冲突", len(conflicts))
	}

	dbByID := make(map[string]*NodeInfo)
	jsonByID := make(map[string]*NodeInfo)
	for _, n := range dbNodes {
		dbByID[n.ID] = n
	}
	for _, n := range jsonNodes {
		jsonByID[n.ID] = n
	}

	conflictNodeIDs := collectConflictNodeIDs(conflicts)

	fmt.Println("⚠️  检测到节点数据冲突！")
	fmt.Printf("\n数据库节点:    %d 个\n", len(dbNodes))
	fmt.Printf("nodes.json:    %d 个\n", len(jsonNodes))
	fmt.Printf("冲突节点数:    %d 个\n\n", len(conflictNodeIDs))

	var syncToDB []string
	var syncToJSON []string

	for i, nodeID := range conflictNodeIDs {
		remaining := len(conflictNodeIDs) - i
		printNodeConflictInfo(nodeID, dbByID, jsonByID, conflicts)
		fmt.Print("\n请选择处理方式:\n")
		fmt.Print("  [1] 用 nodes.json 覆盖数据库\n")
		fmt.Print("  [2] 用数据库覆盖 nodes.json\n")
		fmt.Print("  [3] 跳过此节点\n")
		if remaining > 1 {
			fmt.Print("  [4] 批量处理剩余节点\n")
		}
		fmt.Print("  输入选择 (1/2/3" + func() string {
			if remaining > 1 {
				return "/4"
			}
			return ""
		}() + "): ")
		var choice string
		fmt.Scanln(&choice)

		if remaining > 1 && choice == "4" {
			fmt.Println("\n批量处理剩余节点:")
			fmt.Print("  [1] 剩余的都不变\n")
			fmt.Print("  [2] 剩余的都用 nodes.json 覆盖数据库\n")
			fmt.Print("  [3] 剩余的都用数据库覆盖 nodes.json\n")
			fmt.Print("  输入选择 (1/2/3): ")
			var batchChoice string
			fmt.Scanln(&batchChoice)
			switch batchChoice {
			case "2":
				for _, rid := range conflictNodeIDs[i:] {
					syncToDB = append(syncToDB, rid)
				}
				fmt.Printf("  ✓ 已标记剩余 %d 个节点: 将用 nodes.json 覆盖数据库\n", remaining)
			case "3":
				for _, rid := range conflictNodeIDs[i:] {
					syncToJSON = append(syncToJSON, rid)
				}
				fmt.Printf("  ✓ 已标记剩余 %d 个节点: 将用数据库覆盖 nodes.json\n", remaining)
			default:
				fmt.Printf("  - 剩余 %d 个节点保持不变\n", remaining)
			}
			break
		}

		switch choice {
		case "1":
			syncToDB = append(syncToDB, nodeID)
			fmt.Println("  ✓ 已标记: 将用 nodes.json 覆盖数据库")
		case "2":
			syncToJSON = append(syncToJSON, nodeID)
			fmt.Println("  ✓ 已标记: 将用数据库覆盖 nodes.json")
		default:
			fmt.Println("  - 已跳过")
		}
		fmt.Println()
	}

	if len(syncToDB) > 0 {
		for _, nodeID := range syncToDB {
			if jsonNode, ok := jsonByID[nodeID]; ok {
				if err := syncSingleNodeToDB(db, jsonNode); err != nil {
					logger.Warn("同步节点到数据库失败", logger.WithField("node_id", nodeID), logger.WithError(err))
				}
			}
		}
		logger.Info("已将 nodes.json 中的节点同步到数据库", logger.WithField("count", len(syncToDB)))
	}

	if len(syncToJSON) > 0 {
		if err := syncNodesFromDBToJSON(db, syncToJSON); err != nil {
			logger.Warn("同步节点到 nodes.json 失败", logger.WithError(err))
		}
		logger.Info("已将数据库中的节点同步到 nodes.json", logger.WithField("count", len(syncToJSON)))
	}

	fmt.Println()
	fmt.Println("冲突处理完成，继续执行。")
	return nil
}

func collectConflictNodeIDs(conflicts []NodeConflict) []string {
	seen := make(map[string]bool)
	var ids []string
	for _, c := range conflicts {
		if c.DBNode != nil && !seen[c.DBNode.ID] {
			seen[c.DBNode.ID] = true
			ids = append(ids, c.DBNode.ID)
		}
		if c.JSONNode != nil && !seen[c.JSONNode.ID] {
			seen[c.JSONNode.ID] = true
			ids = append(ids, c.JSONNode.ID)
		}
	}
	return ids
}

func printNodeConflictInfo(nodeID string, dbByID, jsonByID map[string]*NodeInfo, allConflicts []NodeConflict) {
	dbNode := dbByID[nodeID]
	jsonNode := jsonByID[nodeID]

	fmt.Printf("--- 节点 %s ---\n", nodeID)

	if dbNode != nil && jsonNode != nil {
		diffs := compareNodeFields(dbNode, jsonNode)
		if len(diffs) > 0 {
			fmt.Printf("  字段不一致: %s\n", strings.Join(diffs, ", "))
		}
		fmt.Printf("  数据库版本:   name=%s, address=%s, port=%d, user=%s\n",
			dbNode.Name, dbNode.Address, dbNode.Port, dbNode.User)
		fmt.Printf("  JSON 版本:    name=%s, address=%s, port=%d, user=%s\n",
			jsonNode.Name, jsonNode.Address, jsonNode.Port, jsonNode.User)
	} else if dbNode != nil {
		fmt.Printf("  名称: %s, 地址: %s:%d, 用户: %s\n", dbNode.Name, dbNode.Address, dbNode.Port, dbNode.User)
		fmt.Println("  (仅存在于数据库中)")
	} else if jsonNode != nil {
		fmt.Printf("  名称: %s, 地址: %s:%d, 用户: %s\n", jsonNode.Name, jsonNode.Address, jsonNode.Port, jsonNode.User)
		fmt.Println("  (仅存在于 nodes.json 中)")
	}

	fmt.Println("  相关冲突:")
	for _, c := range allConflicts {
		matches := (c.DBNode != nil && c.DBNode.ID == nodeID) || (c.JSONNode != nil && c.JSONNode.ID == nodeID)
		if matches {
			fmt.Printf("    [%s] %s\n", c.Type, c.Description)
		}
	}
}

func syncSingleNodeToDB(db *sql.DB, node *NodeInfo) error {
	store := NewNodeStoreDB(db)
	return store.BulkUpsert([]*NodeInfo{node})
}

func syncNodesFromDBToJSON(db *sql.DB, nodeIDs []string) error {
	store := NewNodeStoreDB(db)
	jsonPath := NodeJSONPath()

	jsonNodes, err := ReadNodesFromJSON(jsonPath)
	if err != nil {
		return err
	}
	if jsonNodes == nil {
		jsonNodes = []*NodeInfo{}
	}

	jsonByID := make(map[string]int)
	for i, n := range jsonNodes {
		jsonByID[n.ID] = i
	}

	for _, nodeID := range nodeIDs {
		dbNode, err := store.Get(nodeID)
		if err != nil {
			continue
		}
		if idx, ok := jsonByID[nodeID]; ok {
			jsonNodes[idx] = dbNode
		} else {
			jsonNodes = append(jsonNodes, dbNode)
		}
	}

	return writeNodesJSON(jsonPath, jsonNodes)
}

func writeNodesJSON(jsonPath string, nodes []*NodeInfo) error {
	data, err := json.MarshalIndent(nodes, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(jsonPath, data, 0644)
}

func PrintConflictReport(conflicts []NodeConflict, dbCount, jsonCount int) {
	conflictNodeIDs := collectConflictNodeIDs(conflicts)

	fmt.Println("⚠️  检测到节点数据冲突！")
	fmt.Printf("\n数据库节点:    %d 个, 涉及 %d 个冲突节点\n", dbCount, len(conflictNodeIDs))
	fmt.Printf("nodes.json:    %d 个\n", jsonCount)
	fmt.Println("\n冲突详情:")
	for _, c := range conflicts {
		fmt.Printf("  [%s] %s\n", c.Type, c.Description)
	}
}

func CheckNodeConflictsBeforeExec() {
	db, err := internalhistory.NewDB(internalhistory.DefaultConfig())
	if err != nil || db == nil {
		return
	}
	sqlDB := db.Connection()
	if sqlDB == nil {
		return
	}
	if err := EnsureNodesConsistent(sqlDB); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		fmt.Fprintf(os.Stderr, "使用 --sync-nodes 自动用 nodes.json 覆盖数据库，或交互式运行以逐个解决冲突。\n")
		os.Exit(1)
	}
}
