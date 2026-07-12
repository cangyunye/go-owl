package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v3"
)

type createNodeRequest struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Address   string            `json:"address"`
	Port      int               `json:"port"`
	User      string            `json:"user"`
	Password  string            `json:"password,omitempty"`
	SSHKey    string            `json:"ssh_key,omitempty"`
	Status    string            `json:"status"`
	Groups    []string          `json:"groups"`
	Labels    map[string]string `json:"labels"`
	ProxyJump string            `json:"proxy_jump,omitempty"`
}

type updateNodeRequest struct {
	Name      *string            `json:"name"`
	Address   *string            `json:"address"`
	Port      *int               `json:"port"`
	User      *string            `json:"user"`
	Password  *string            `json:"password,omitempty"`
	SSHKey    *string            `json:"ssh_key,omitempty"`
	Status    *string            `json:"status"`
	Groups    *[]string          `json:"groups"`
	Labels    *map[string]string `json:"labels"`
	ProxyJump *string            `json:"proxy_jump,omitempty"`
}

type NodeResponse struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Address   string            `json:"address"`
	Port      int               `json:"port"`
	User      string            `json:"user"`
	Status    string            `json:"status"`
	Groups    []string          `json:"groups"`
	Labels    map[string]string `json:"labels"`
	ProxyJump string            `json:"proxy_jump,omitempty"`
	CreatedAt string            `json:"created_at,omitempty"`
	UpdatedAt string            `json:"updated_at,omitempty"`
	// Intentionally excluding Password and SSHKey
}

type NodeHandler struct {
	db *sql.DB
}

func NewNodeHandler(db *sql.DB) *NodeHandler {
	return &NodeHandler{db: db}
}

func (h *NodeHandler) List(c *gin.Context) {
	query := `SELECT id, name, address, port, user, status, groups, labels,
		COALESCE(proxy_jump, ''), COALESCE(created_at, ''), COALESCE(updated_at, '')
		FROM nodes WHERE 1=1`
	args := []interface{}{}

	if g := c.Query("group"); g != "" {
		groupNames := strings.Split(g, ",")
		clauses := []string{}
		for _, gn := range groupNames {
			gn = strings.TrimSpace(gn)
			if gn == "" { continue }
			clauses = append(clauses, "(groups LIKE ? OR groups LIKE ? OR groups LIKE ?)")
			args = append(args, `%"`+gn+`"%`, `%`+gn+`%`, `%`+gn+`"%"`)
		}
		if len(clauses) > 0 {
			query += " AND (" + strings.Join(clauses, " OR ") + ")"
		}
	}
	if s := c.Query("status"); s != "" {
		switch s {
		case "offline":
			query += " AND status IN ('offline', 'unknown')"
		case "warn":
			query += " AND status IN ('warn', 'warning')"
		default:
			query += " AND status = ?"
			args = append(args, s)
		}
	}
	if u := c.Query("user"); u != "" {
		query += fmt.Sprintf(" AND user = ?")
		args = append(args, u)
	}
	if l := c.Query("label"); l != "" {
		parts := strings.SplitN(l, ":", 2)
		if len(parts) == 2 {
			query += ` AND labels LIKE ?`
			args = append(args, `%"`+parts[0]+`":"`+parts[1]+`"%`)
		}
	}

	var total int
	countQuery := "SELECT COUNT(*) FROM (" + query + ") AS filtered"
	err := h.db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "query error"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	query += " ORDER BY name LIMIT ? OFFSET ?"
	args = append(args, pageSize, offset)

	rows, err := h.db.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "query error"})
		return
	}
	defer rows.Close()

	nodes := make([]NodeResponse, 0)
	for rows.Next() {
		var n NodeResponse
		var groupsJSON, labelsJSON string
		err := rows.Scan(&n.ID, &n.Name, &n.Address, &n.Port, &n.User, &n.Status,
			&groupsJSON, &labelsJSON, &n.ProxyJump, &n.CreatedAt, &n.UpdatedAt)
		if err != nil {
			continue
		}
		json.Unmarshal([]byte(groupsJSON), &n.Groups)
		json.Unmarshal([]byte(labelsJSON), &n.Labels)
		if n.Groups == nil {
			n.Groups = []string{}
		}
		if n.Labels == nil {
			n.Labels = map[string]string{}
		}
		nodes = append(nodes, n)
	}

	c.JSON(http.StatusOK, gin.H{
		"data": nodes,
		"meta": gin.H{
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

func (h *NodeHandler) Get(c *gin.Context) {
	id := c.Param("id")

	var n NodeResponse
	var groupsJSON, labelsJSON string
	err := h.db.QueryRow(
		`SELECT id, name, address, port, user, status, groups, labels,
		COALESCE(proxy_jump, ''), COALESCE(created_at, ''), COALESCE(updated_at, '')
		FROM nodes WHERE id = ?`, id).
		Scan(&n.ID, &n.Name, &n.Address, &n.Port, &n.User, &n.Status,
			&groupsJSON, &labelsJSON, &n.ProxyJump, &n.CreatedAt, &n.UpdatedAt)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "node not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "query error"})
		return
	}
	json.Unmarshal([]byte(groupsJSON), &n.Groups)
	json.Unmarshal([]byte(labelsJSON), &n.Labels)
	if n.Groups == nil {
		n.Groups = []string{}
	}
	if n.Labels == nil {
		n.Labels = map[string]string{}
	}

	c.JSON(http.StatusOK, n)
}

func (h *NodeHandler) Search(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		c.JSON(http.StatusOK, gin.H{"data": []NodeResponse{}})
		return
	}

	query := `SELECT id, name, address, port, user, status, groups, labels,
		COALESCE(proxy_jump, ''), COALESCE(created_at, ''), COALESCE(updated_at, '')
		FROM nodes WHERE
		LOWER(name) LIKE ? OR LOWER(address) LIKE ? OR LOWER(user) LIKE ?
		OR LOWER(groups) LIKE ? OR LOWER(labels) LIKE ?`
	pattern := "%" + strings.ToLower(q) + "%"
	args := []interface{}{pattern, pattern, pattern, pattern, pattern}

	rows, err := h.db.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "query error"})
		return
	}
	defer rows.Close()

	nodes := make([]NodeResponse, 0)
	for rows.Next() {
		var n NodeResponse
		var groupsJSON, labelsJSON string
		err := rows.Scan(&n.ID, &n.Name, &n.Address, &n.Port, &n.User, &n.Status,
			&groupsJSON, &labelsJSON, &n.ProxyJump, &n.CreatedAt, &n.UpdatedAt)
		if err != nil {
			continue
		}
		json.Unmarshal([]byte(groupsJSON), &n.Groups)
		json.Unmarshal([]byte(labelsJSON), &n.Labels)
		if n.Groups == nil {
			n.Groups = []string{}
		}
		if n.Labels == nil {
			n.Labels = map[string]string{}
		}
		nodes = append(nodes, n)
	}

	c.JSON(http.StatusOK, gin.H{"data": nodes})
}

func (h *NodeHandler) Filters(c *gin.Context) {
	rows, err := h.db.Query(`SELECT groups, labels, user FROM nodes`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "query error"})
		return
	}
	defer rows.Close()

	groupSet := map[string]bool{}
	userSet := map[string]bool{}
	labelKeySet := map[string]bool{}

	for rows.Next() {
		var groupsJSON, labelsJSON, user string
		rows.Scan(&groupsJSON, &labelsJSON, &user)
		userSet[user] = true

		var groups []string
		json.Unmarshal([]byte(groupsJSON), &groups)
		for _, g := range groups {
			groupSet[g] = true
		}

		var labels map[string]string
		json.Unmarshal([]byte(labelsJSON), &labels)
		for k := range labels {
			labelKeySet[k] = true
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"groups": keys(groupSet),
		"users":  keys(userSet),
		"labels": keys(labelKeySet),
	})
}

func (h *NodeHandler) Create(c *gin.Context) {
	var req createNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request body"})
		return
	}
	if req.ID == "" || req.Address == "" || req.User == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "id, address, and user are required"})
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if req.Port == 0 {
		req.Port = 22
	}
	if req.Status == "" {
		req.Status = "unknown"
	}
	groupsJSON, _ := json.Marshal(req.Groups)
	labelsJSON, _ := json.Marshal(req.Labels)
	if req.Groups == nil {
		groupsJSON = []byte("[]")
	}
	if req.Labels == nil {
		labelsJSON = []byte("{}")
	}

	_, err := h.db.Exec(
		`INSERT INTO nodes (id, name, address, port, user, password, ssh_key, status, groups, labels, proxy_jump, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.Name, req.Address, req.Port, req.User,
		req.Password, req.SSHKey, req.Status,
		string(groupsJSON), string(labelsJSON), req.ProxyJump,
		now, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "PRIMARY KEY") {
			c.JSON(http.StatusConflict, gin.H{"code": 409, "message": "node ID already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "create failed"})
		return
	}

	// Fetch and return the created node
	var n NodeResponse
	var gJSON, lJSON string
	err = h.db.QueryRow(
		`SELECT id, name, address, port, user, status, groups, labels,
		COALESCE(proxy_jump, ''), COALESCE(created_at, ''), COALESCE(updated_at, '')
		FROM nodes WHERE id = ?`, req.ID,
	).Scan(&n.ID, &n.Name, &n.Address, &n.Port, &n.User, &n.Status,
		&gJSON, &lJSON, &n.ProxyJump, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "create failed"})
		return
	}
	json.Unmarshal([]byte(gJSON), &n.Groups)
	json.Unmarshal([]byte(lJSON), &n.Labels)
	if n.Groups == nil {
		n.Groups = []string{}
	}
	if n.Labels == nil {
		n.Labels = map[string]string{}
	}

	c.JSON(http.StatusCreated, n)
}

func (h *NodeHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var req updateNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request body"})
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Build dynamic UPDATE
	setClauses := []string{}
	args := []interface{}{}

	if req.Name != nil {
		setClauses = append(setClauses, "name = ?")
		args = append(args, *req.Name)
	}
	if req.Address != nil {
		setClauses = append(setClauses, "address = ?")
		args = append(args, *req.Address)
	}
	if req.Port != nil {
		setClauses = append(setClauses, "port = ?")
		args = append(args, *req.Port)
	}
	if req.User != nil {
		setClauses = append(setClauses, "user = ?")
		args = append(args, *req.User)
	}
	if req.Password != nil {
		setClauses = append(setClauses, "password = ?")
		args = append(args, *req.Password)
	}
	if req.SSHKey != nil {
		setClauses = append(setClauses, "ssh_key = ?")
		args = append(args, *req.SSHKey)
	}
	if req.Status != nil {
		setClauses = append(setClauses, "status = ?")
		args = append(args, *req.Status)
	}
	if req.ProxyJump != nil {
		setClauses = append(setClauses, "proxy_jump = ?")
		args = append(args, *req.ProxyJump)
	}
	if req.Groups != nil {
		gJSON, _ := json.Marshal(*req.Groups)
		setClauses = append(setClauses, "groups = ?")
		args = append(args, string(gJSON))
	}
	if req.Labels != nil {
		lJSON, _ := json.Marshal(*req.Labels)
		setClauses = append(setClauses, "labels = ?")
		args = append(args, string(lJSON))
	}

	if len(setClauses) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "no fields to update"})
		return
	}

	setClauses = append(setClauses, "updated_at = ?")
	args = append(args, now)
	args = append(args, id)

	query := "UPDATE nodes SET " + strings.Join(setClauses, ", ") + " WHERE id = ?"
	res, err := h.db.Exec(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "update failed"})
		return
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "node not found"})
		return
	}

	// Return updated node
	c.Request.URL.Path = "/api/v1/nodes/" + id
	h.Get(c)
}

func (h *NodeHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	res, err := h.db.Exec("DELETE FROM nodes WHERE id = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "delete failed"})
		return
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "node not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func keys(set map[string]bool) []string {
	r := make([]string, 0, len(set))
	for k := range set {
		r = append(r, k)
	}
	return r
}

type batchGroupsRequest struct {
	NodeIDs []string `json:"node_ids"`
	Add     []string `json:"add"`
	Remove  []string `json:"remove"`
}

func (h *NodeHandler) BatchGroups(c *gin.Context) {
	var req batchGroupsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request body"})
		return
	}
	if len(req.NodeIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "node_ids is required"})
		return
	}

	addSet := map[string]bool{}
	for _, g := range req.Add {
		if g != "" {
			addSet[g] = true
		}
	}
	removeSet := map[string]bool{}
	for _, g := range req.Remove {
		if g != "" {
			removeSet[g] = true
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	updated := 0
	errors := []string{}

	for _, nodeID := range req.NodeIDs {
		var groupsJSON string
		err := h.db.QueryRow("SELECT groups FROM nodes WHERE id = ?", nodeID).Scan(&groupsJSON)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: not found", nodeID))
			continue
		}

		var groups []string
		json.Unmarshal([]byte(groupsJSON), &groups)

		groupSet := map[string]bool{}
		for _, g := range groups {
			groupSet[g] = true
		}

		for g := range addSet {
			groupSet[g] = true
		}
		for g := range removeSet {
			delete(groupSet, g)
		}

		newGroups := make([]string, 0, len(groupSet))
		for g := range groupSet {
			newGroups = append(newGroups, g)
		}

		gJSON, _ := json.Marshal(newGroups)
		_, err = h.db.Exec("UPDATE nodes SET groups = ?, updated_at = ? WHERE id = ?", string(gJSON), now, nodeID)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: update failed", nodeID))
			continue
		}
		updated++
	}

	c.JSON(http.StatusOK, gin.H{
		"updated": updated,
		"errors":  errors,
	})
}

type exportRequest struct {
	NodeIDs []string `json:"node_ids"`
	Groups  []string `json:"groups"`
	Format  string   `json:"format"`
}

type nodeExportFile struct {
	Version string         `json:"version" yaml:"version"`
	Nodes   []nodeExport   `json:"nodes" yaml:"nodes"`
}

type nodeExport struct {
	ID        string            `json:"id" yaml:"id"`
	Name      string            `json:"name" yaml:"name"`
	Address   string            `json:"address" yaml:"address"`
	Port      int               `json:"port" yaml:"port"`
	User      string            `json:"user" yaml:"user"`
	Status    string            `json:"status" yaml:"status"`
	Groups    []string          `json:"groups" yaml:"groups"`
	Labels    map[string]string `json:"labels" yaml:"labels"`
	ProxyJump string            `json:"proxy_jump,omitempty" yaml:"proxy_jump,omitempty"`
}

func (h *NodeHandler) Export(c *gin.Context) {
	var req exportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request body"})
		return
	}

	query := `SELECT id, name, address, port, user, status, groups, labels,
		COALESCE(proxy_jump, '') FROM nodes WHERE 1=1`
	args := []interface{}{}

	if len(req.NodeIDs) > 0 {
		placeholders := make([]string, len(req.NodeIDs))
		for i, id := range req.NodeIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		query += " AND id IN (" + strings.Join(placeholders, ",") + ")"
	}

	if len(req.Groups) > 0 {
		groupClauses := []string{}
		for _, g := range req.Groups {
			groupClauses = append(groupClauses, "(groups LIKE ? OR groups LIKE ? OR groups LIKE ?)")
			args = append(args, `%"`+g+`"%`, `%`+g+`%`, `%`+g+`"%"`)
		}
		query += " AND (" + strings.Join(groupClauses, " OR ") + ")"
	}

	query += " ORDER BY name"

	rows, err := h.db.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "query error"})
		return
	}
	defer rows.Close()

	nodes := []nodeExport{}
	for rows.Next() {
		var n nodeExport
		var groupsJSON, labelsJSON string
		err := rows.Scan(&n.ID, &n.Name, &n.Address, &n.Port, &n.User, &n.Status,
			&groupsJSON, &labelsJSON, &n.ProxyJump)
		if err != nil {
			continue
		}
		json.Unmarshal([]byte(groupsJSON), &n.Groups)
		json.Unmarshal([]byte(labelsJSON), &n.Labels)
		if n.Groups == nil {
			n.Groups = []string{}
		}
		if n.Labels == nil {
			n.Labels = map[string]string{}
		}
		nodes = append(nodes, n)
	}

	nf := nodeExportFile{Version: "1.0", Nodes: nodes}

	format := req.Format
	if format == "" {
		format = "yaml"
	}

	var data []byte
	var contentType, ext string
	if format == "json" {
		data, _ = json.MarshalIndent(nf, "", "  ")
		contentType = "application/json"
		ext = "json"
	} else {
		data, _ = yaml.Marshal(nf)
		contentType = "application/x-yaml"
		ext = "yaml"
	}

	filename := fmt.Sprintf("nodes-%s.%s", time.Now().Format("20060102-150405"), ext)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Data(http.StatusOK, contentType, data)
}

type importResult struct {
	Success int      `json:"success"`
	Skipped int      `json:"skipped"`
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors,omitempty"`
}

func (h *NodeHandler) Import(c *gin.Context) {
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "file is required"})
		return
	}
	defer file.Close()

	overwrite := c.PostForm("overwrite") == "true"
	skipExisting := c.PostForm("skip_existing") == "true"
	dryRun := c.PostForm("dry_run") == "true"

	data, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "failed to read file"})
		return
	}

	var nf nodeExportFile
	if err := yaml.Unmarshal(data, &nf); err != nil {
		if err2 := json.Unmarshal(data, &nf); err2 != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "failed to parse file as YAML or JSON"})
			return
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	result := importResult{}

	for _, node := range nf.Nodes {
		if node.ID == "" {
			result.Errors = append(result.Errors, "skipped: empty ID")
			result.Failed++
			continue
		}
		if node.Address == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: empty address", node.ID))
			result.Failed++
			continue
		}

		var exists int
		h.db.QueryRow("SELECT COUNT(*) FROM nodes WHERE id = ?", node.ID).Scan(&exists)
		nodeExists := exists > 0

		if nodeExists && skipExisting {
			result.Skipped++
			continue
		}
		if nodeExists && !overwrite {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: already exists", node.ID))
			result.Skipped++
			continue
		}

		if dryRun {
			if nodeExists {
				result.Errors = append(result.Errors, fmt.Sprintf("[preview] update %s", node.ID))
			} else {
				result.Errors = append(result.Errors, fmt.Sprintf("[preview] add %s", node.ID))
			}
			result.Success++
			continue
		}

		if node.Port == 0 {
			node.Port = 22
		}
		if node.Status == "" {
			node.Status = "unknown"
		}
		if node.Groups == nil {
			node.Groups = []string{}
		}
		if node.Labels == nil {
			node.Labels = map[string]string{}
		}
		groupsJSON, _ := json.Marshal(node.Groups)
		labelsJSON, _ := json.Marshal(node.Labels)

		if nodeExists {
			_, err = h.db.Exec(
				`UPDATE nodes SET name=?, address=?, port=?, user=?, status=?, groups=?, labels=?, proxy_jump=?, updated_at=? WHERE id=?`,
				node.Name, node.Address, node.Port, node.User, node.Status,
				string(groupsJSON), string(labelsJSON), node.ProxyJump, now, node.ID)
		} else {
			_, err = h.db.Exec(
				`INSERT INTO nodes (id, name, address, port, user, status, groups, labels, proxy_jump, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
				node.ID, node.Name, node.Address, node.Port, node.User, node.Status,
				string(groupsJSON), string(labelsJSON), node.ProxyJump, now, now)
		}
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", node.ID, err))
			result.Failed++
		} else {
			result.Success++
		}
	}

	c.JSON(http.StatusOK, result)
}

type connectivityRequest struct {
	NodeIDs []string `json:"node_ids"`
}

type pingResult struct {
	NodeID    string  `json:"node_id"`
	Success   bool    `json:"success"`
	LatencyMs float64 `json:"latency_ms"`
	Error     string  `json:"error,omitempty"`
}

func (h *NodeHandler) Ping(c *gin.Context) {
	var req connectivityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request body"})
		return
	}
	if len(req.NodeIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "node_ids is required"})
		return
	}

	type nodeAddr struct {
		ID      string
		Address string
		Port    int
	}

	var nodes []nodeAddr
	for _, id := range req.NodeIDs {
		var n nodeAddr
		err := h.db.QueryRow("SELECT id, address, port FROM nodes WHERE id = ?", id).Scan(&n.ID, &n.Address, &n.Port)
		if err != nil {
			continue
		}
		nodes = append(nodes, n)
	}

	results := make([]pingResult, len(nodes))
	var wg sync.WaitGroup
	timeout := 5 * time.Second

	for i, n := range nodes {
		wg.Add(1)
		go func(idx int, n nodeAddr) {
			defer wg.Done()
			addr := fmt.Sprintf("%s:%d", n.Address, n.Port)
			start := time.Now()
			conn, err := net.DialTimeout("tcp", addr, timeout)
			latency := time.Since(start)
			if err == nil {
				conn.Close()
				results[idx] = pingResult{NodeID: n.ID, Success: true, LatencyMs: float64(latency.Milliseconds())}
			} else {
				results[idx] = pingResult{NodeID: n.ID, Success: false, Error: err.Error()}
			}
		}(i, n)
	}
	wg.Wait()

	c.JSON(http.StatusOK, gin.H{"results": results})
}

type checkResult struct {
	NodeID string `json:"node_id"`
	Success bool   `json:"success"`
	Method  string `json:"method,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (h *NodeHandler) Check(c *gin.Context) {
	var req connectivityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request body"})
		return
	}
	if len(req.NodeIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "node_ids is required"})
		return
	}

	type nodeSSH struct {
		ID       string
		Address  string
		Port     int
		User     string
		Password string
		SSHKey   string
	}

	var nodes []nodeSSH
	for _, id := range req.NodeIDs {
		var n nodeSSH
		var pw, key sql.NullString
		err := h.db.QueryRow("SELECT id, address, port, user, password, ssh_key FROM nodes WHERE id = ?", id).
			Scan(&n.ID, &n.Address, &n.Port, &n.User, &pw, &key)
		if err != nil {
			continue
		}
		if pw.Valid {
			n.Password = pw.String
		}
		if key.Valid {
			n.SSHKey = key.String
		}
		nodes = append(nodes, n)
	}

	results := make([]checkResult, len(nodes))
	var wg sync.WaitGroup
	timeout := 10 * time.Second
	now := time.Now().UTC().Format(time.RFC3339)

	for i, n := range nodes {
		wg.Add(1)
		go func(idx int, n nodeSSH) {
			defer wg.Done()
			r := checkNodeSSH(h.db, n.ID, n.Address, n.Port, n.User, n.Password, n.SSHKey, timeout)
			results[idx] = r

			status := "offline"
			if r.Success {
				status = "online"
			}
			h.db.Exec("UPDATE nodes SET status = ?, updated_at = ? WHERE id = ?", status, now, n.ID)
		}(i, n)
	}
	wg.Wait()

	c.JSON(http.StatusOK, gin.H{"results": results})
}

func checkNodeSSH(db *sql.DB, nodeID, address string, port int, user, password, sshKey string, timeout time.Duration) checkResult {
	addr := fmt.Sprintf("%s:%d", address, port)
	if user == "" {
		user = "root"
	}

	if sshKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(sshKey))
		if err == nil {
			config := &ssh.ClientConfig{
				User:            user,
				Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
				HostKeyCallback: ssh.InsecureIgnoreHostKey(),
				Timeout:         timeout,
			}
			client, err := ssh.Dial("tcp", addr, config)
			if err == nil {
				client.Close()
				return checkResult{NodeID: nodeID, Success: true, Method: "key"}
			}
			if password == "" {
				return checkResult{NodeID: nodeID, Success: false, Error: fmt.Sprintf("key auth failed: %v", err)}
			}
		}
	}

	if password != "" {
		config := &ssh.ClientConfig{
			User:            user,
			Auth:            []ssh.AuthMethod{ssh.Password(password)},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         timeout,
		}
		client, err := ssh.Dial("tcp", addr, config)
		if err == nil {
			client.Close()
			return checkResult{NodeID: nodeID, Success: true, Method: "password"}
		}
		return checkResult{NodeID: nodeID, Success: false, Error: fmt.Sprintf("password auth failed: %v", err)}
	}

	return checkResult{NodeID: nodeID, Success: false, Error: "no credentials configured"}
}
