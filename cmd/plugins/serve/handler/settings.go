package handler

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/cangyunye/go-owl/internal/control/blacklist"
	owlmonitor "github.com/cangyunye/go-owl/internal/monitor"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
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
	// monitor.collect_window 允许空值（空 = 全天采集）
	if err := c.ShouldBindJSON(&req); err != nil || (req.Value == "" && key != "monitor.collect_window") {
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
	case "monitor.enabled":
		switch strings.ToLower(strings.TrimSpace(req.Value)) {
		case "true", "false", "1", "0", "yes", "no", "on", "off":
		default:
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "monitor.enabled must be true or false"})
			return
		}
	case "monitor.collect_window":
		if err := owlmonitor.ValidateCollectWindow(req.Value); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "monitor.collect_window must be HH:MM-HH:MM"})
			return
		}
	case "monitor.alert_retention_days":
		n, err := strconv.Atoi(strings.TrimSpace(req.Value))
		if err != nil || n < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "monitor.alert_retention_days must be a non-negative integer"})
			return
		}
	case "monitor.realert_window_minutes":
		f, err := strconv.ParseFloat(strings.TrimSpace(req.Value), 64)
		if err != nil || f < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "monitor.realert_window_minutes must be a non-negative number"})
			return
		}
	case "monitor.escalate_after_minutes":
		f, err := strconv.ParseFloat(strings.TrimSpace(req.Value), 64)
		if err != nil || f <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "monitor.escalate_after_minutes must be a positive number"})
			return
		}
	case "ai.rate_limit_per_min":
		n, err := strconv.Atoi(strings.TrimSpace(req.Value))
		if err != nil || n < 0 || n > 10000 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "ai.rate_limit_per_min must be 0-10000 (0 = unlimited)"})
			return
		}
	case "monitor.rollback_enabled":
		switch strings.ToLower(strings.TrimSpace(req.Value)) {
		case "true", "false", "1", "0", "yes", "no", "on", "off":
		default:
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "monitor.rollback_enabled must be true or false"})
			return
		}
	case "monitor.verify_enabled":
		switch strings.ToLower(strings.TrimSpace(req.Value)) {
		case "true", "false", "1", "0", "yes", "no", "on", "off":
		default:
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "monitor.verify_enabled must be true or false"})
			return
		}
	case "monitor.verify_delay_seconds":
		n, err := strconv.Atoi(strings.TrimSpace(req.Value))
		if err != nil || n < 0 || n > 600 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "monitor.verify_delay_seconds must be 0-600"})
			return
		}
	case "monitor.retention_days":
		n, err := strconv.Atoi(strings.TrimSpace(req.Value))
		if err != nil || n < 1 || n > 3650 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "monitor.retention_days must be 1-3650"})
			return
		}
	case "monitor.cleanup_schedule":
		switch strings.ToLower(strings.TrimSpace(req.Value)) {
		case "daily", "weekly", "monthly":
		default:
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "monitor.cleanup_schedule must be daily, weekly or monthly"})
			return
		}
	case "monitor.interval_seconds":
		n, err := strconv.Atoi(strings.TrimSpace(req.Value))
		if err != nil || n < 10 || n > 86400 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "monitor.interval_seconds must be 10-86400"})
			return
		}
	case "logs.executions_retention_days":
		n, err := strconv.Atoi(strings.TrimSpace(req.Value))
		if err != nil || n < 0 || n > 3650 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "logs.executions_retention_days must be 0-3650 (0 = disable periodic cleanup)"})
			return
		}
	case "history.retention_days":
		n, err := strconv.Atoi(strings.TrimSpace(req.Value))
		if err != nil || n < 0 || n > 3650 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "history.retention_days must be 0-3650 (0 = disable periodic cleanup)"})
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

// ---- 危险命令黑名单（~/.owl/blacklist.yaml）管理 ----
//
// serve/CLI/AI 的黑名单均以该文件为事实源（newBlacklistChecker 经
// blacklist.LoadConfig 读取）。设置页保存后原子写盘并对共享检查器
// 热重载，全部执行路径即时生效、无需重启。

type blacklistConfigResponse struct {
	Path     string `json:"path"`
	Exists   bool   `json:"exists"`
	Content  string `json:"content"`
	Defaults string `json:"defaults"`
}

type setBlacklistRequest struct {
	Content string `json:"content"`
}

// GetBlacklist 返回黑名单配置文件原文与内置默认规则（文件不存在时
// exists=false，编辑器可「载入默认规则」起步）。
func (h *SettingsHandler) GetBlacklist(c *gin.Context) {
	resp := blacklistConfigResponse{
		Path:     blacklist.ConfigPath(),
		Defaults: renderDefaultBlacklistYAML(),
	}
	if resp.Path == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "cannot resolve home directory"})
		return
	}
	if data, err := os.ReadFile(resp.Path); err == nil {
		resp.Exists = true
		resp.Content = string(data)
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// SetBlacklist 校验并保存黑名单配置：YAML 结构校验（规则必须带 user），
// 原子写入 0600，成功后热重载共享检查器。规则清空属管理员显式决定，
// 允许保存但响应携带警告。
func (h *SettingsHandler) SetBlacklist(c *gin.Context) {
	var req setBlacklistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request body"})
		return
	}
	path := blacklist.ConfigPath()
	if path == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "cannot resolve home directory"})
		return
	}

	cfg, warnings, verr := validateBlacklistYAML(req.Content)
	if verr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": verr.Error()})
		return
	}
	if len(cfg.Rules) == 0 {
		warnings = append(warnings, "规则为空：黑名单将不拦截任何命令（等于关闭拦截）")
	}

	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "create config dir failed"})
			return
		}
	}
	// 原子写入：同目录临时文件 + rename，避免半截文件被并发加载
	tmp, err := os.CreateTemp(filepath.Dir(path), ".blacklist-*.yaml")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "write config failed"})
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.WriteString(req.Content); err != nil {
		tmp.Close()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "write config failed"})
		return
	}
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "chmod config failed"})
		return
	}
	if err := tmp.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "write config failed"})
		return
	}
	if err := os.Rename(tmpName, path); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "write config failed"})
		return
	}

	if _, err := blacklist.ReloadShared(); err != nil {
		log.Printf("reload blacklist checker: %v", err)
	}
	ruleCount := 0
	for _, r := range cfg.Rules {
		ruleCount += len(r.Patterns)
	}
	log.Printf("blacklist config updated by %s: %d rules, %d patterns", c.GetString("username"), len(cfg.Rules), ruleCount)

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"ok":       true,
		"rules":    len(cfg.Rules),
		"patterns": ruleCount,
		"warnings": warnings,
	}})
}

// validateBlacklistYAML 校验黑名单 YAML 结构：必须可解析为规则数组，
// 每条规则必须带 user（缺 user 的规则永不生效，属死配置）。返回解析
// 结果与提示（无法编译为正则的 pattern 将按字面匹配——既有语义，仅提示）。
func validateBlacklistYAML(content string) (*blacklist.Config, []string, error) {
	var cfg blacklist.Config
	if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
		return nil, nil, fmt.Errorf("YAML 解析失败: %w", err)
	}
	var warnings []string
	for i, r := range cfg.Rules {
		if strings.TrimSpace(r.User) == "" {
			return nil, nil, fmt.Errorf("规则 %d 缺少 user 字段（user 为空时规则永不生效）", i+1)
		}
		for _, p := range r.Patterns {
			if _, err := regexp.Compile(`(?i)` + p); err != nil {
				warnings = append(warnings,
					fmt.Sprintf("pattern %q 无法编译为正则，将按字面子串匹配: %v", p, err))
			}
		}
	}
	return &cfg, warnings, nil
}

func renderDefaultBlacklistYAML() string {
	data, err := yaml.Marshal(blacklist.Config{Rules: blacklist.DefaultRules()})
	if err != nil {
		return ""
	}
	return string(data)
}
