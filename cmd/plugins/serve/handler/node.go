package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
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
	argIdx := 1

	if g := c.Query("group"); g != "" {
		query += fmt.Sprintf(" AND (groups LIKE ? OR groups LIKE ? OR groups LIKE ?)")
		args = append(args, `%"`+g+`"%`, `%`+g+`%`, `%`+g+`"%"`)
		argIdx += 3
	}
	if s := c.Query("status"); s != "" {
		query += fmt.Sprintf(" AND status = ?")
		args = append(args, s)
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
