package common

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	internalhistory "github.com/cangyunye/go-owl/internal/history"
	"github.com/cangyunye/go-owl/internal/i18n"
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
		return errors.New(i18n.T("common.conflict.err_detected", i18n.F(len(conflicts))))
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

	fmt.Println(i18n.T("common.conflict.banner"))
	fmt.Printf("%s", i18n.T("common.conflict.db_count", i18n.F(len(dbNodes))))
	fmt.Printf("%s", i18n.T("common.conflict.json_count", i18n.F(len(jsonNodes))))
	fmt.Printf("%s", i18n.T("common.conflict.conflict_count", i18n.F(len(conflictNodeIDs))))

	var syncToDB []string
	var syncToJSON []string

	for i, nodeID := range conflictNodeIDs {
		remaining := len(conflictNodeIDs) - i
		printNodeConflictInfo(nodeID, dbByID, jsonByID, conflicts)
		fmt.Print(i18n.T("common.conflict.choose_title"))
		fmt.Print(i18n.T("common.conflict.opt_json_over_db"))
		fmt.Print(i18n.T("common.conflict.opt_db_over_json"))
		fmt.Print(i18n.T("common.conflict.opt_skip"))
		if remaining > 1 {
			fmt.Print(i18n.T("common.conflict.opt_batch"))
		}
		opts := "1/2/3"
		if remaining > 1 {
			opts = "1/2/3/4"
		}
		fmt.Print(i18n.T("common.conflict.choice_prompt", opts))
		var choice string
		fmt.Scanln(&choice)

		if remaining > 1 && choice == "4" {
			fmt.Println(i18n.T("common.conflict.batch_title"))
			fmt.Print(i18n.T("common.conflict.batch_opt_keep"))
			fmt.Print(i18n.T("common.conflict.batch_opt_json"))
			fmt.Print(i18n.T("common.conflict.batch_opt_db"))
			fmt.Print(i18n.T("common.conflict.batch_prompt"))
			var batchChoice string
			fmt.Scanln(&batchChoice)
			switch batchChoice {
			case "2":
				for _, rid := range conflictNodeIDs[i:] {
					syncToDB = append(syncToDB, rid)
				}
				fmt.Printf("%s", i18n.T("common.conflict.batch_marked_json", i18n.F(remaining)))
			case "3":
				for _, rid := range conflictNodeIDs[i:] {
					syncToJSON = append(syncToJSON, rid)
				}
				fmt.Printf("%s", i18n.T("common.conflict.batch_marked_db", i18n.F(remaining)))
			default:
				fmt.Printf("%s", i18n.T("common.conflict.batch_keep", i18n.F(remaining)))
			}
			break
		}

		switch choice {
		case "1":
			syncToDB = append(syncToDB, nodeID)
			fmt.Println(i18n.T("common.conflict.marked_json"))
		case "2":
			syncToJSON = append(syncToJSON, nodeID)
			fmt.Println(i18n.T("common.conflict.marked_db"))
		default:
			fmt.Println(i18n.T("common.conflict.skipped"))
		}
		fmt.Println()
	}

	if len(syncToDB) > 0 {
		for _, nodeID := range syncToDB {
			if jsonNode, ok := jsonByID[nodeID]; ok {
				if err := syncSingleNodeToDB(db, jsonNode); err != nil {
					logger.Warn("failed to sync node to database", logger.WithField("node_id", nodeID), logger.WithError(err))
				}
			}
		}
		logger.Info("Synced nodes from nodes.json to database", logger.WithField("count", len(syncToDB)))
	}

	if len(syncToJSON) > 0 {
		if err := syncNodesFromDBToJSON(db, syncToJSON); err != nil {
			logger.Warn("failed to sync nodes to nodes.json", logger.WithError(err))
		}
		logger.Info("Synced nodes from database to nodes.json", logger.WithField("count", len(syncToJSON)))
	}

	fmt.Println()
	fmt.Println(i18n.T("common.conflict.done"))
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

	fmt.Printf("%s", i18n.T("common.conflict.node_header", nodeID))

	if dbNode != nil && jsonNode != nil {
		diffs := compareNodeFields(dbNode, jsonNode)
		if len(diffs) > 0 {
			fmt.Printf("%s", i18n.T("common.conflict.fields_differ", strings.Join(diffs, ", ")))
		}
		fmt.Printf("%s", i18n.T("common.conflict.db_version",
			dbNode.Name, dbNode.Address, i18n.F(dbNode.Port), dbNode.User))
		fmt.Printf("%s", i18n.T("common.conflict.json_version",
			jsonNode.Name, jsonNode.Address, i18n.F(jsonNode.Port), jsonNode.User))
	} else if dbNode != nil {
		fmt.Printf("%s", i18n.T("common.conflict.node_brief", dbNode.Name, dbNode.Address, i18n.F(dbNode.Port), dbNode.User))
		fmt.Println(i18n.T("common.conflict.only_db"))
	} else if jsonNode != nil {
		fmt.Printf("%s", i18n.T("common.conflict.node_brief", jsonNode.Name, jsonNode.Address, i18n.F(jsonNode.Port), jsonNode.User))
		fmt.Println(i18n.T("common.conflict.only_json"))
	}

	fmt.Println(i18n.T("common.conflict.related"))
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

	fmt.Println(i18n.T("common.conflict.banner"))
	fmt.Printf("%s", i18n.T("common.conflict.report_count", i18n.F(dbCount), i18n.F(len(conflictNodeIDs))))
	fmt.Printf("%s", i18n.T("common.conflict.json_count", i18n.F(jsonCount)))
	fmt.Println(i18n.T("common.conflict.report_details"))
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
		fmt.Fprintln(os.Stderr, i18n.T("common.conflict.err_generic", err))
		fmt.Fprintln(os.Stderr, i18n.T("common.conflict.err_hint"))
		os.Exit(1)
	}
}
