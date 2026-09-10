package handler

import (
	"database/sql"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"
)

type SettingResponse struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type setSettingRequest struct {
	Value string `json:"value"`
}

type SettingsHandler struct {
	db *sql.DB
}

// sensitiveSettings 是既不可读也不可写的键。jwt_secret 用于签发与校验会话
// 令牌：读出来可离线伪造任意账号，写进去会使全部既有 token 失效且语义混乱。
var sensitiveSettings = map[string]bool{
	"jwt_secret": true,
}

func NewSettingsHandler(db *sql.DB) *SettingsHandler {
	return &SettingsHandler{db: db}
}

func (h *SettingsHandler) List(c *gin.Context) {
	rows, err := h.db.Query(`SELECT key, value FROM settings ORDER BY key`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "query error"})
		return
	}
	defer rows.Close()

	settings := make([]SettingResponse, 0)
	for rows.Next() {
		var s SettingResponse
		if err := rows.Scan(&s.Key, &s.Value); err == nil && !sensitiveSettings[s.Key] {
			settings = append(settings, s)
		}
	}

	c.JSON(http.StatusOK, gin.H{"data": settings})
}

func (h *SettingsHandler) Get(c *gin.Context) {
	key := c.Param("key")
	if sensitiveSettings[key] {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "setting is not accessible"})
		return
	}
	var s SettingResponse
	err := h.db.QueryRow(`SELECT key, value FROM settings WHERE key = ?`, key).Scan(&s.Key, &s.Value)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "setting not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "query error"})
		return
	}
	c.JSON(http.StatusOK, s)
}

func (h *SettingsHandler) Set(c *gin.Context) {
	key := c.Param("key")
	if sensitiveSettings[key] {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "setting is read-only"})
		return
	}
	var req setSettingRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Value == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "value is required"})
		return
	}

	switch key {
	case "staging_dir":
		if !filepath.IsAbs(req.Value) {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "staging_dir must be an absolute path"})
			return
		}
	case "staging_min_free":
		n, err := strconv.ParseUint(req.Value, 10, 64)
		if err != nil || n == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "staging_min_free must be a positive integer"})
			return
		}
	}

	_, err := h.db.Exec(
		`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, req.Value,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "save failed"})
		return
	}

	c.JSON(http.StatusOK, SettingResponse{Key: key, Value: req.Value})
}
