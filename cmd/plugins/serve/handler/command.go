package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/gin-gonic/gin"
)

// ShortcutHandler 管理用户级快捷命令(/api/v1/shortcuts)。
type ShortcutHandler struct {
	commands *store.CommandStore
	users    *store.UserStore
}

func NewShortcutHandler(commands *store.CommandStore, users *store.UserStore) *ShortcutHandler {
	return &ShortcutHandler{commands: commands, users: users}
}

// currentUserID 从 JWT claims.username 反查当前用户 ID。
func (h *ShortcutHandler) currentUserID(c *gin.Context) (int64, bool) {
	claimsVal, ok := c.Get("claims")
	if !ok {
		return 0, false
	}
	claims, ok := claimsVal.(*service.Claims)
	if !ok {
		return 0, false
	}
	user, err := h.users.FindByUsername(c.Request.Context(), claims.Username)
	if err != nil {
		return 0, false
	}
	return user.ID, true
}

type shortcutRequest struct {
	Name    string `json:"name" binding:"required"`
	Command string `json:"command" binding:"required"`
}

func (h *ShortcutHandler) List(c *gin.Context) {
	userID, ok := h.currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "unauthorized"})
		return
	}
	list, err := h.commands.ListByUser(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "query failed"})
		return
	}
	if list == nil {
		list = []*model.UserCommand{}
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *ShortcutHandler) Create(c *gin.Context) {
	userID, ok := h.currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "unauthorized"})
		return
	}
	var req shortcutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "name and command are required"})
		return
	}
	cmd := &model.UserCommand{UserID: userID, Name: req.Name, Command: req.Command}
	if err := h.commands.Create(c.Request.Context(), cmd); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "create failed"})
		return
	}
	c.JSON(http.StatusCreated, cmd)
}

func (h *ShortcutHandler) Update(c *gin.Context) {
	userID, ok := h.currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "unauthorized"})
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid id"})
		return
	}
	var req shortcutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "name and command are required"})
		return
	}
	cmd := &model.UserCommand{ID: id, UserID: userID, Name: req.Name, Command: req.Command}
	result, err := h.commands.Update(c.Request.Context(), cmd)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "update failed"})
		return
	}
	if result == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "not found"})
		return
	}
	c.JSON(http.StatusOK, cmd)
}

func (h *ShortcutHandler) Delete(c *gin.Context) {
	userID, ok := h.currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "unauthorized"})
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid id"})
		return
	}
	result, err := h.commands.Delete(c.Request.Context(), id, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "delete failed"})
		return
	}
	if result == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "not found"})
		return
	}
	okAction(c, "deleted")
}

type reorderRequest struct {
	OrderedIDs []int64 `json:"ordered_ids" binding:"required"`
}

func (h *ShortcutHandler) Reorder(c *gin.Context) {
	userID, ok := h.currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "unauthorized"})
		return
	}
	var req reorderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "ordered_ids is required"})
		return
	}
	if err := h.commands.Reorder(c.Request.Context(), userID, req.OrderedIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "reorder failed"})
		return
	}
	okAction(c, "reordered")
}

// ---- 导出 / 导入（用户级快捷命令迁移） ----

type shortcutExport struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	Position int   `json:"position"`
}

type shortcutExportFile struct {
	Version    string           `json:"version"`
	ExportedAt string           `json:"exported_at,omitempty"`
	Shortcuts  []shortcutExport `json:"shortcuts"`
}

// Export 导出当前用户的全部快捷命令为 JSON 附件（跨部署迁移/备份）。
func (h *ShortcutHandler) Export(c *gin.Context) {
	userID, ok := h.currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "unauthorized"})
		return
	}
	list, err := h.commands.ListByUser(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "query failed"})
		return
	}
	items := make([]shortcutExport, 0, len(list))
	for _, cmd := range list {
		items = append(items, shortcutExport{Name: cmd.Name, Command: cmd.Command, Position: cmd.Position})
	}
	nf := shortcutExportFile{Version: "1.0", ExportedAt: time.Now().UTC().Format(time.RFC3339), Shortcuts: items}
	data, err := json.MarshalIndent(nf, "", "  ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "marshal failed"})
		return
	}
	filename := fmt.Sprintf("owl-shortcuts-%s.json", time.Now().Format("20060102-150405"))
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Data(http.StatusOK, "application/json", data)
}

// maxImportShortcuts 单次导入的条目上限，防误传超大文件拖垮写入。
const maxImportShortcuts = 500

type shortcutImportRequest struct {
	Shortcuts []shortcutExport `json:"shortcuts" binding:"required"`
}

// Import 合并导入快捷命令：同名（与现有或文件内靠前的条目）跳过，
// 不同名追加到列表末尾（复用 Create 的 max(position)+1）。命令不会被覆盖。
func (h *ShortcutHandler) Import(c *gin.Context) {
	userID, ok := h.currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "unauthorized"})
		return
	}
	var req shortcutImportRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Shortcuts) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "shortcuts is required"})
		return
	}
	if len(req.Shortcuts) > maxImportShortcuts {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": fmt.Sprintf("too many entries (max %d)", maxImportShortcuts)})
		return
	}

	ctx := c.Request.Context()
	existing, err := h.commands.ListByUser(ctx, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "query failed"})
		return
	}
	known := make(map[string]bool, len(existing))
	for _, cmd := range existing {
		known[cmd.Name] = true
	}

	var created, skipped, failed int
	var errs []string
	for _, item := range req.Shortcuts {
		name := strings.TrimSpace(item.Name)
		command := strings.TrimSpace(item.Command)
		if name == "" || command == "" {
			failed++
			errs = append(errs, "entry with empty name or command skipped")
			continue
		}
		if known[name] {
			skipped++
			continue
		}
		cmd := &model.UserCommand{UserID: userID, Name: name, Command: command}
		if err := h.commands.Create(ctx, cmd); err != nil {
			failed++
			errs = append(errs, fmt.Sprintf("create %q failed", name))
			continue
		}
		known[name] = true
		created++
	}
	c.JSON(http.StatusOK, gin.H{"created": created, "skipped": skipped, "failed": failed, "errors": errs})
}
