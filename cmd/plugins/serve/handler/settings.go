package handler

import (
	"database/sql"
	"net/http"

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
		if err := rows.Scan(&s.Key, &s.Value); err == nil {
			settings = append(settings, s)
		}
	}

	c.JSON(http.StatusOK, gin.H{"data": settings})
}

func (h *SettingsHandler) Get(c *gin.Context) {
	key := c.Param("key")
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
	var req setSettingRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Value == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "value is required"})
		return
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
