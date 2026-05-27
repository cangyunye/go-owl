package common

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func makeNode(id, name, address string, port int, user string, groups []string, labels map[string]string) *NodeInfo {
	if groups == nil {
		groups = []string{}
	}
	if labels == nil {
		labels = map[string]string{}
	}
	return &NodeInfo{
		ID:      id,
		Name:    name,
		Address: address,
		Port:    port,
		User:    user,
		Status:  "online",
		Groups:  groups,
		Labels:  labels,
	}
}

func TestDetectConflicts_NoConflicts(t *testing.T) {
	dbNodes := []*NodeInfo{
		makeNode("id1", "node-a", "10.0.0.1", 22, "root", nil, nil),
		makeNode("id2", "node-b", "10.0.0.2", 22, "root", nil, nil),
	}
	jsonNodes := []*NodeInfo{
		makeNode("id1", "node-a", "10.0.0.1", 22, "root", nil, nil),
		makeNode("id2", "node-b", "10.0.0.2", 22, "root", nil, nil),
	}

	conflicts := DetectConflicts(dbNodes, jsonNodes)
	if len(conflicts) != 0 {
		t.Errorf("expected 0 conflicts, got %d: %v", len(conflicts), conflicts)
	}
}

func TestDetectConflicts_DuplicateNameInDB(t *testing.T) {
	dbNodes := []*NodeInfo{
		makeNode("id1", "duplicate", "10.0.0.1", 22, "root", nil, nil),
		makeNode("id2", "duplicate", "10.0.0.2", 22, "root", nil, nil),
	}

	conflicts := DetectConflicts(dbNodes, nil)
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
	if conflicts[0].Type != ConflictDuplicateNameInDB {
		t.Errorf("expected type %s, got %s", ConflictDuplicateNameInDB, conflicts[0].Type)
	}
}

func TestDetectConflicts_DuplicateNameInJSON(t *testing.T) {
	jsonNodes := []*NodeInfo{
		makeNode("id1", "duplicate", "10.0.0.1", 22, "root", nil, nil),
		makeNode("id2", "duplicate", "10.0.0.2", 22, "root", nil, nil),
	}

	conflicts := DetectConflicts(nil, jsonNodes)
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
	if conflicts[0].Type != ConflictDuplicateNameInJSON {
		t.Errorf("expected type %s, got %s", ConflictDuplicateNameInJSON, conflicts[0].Type)
	}
}

func TestDetectConflicts_CrossSourceSameNameDiffID(t *testing.T) {
	dbNodes := []*NodeInfo{
		makeNode("db-id", "web-server", "10.0.0.1", 22, "root", nil, nil),
	}
	jsonNodes := []*NodeInfo{
		makeNode("json-id", "web-server", "10.0.0.2", 22, "root", nil, nil),
	}

	conflicts := DetectConflicts(dbNodes, jsonNodes)
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
	if conflicts[0].Type != ConflictCrossSourceName {
		t.Errorf("expected type %s, got %s", ConflictCrossSourceName, conflicts[0].Type)
	}
	if conflicts[0].DBNode == nil || conflicts[0].DBNode.ID != "db-id" {
		t.Error("expected DB node with id 'db-id' in conflict")
	}
	if conflicts[0].JSONNode == nil || conflicts[0].JSONNode.ID != "json-id" {
		t.Error("expected JSON node with id 'json-id' in conflict")
	}
}

func TestDetectConflicts_CrossSourceSameIDDiffFields(t *testing.T) {
	dbNodes := []*NodeInfo{
		makeNode("srv1", "web", "10.0.0.1", 22, "root", nil, nil),
	}
	jsonNodes := []*NodeInfo{
		makeNode("srv1", "web", "10.0.0.1", 2222, "admin", nil, nil),
	}

	conflicts := DetectConflicts(dbNodes, jsonNodes)
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
	if conflicts[0].Type != ConflictCrossSourceIDFields {
		t.Errorf("expected type %s, got %s", ConflictCrossSourceIDFields, conflicts[0].Type)
	}
}

func TestDetectConflicts_CrossSourceSameIDDiffFields_Groups(t *testing.T) {
	dbNodes := []*NodeInfo{
		makeNode("srv1", "web", "10.0.0.1", 22, "root", []string{"web"}, map[string]string{"env": "prod"}),
	}
	jsonNodes := []*NodeInfo{
		makeNode("srv1", "web", "10.0.0.1", 22, "root", []string{"db"}, map[string]string{"env": "staging"}),
	}

	conflicts := DetectConflicts(dbNodes, jsonNodes)
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
	if conflicts[0].Type != ConflictCrossSourceIDFields {
		t.Errorf("expected type %s, got %s", ConflictCrossSourceIDFields, conflicts[0].Type)
	}
}

func TestDetectConflicts_MultipleConflictTypes(t *testing.T) {
	dbNodes := []*NodeInfo{
		makeNode("db-1", "dup-name", "10.0.0.1", 22, "root", nil, nil),
		makeNode("db-2", "dup-name", "10.0.0.2", 22, "root", nil, nil),
		makeNode("shared-id", "original", "10.0.0.3", 22, "user1", nil, nil),
	}
	jsonNodes := []*NodeInfo{
		makeNode("json-1", "dup-name", "10.0.0.4", 22, "root", nil, nil),
		makeNode("shared-id", "original", "10.0.0.3", 2222, "user2", nil, nil),
	}

	conflicts := DetectConflicts(dbNodes, jsonNodes)

	dbNameCount := 0
	crossNameCount := 0
	crossFieldCount := 0
	for _, c := range conflicts {
		switch c.Type {
		case ConflictDuplicateNameInDB:
			dbNameCount++
		case ConflictCrossSourceName:
			crossNameCount++
		case ConflictCrossSourceIDFields:
			crossFieldCount++
		}
	}

	if dbNameCount != 1 {
		t.Errorf("expected 1 duplicate name in DB conflict, got %d", dbNameCount)
	}
	if crossNameCount < 2 {
		t.Errorf("expected at least 2 cross-source name conflicts, got %d", crossNameCount)
	}
	if crossFieldCount != 1 {
		t.Errorf("expected 1 cross-source ID field conflict, got %d", crossFieldCount)
	}
}

func TestDetectConflicts_EmptyDB(t *testing.T) {
	jsonNodes := []*NodeInfo{
		makeNode("id1", "node-a", "10.0.0.1", 22, "root", nil, nil),
	}

	conflicts := DetectConflicts(nil, jsonNodes)
	if len(conflicts) != 0 {
		t.Errorf("expected 0 conflicts when DB is empty, got %d", len(conflicts))
	}
}

func TestDetectConflicts_EmptyJSON(t *testing.T) {
	dbNodes := []*NodeInfo{
		makeNode("id1", "node-a", "10.0.0.1", 22, "root", nil, nil),
	}

	conflicts := DetectConflicts(dbNodes, nil)
	if len(conflicts) != 0 {
		t.Errorf("expected 0 conflicts when JSON is empty, got %d", len(conflicts))
	}
}

func TestDetectConflicts_BothEmpty(t *testing.T) {
	conflicts := DetectConflicts(nil, nil)
	if len(conflicts) != 0 {
		t.Errorf("expected 0 conflicts when both empty, got %d", len(conflicts))
	}
}

func TestCompareNodeFields(t *testing.T) {
	dbNode := makeNode("id1", "web", "10.0.0.1", 22, "root", []string{"web"}, map[string]string{"env": "prod"})
	jsonNode := makeNode("id1", "web", "10.0.0.1", 22, "root", []string{"web"}, map[string]string{"env": "prod"})

	diffs := compareNodeFields(dbNode, jsonNode)
	if len(diffs) != 0 {
		t.Errorf("expected 0 diffs, got %d: %v", len(diffs), diffs)
	}

	jsonNode.Port = 2222
	jsonNode.User = "admin"
	diffs = compareNodeFields(dbNode, jsonNode)
	if len(diffs) != 2 {
		t.Errorf("expected 2 diffs, got %d: %v", len(diffs), diffs)
	}
}

func TestBulkUpsert_InsertNewNodes(t *testing.T) {
	store := setupTestDB(t)

	nodes := []*NodeInfo{
		makeNode("n1", "node-1", "10.0.0.1", 22, "root", []string{"web"}, map[string]string{"env": "prod"}),
		makeNode("n2", "node-2", "10.0.0.2", 22, "admin", nil, nil),
	}

	err := store.BulkUpsert(nodes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	list, err := store.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(list))
	}

	idMap := make(map[string]*NodeInfo)
	for _, n := range list {
		idMap[n.ID] = n
	}

	n1, ok := idMap["n1"]
	if !ok {
		t.Fatal("expected node n1 in list")
	}
	if n1.Name != "node-1" {
		t.Errorf("expected Name 'node-1', got %q", n1.Name)
	}
	if len(n1.Groups) != 1 || n1.Groups[0] != "web" {
		t.Errorf("expected Groups ['web'], got %v", n1.Groups)
	}
	if n1.Labels["env"] != "prod" {
		t.Errorf("expected Labels[env]='prod', got %q", n1.Labels["env"])
	}

	n2, ok := idMap["n2"]
	if !ok {
		t.Fatal("expected node n2 in list")
	}
	if n2.Name != "node-2" {
		t.Errorf("expected Name 'node-2', got %q", n2.Name)
	}
}

func TestBulkUpsert_ReplaceExistingNode(t *testing.T) {
	store := setupTestDB(t)

	original := makeNode("n1", "original", "10.0.0.1", 22, "root", nil, nil)
	err := store.Add(original)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := makeNode("n1", "updated", "10.0.0.2", 2222, "admin", []string{"web"}, map[string]string{"env": "prod"})
	err = store.BulkUpsert([]*NodeInfo{updated})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stored, err := store.Get("n1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stored.Name != "updated" {
		t.Errorf("expected Name 'updated', got %q", stored.Name)
	}
	if stored.Address != "10.0.0.2" {
		t.Errorf("expected Address '10.0.0.2', got %q", stored.Address)
	}
	if stored.Port != 2222 {
		t.Errorf("expected Port 2222, got %d", stored.Port)
	}
	if stored.User != "admin" {
		t.Errorf("expected User 'admin', got %q", stored.User)
	}
}

func TestBulkUpsert_GroupsLabelsJSON(t *testing.T) {
	store := setupTestDB(t)

	nodes := []*NodeInfo{
		{
			ID:      "n1",
			Name:    "test",
			Address: "10.0.0.1",
			Port:    22,
			User:    "root",
			Status:  "online",
			Groups:  []string{"production", "web"},
			Labels:  map[string]string{"region": "cn-east", "tier": "frontend"},
		},
	}

	err := store.BulkUpsert(nodes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stored, err := store.Get("n1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(stored.Groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(stored.Groups))
	}
	if stored.Groups[0] != "production" || stored.Groups[1] != "web" {
		t.Errorf("unexpected groups: %v", stored.Groups)
	}
	if stored.Labels["region"] != "cn-east" {
		t.Errorf("expected Labels[region]='cn-east', got %q", stored.Labels["region"])
	}
	if stored.Labels["tier"] != "frontend" {
		t.Errorf("expected Labels[tier]='frontend', got %q", stored.Labels["tier"])
	}
}

func TestBulkUpsert_EmptySlice(t *testing.T) {
	store := setupTestDB(t)

	err := store.BulkUpsert([]*NodeInfo{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	list, err := store.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(list))
	}
}

func TestSyncNodesJSONToDB_Success(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "nodes.json")

	nodes := []*NodeInfo{
		makeNode("n1", "test-node", "10.0.0.1", 22, "root", []string{"web"}, map[string]string{"env": "prod"}),
	}
	data, err := json.MarshalIndent(nodes, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal nodes: %v", err)
	}
	if err := os.WriteFile(jsonPath, data, 0644); err != nil {
		t.Fatalf("failed to write nodes.json: %v", err)
	}

	store := setupTestDB(t)
	err = syncNodesJSONToDBAt(store.db, jsonPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	list, err := store.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 node, got %d", len(list))
	}
	if list[0].ID != "n1" {
		t.Errorf("expected ID 'n1', got %q", list[0].ID)
	}
}

func TestSyncNodesJSONToDB_Overwrite(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "nodes.json")

	nodes := []*NodeInfo{
		makeNode("n1", "updated-name", "10.0.0.2", 2222, "admin", nil, nil),
	}
	data, err := json.MarshalIndent(nodes, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal nodes: %v", err)
	}
	if err := os.WriteFile(jsonPath, data, 0644); err != nil {
		t.Fatalf("failed to write nodes.json: %v", err)
	}

	store := setupTestDB(t)
	original := makeNode("n1", "original", "10.0.0.1", 22, "root", nil, nil)
	if err := store.Add(original); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = syncNodesJSONToDBAt(store.db, jsonPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stored, err := store.Get("n1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stored.Name != "updated-name" {
		t.Errorf("expected Name 'updated-name', got %q", stored.Name)
	}
}

func TestReadNodesFromJSON_FileNotExist(t *testing.T) {
	nodes, err := ReadNodesFromJSON("/nonexistent/path/nodes.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nodes != nil {
		t.Errorf("expected nil nodes for nonexistent file, got %v", nodes)
	}
}

func TestReadNodesFromJSON_ValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "nodes.json")

	expected := []*NodeInfo{
		makeNode("n1", "node-1", "10.0.0.1", 22, "root", nil, nil),
		makeNode("n2", "node-2", "10.0.0.2", 22, "admin", nil, nil),
	}
	data, err := json.MarshalIndent(expected, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal nodes: %v", err)
	}
	if err := os.WriteFile(jsonPath, data, 0644); err != nil {
		t.Fatalf("failed to write nodes.json: %v", err)
	}

	nodes, err := ReadNodesFromJSON(jsonPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
	if nodes[0].ID != "n1" {
		t.Errorf("expected ID 'n1', got %q", nodes[0].ID)
	}
	if nodes[1].ID != "n2" {
		t.Errorf("expected ID 'n2', got %q", nodes[1].ID)
	}
}

func TestNodeConflict_Struct(t *testing.T) {
	c := NodeConflict{
		Type:        ConflictCrossSourceName,
		Description: "test conflict",
		DBNode:      makeNode("db-1", "web", "10.0.0.1", 22, "root", nil, nil),
		JSONNode:    makeNode("json-1", "web", "10.0.0.2", 22, "root", nil, nil),
	}

	if c.Type != ConflictCrossSourceName {
		t.Errorf("expected type %s, got %s", ConflictCrossSourceName, c.Type)
	}
	if c.Description != "test conflict" {
		t.Errorf("expected description 'test conflict', got %q", c.Description)
	}
	if c.DBNode == nil || c.DBNode.ID != "db-1" {
		t.Error("expected DBNode with id 'db-1'")
	}
	if c.JSONNode == nil || c.JSONNode.ID != "json-1" {
		t.Error("expected JSONNode with id 'json-1'")
	}
}

func TestCollectConflictNodeIDs(t *testing.T) {
	conflicts := []NodeConflict{
		{DBNode: &NodeInfo{ID: "n1"}, JSONNode: &NodeInfo{ID: "n1"}},
		{DBNode: &NodeInfo{ID: "n2"}, JSONNode: nil},
		{DBNode: nil, JSONNode: &NodeInfo{ID: "n3"}},
		{DBNode: &NodeInfo{ID: "n1"}, JSONNode: &NodeInfo{ID: "n4"}},
	}

	ids := collectConflictNodeIDs(conflicts)
	if len(ids) != 4 {
		t.Fatalf("expected 4 unique node IDs, got %d: %v", len(ids), ids)
	}

	idSet := make(map[string]bool)
	for _, id := range ids {
		idSet[id] = true
	}
	for _, want := range []string{"n1", "n2", "n3", "n4"} {
		if !idSet[want] {
			t.Errorf("expected ID %q in result", want)
		}
	}
}

func TestSyncNodesFromDBToJSON_UpdateExisting(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "nodes.json")

	original := []*NodeInfo{
		makeNode("n1", "original", "10.0.0.1", 22, "root", nil, nil),
	}
	writeNodesJSON(jsonPath, original)

	originalNodeJSONPath := NodeJSONPath
	defer func() { NodeJSONPath = originalNodeJSONPath }()
	NodeJSONPath = func() string { return jsonPath }

	store := setupTestDB(t)
	updated := makeNode("n1", "updated-from-db", "10.0.0.2", 2222, "admin", []string{"db"}, map[string]string{"source": "db"})
	store.Add(updated)

	err := syncNodesFromDBToJSON(store.db, []string{"n1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	nodes, err := ReadNodesFromJSON(jsonPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].Name != "updated-from-db" {
		t.Errorf("expected Name 'updated-from-db', got %q", nodes[0].Name)
	}
	if nodes[0].Port != 2222 {
		t.Errorf("expected Port 2222, got %d", nodes[0].Port)
	}
	if nodes[0].User != "admin" {
		t.Errorf("expected User 'admin', got %q", nodes[0].User)
	}
}

func TestSyncNodesFromDBToJSON_AddNew(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "nodes.json")

	original := []*NodeInfo{
		makeNode("n1", "node-1", "10.0.0.1", 22, "root", nil, nil),
	}
	writeNodesJSON(jsonPath, original)

	originalNodeJSONPath := NodeJSONPath
	defer func() { NodeJSONPath = originalNodeJSONPath }()
	NodeJSONPath = func() string { return jsonPath }

	store := setupTestDB(t)
	newNode := makeNode("n2", "new-from-db", "10.0.0.2", 22, "admin", nil, nil)
	store.Add(newNode)

	err := syncNodesFromDBToJSON(store.db, []string{"n2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	nodes, err := ReadNodesFromJSON(jsonPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
}

func TestWriteNodesJSON(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "nodes.json")

	nodes := []*NodeInfo{
		makeNode("n1", "test", "10.0.0.1", 22, "root", []string{"web"}, map[string]string{"env": "prod"}),
	}
	err := writeNodesJSON(jsonPath, nodes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	readNodes, err := ReadNodesFromJSON(jsonPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(readNodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(readNodes))
	}
	if readNodes[0].ID != "n1" {
		t.Errorf("expected ID 'n1', got %q", readNodes[0].ID)
	}
	if len(readNodes[0].Groups) != 1 || readNodes[0].Groups[0] != "web" {
		t.Errorf("unexpected groups: %v", readNodes[0].Groups)
	}
}

func TestPrintNodeConflictInfo(t *testing.T) {
	dbByID := map[string]*NodeInfo{
		"srv1": makeNode("srv1", "web", "10.0.0.1", 22, "root", nil, nil),
	}
	jsonByID := map[string]*NodeInfo{
		"srv1": makeNode("srv1", "web", "10.0.0.1", 2222, "admin", nil, nil),
	}
	conflicts := DetectConflicts([]*NodeInfo{dbByID["srv1"]}, []*NodeInfo{jsonByID["srv1"]})

	printNodeConflictInfo("srv1", dbByID, jsonByID, conflicts)
}

func TestSyncSingleNodeToDB(t *testing.T) {
	store := setupTestDB(t)

	node := makeNode("n1", "from-json", "10.0.0.1", 22, "root", nil, nil)
	err := syncSingleNodeToDB(store.db, node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stored, err := store.Get("n1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stored.Name != "from-json" {
		t.Errorf("expected Name 'from-json', got %q", stored.Name)
	}
}
