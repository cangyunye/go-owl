package serve

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/handler"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	ai2 "github.com/cangyunye/go-owl/internal/ai"
	"github.com/cangyunye/go-owl/internal/control/node"
	"github.com/cangyunye/go-owl/internal/history"
	"github.com/cangyunye/go-owl/internal/logfile"
	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
)

//go:embed web/index.html web/favicon.svg web/css/* web/js/* web/js/pages/*
var webFS embed.FS

type Config struct {
	DBPath      string
	ListenAddr  string
	DevMode     bool
	AIDebugMode bool
}

type AdminCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type Server struct {
	Config *Config
	DB     *sql.DB
	Users  *store.UserStore
	Tasks  *store.TaskStore
	Auth   *service.AuthService
	Router *gin.Engine

	authHandler         *handler.AuthHandler
	userHandler         *handler.UserHandler
	nodeHandler         *handler.NodeHandler
	settingsHandler     *handler.SettingsHandler
	execHandler         *handler.ExecHandler
	playbookHandler     *handler.PlaybookHandler
	stagingHandler      *handler.StagingHandler
	transferRecordStore *store.TransferRecordStore
	transferHandler     *handler.TransferHandler
	auditStore          *store.AIAuditStore
	keyManager          *handler.KeyManager
	aiHandler           *handler.AIHandler
	wsHub               *handler.WSHub
	historyHandler      *handler.HistoryHandler
	logHandler          *handler.LogHandler
	History             *store.HistoryStore
	terminalHandler     *handler.TerminalHandler
}

func NewServer(cfg *Config) *Server {
	return &Server{Config: cfg}
}

func (s *Server) Init() (*AdminCredentials, error) {
	if err := ensureDBDir(s.Config.DBPath); err != nil {
		return nil, fmt.Errorf("ensure db dir: %w", err)
	}
	db, err := sql.Open("sqlite", sqliteDSN(s.Config.DBPath))
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	s.DB = db

	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.ExecContext(context.Background(), pragma); err != nil {
			return nil, fmt.Errorf("set pragma: %w", err)
		}
	}

	// Init stores
	s.Users = store.NewUserStore(db)
	if err := s.Users.Init(context.Background()); err != nil {
		return nil, fmt.Errorf("init user store: %w", err)
	}
	s.Tasks = store.NewTaskStore(db)
	if err := s.Tasks.Init(context.Background()); err != nil {
		return nil, fmt.Errorf("init task store: %w", err)
	}
	if err := initNodes(context.Background(), db); err != nil {
		return nil, fmt.Errorf("init nodes: %w", err)
	}
	if err := initSettings(context.Background(), db); err != nil {
		return nil, fmt.Errorf("init settings: %w", err)
	}

	if _, err := history.NewDB(history.DefaultConfig()); err != nil {
		return nil, fmt.Errorf("init shared history db: %w", err)
	}

	// JWT secret
	secret, err := getOrCreateJWTSecret(context.Background(), db, s.Config.DBPath)
	if err != nil {
		return nil, fmt.Errorf("jwt secret: %w", err)
	}
	s.Auth = service.NewAuthService(secret)

	// Admin user
	creds, err := ensureAdmin(context.Background(), s.Users, s.Auth)
	if err != nil {
		return nil, fmt.Errorf("ensure admin: %w", err)
	}

	s.authHandler = handler.NewAuthHandler(s.Users, s.Auth)
	s.userHandler = handler.NewUserHandler(s.Users, s.Auth)
	s.nodeHandler = handler.NewNodeHandler(db)
	s.settingsHandler = handler.NewSettingsHandler(db)
	s.wsHub = handler.NewWSHub()
	s.execHandler = handler.NewExecHandler(db, s.Tasks, s.wsHub)

	playbookStore := store.NewPlaybookStore(db)
	if err := playbookStore.Init(context.Background()); err != nil {
		return nil, fmt.Errorf("init playbook store: %w", err)
	}

	home, err := os.UserHomeDir()
	if err == nil {
		globalDir := filepath.Join(home, ".owl", "playbooks")
		if err := os.MkdirAll(globalDir, 0755); err == nil {
			entries, _ := os.ReadDir(globalDir)
			if len(entries) == 0 {
				copySamplePlaybooks(globalDir)
			}
			if _, _, err := playbookStore.SyncFromDir(context.Background(), globalDir); err != nil {
				log.Printf("sync playbook library: %v", err)
			}
		}
	}

	playbookRunStore := store.NewPlaybookRunStore(db)
	if err := playbookRunStore.Init(context.Background()); err != nil {
		return nil, fmt.Errorf("init playbook run store: %w", err)
	}
	s.transferRecordStore = store.NewTransferRecordStore(db)
	if err := s.transferRecordStore.Init(context.Background()); err != nil {
		return nil, fmt.Errorf("init transfer record store: %w", err)
	}
	s.stagingHandler = handler.NewStagingHandler(db)
	s.transferHandler = handler.NewTransferHandler(db, s.Tasks, s.transferRecordStore)

	nodeStore := store.NewNodeStore(db)

	s.auditStore = store.NewAIAuditStore(db)
	if err := s.auditStore.Init(context.Background()); err != nil {
		return nil, fmt.Errorf("init ai audit store: %w", err)
	}

	s.keyManager = handler.NewKeyManager()

	webExecutor := handler.NewWebExecutor(db, s.Tasks, s.transferRecordStore,
		playbookRunStore, nodeStore, playbookStore, s.auditStore, s.keyManager, s.Config.AIDebugMode)

	config := &ai2.Config{}
	agent, err := ai2.NewAgent(webExecutor, config,
		node.NewManager(node.NewInMemoryNodeStore()),
		nil, nil, s.Config.AIDebugMode)
	if err != nil {
		return nil, fmt.Errorf("init ai agent: %w", err)
	}

	s.aiHandler = handler.NewAIHandler(db, s.auditStore, webExecutor, s.keyManager, agent, s.Config.AIDebugMode)
	s.playbookHandler = handler.NewPlaybookHandler(db, playbookStore, playbookRunStore, nodeStore, s.wsHub)

	s.History = store.NewHistoryStore(db)
	if err := s.History.Init(context.Background()); err != nil {
		return nil, fmt.Errorf("init history store: %w", err)
	}

	s.nodeHandler.History = s.History
	s.nodeHandler.Hub = s.wsHub
	s.execHandler.History = s.History
	s.execHandler.LogWriter = logfile.NewNodeLogWriter("")
	s.transferHandler.History = s.History
	s.transferHandler.Hub = s.wsHub
	s.playbookHandler.History = s.History
	webExecutor.History = s.History
	webExecutor.PlaybookHandler = s.playbookHandler
	s.historyHandler = handler.NewHistoryHandler(s.History)
	s.logHandler = handler.NewLogHandler()
	s.terminalHandler = handler.NewTerminalHandler(db, s.Auth)

	s.setupRoutes()

	return creds, nil
}

func (s *Server) setupRoutes() {
	s.Router = gin.New()
	s.Router.Use(gin.Recovery())

	// API routes
	s.Router.GET("/api/v1/health", handler.Health)
	s.Router.POST("/api/v1/login", s.authHandler.Login)
	s.Router.GET("/api/v1/ws", s.wsHub.WsHandler(s.authHandler))
	s.Router.GET("/api/v1/session/terminal", s.terminalHandler.Terminal)

	auth := s.Router.Group("/api/v1", s.authHandler.AuthMiddleware())
	{
		auth.GET("/me", s.authHandler.Me)

		reader := auth.Group("", s.authHandler.RBACMiddleware(model.RoleViewer, model.RoleEditor, model.RoleOperator, model.RoleAdmin))
		{
			reader.GET("/nodes", s.nodeHandler.List)
			reader.GET("/nodes/stats", s.nodeHandler.Stats)
			reader.GET("/nodes/search", s.nodeHandler.Search)
			reader.GET("/nodes/filters", s.nodeHandler.Filters)
			reader.GET("/nodes/:id", s.nodeHandler.Get)
		}

		reader.GET("/tasks", s.execHandler.List)
		reader.GET("/tasks/:id", s.execHandler.Get)
		reader.GET("/staging/files", s.stagingHandler.List)
		reader.GET("/staging/disk", s.stagingHandler.DiskInfo)
		reader.GET("/transfer/records", s.transferHandler.Records)
		reader.GET("/transfer/records/:id", s.transferHandler.RecordGet)
		reader.GET("/ai/session-key", s.aiHandler.GetSessionKey)
		reader.GET("/ai/permissions", s.aiHandler.Permissions)
		reader.GET("/ai/context", s.aiHandler.GetContext)
		reader.POST("/ai/chat", s.aiHandler.Chat)
		reader.POST("/ai/audit", s.aiHandler.Audit)
		reader.POST("/ai/models", s.aiHandler.Models)
		reader.POST("/ai/test", s.aiHandler.Test)
		reader.GET("/history", s.historyHandler.List)
		reader.GET("/history/stats", s.historyHandler.Stats)
		reader.GET("/history/export", s.historyHandler.Export)
		reader.GET("/history/detail/:task_id", s.historyHandler.Get)
		reader.GET("/executions/:op_id/logs", s.logHandler.List)
		reader.GET("/executions/:op_id/logs/archive", s.logHandler.Archive)
		reader.GET("/executions/:op_id/logs/:node_id", s.logHandler.Download)

		writer := auth.Group("", s.authHandler.RBACMiddleware(model.RoleEditor, model.RoleOperator, model.RoleAdmin))
		{
			writer.POST("/nodes", s.nodeHandler.Create)
			writer.PUT("/nodes/:id", s.nodeHandler.Update)
			writer.POST("/nodes/batch/groups", s.nodeHandler.BatchGroups)
			writer.POST("/nodes/export", s.nodeHandler.Export)
			writer.POST("/nodes/import", s.nodeHandler.Import)
			writer.POST("/nodes/ping", s.nodeHandler.Ping)
			writer.POST("/nodes/check", s.nodeHandler.Check)
		}

		operator := auth.Group("", s.authHandler.RBACMiddleware(model.RoleOperator, model.RoleAdmin))
		{
			operator.POST("/exec", s.execHandler.Create)
			operator.POST("/transfer", s.transferHandler.Create)
			operator.POST("/transfer/records/:id/rerun", s.transferHandler.Rerun)
			operator.GET("/transfers", s.transferHandler.List)
			operator.POST("/staging/upload", s.stagingHandler.Upload)
			operator.POST("/playbooks/upload", s.playbookHandler.Upload)
			operator.GET("/playbooks", s.playbookHandler.List)
			operator.GET("/playbooks/:id", s.playbookHandler.Get)
			operator.GET("/playbooks/:id/file", s.playbookHandler.GetFile)
			operator.GET("/playbooks/:id/download", s.playbookHandler.Download)
			operator.GET("/playbooks/:id/edit", s.playbookHandler.Edit)
			operator.POST("/playbooks/:id/run", s.playbookHandler.Run)
			operator.GET("/playbook/runs", s.playbookHandler.RunList)
			operator.GET("/playbook/runs/:id", s.playbookHandler.RunGet)
			operator.GET("/playbook/settings/path", s.playbookHandler.GetSettingsPath)
		}

		admin := auth.Group("", s.authHandler.RBACMiddleware(model.RoleAdmin))
		{
			admin.DELETE("/nodes/:id", s.nodeHandler.Delete)
			admin.DELETE("/tasks/:id", s.execHandler.Cancel)
			admin.DELETE("/staging/:name", s.stagingHandler.Delete)
			admin.GET("/settings", s.settingsHandler.List)
			admin.GET("/settings/:key", s.settingsHandler.Get)
			admin.PUT("/settings/:key", s.settingsHandler.Set)
			admin.GET("/users", s.userHandler.List)
			admin.GET("/users/:id", s.userHandler.Get)
			admin.POST("/users", s.userHandler.Create)
			admin.PUT("/users/:id", s.userHandler.Update)
			admin.DELETE("/users/:id", s.userHandler.Delete)
			admin.POST("/playbook/template", s.playbookHandler.Create)
			admin.POST("/playbook/refresh", s.playbookHandler.Refresh)
			admin.DELETE("/playbook/runs/:id", s.playbookHandler.RunCancel)
			admin.DELETE("/history", s.historyHandler.Clean)
		}
	}

	// Static files
	var staticFS fs.FS
	if s.Config.DevMode {
		staticFS = s.devStaticFS()
	} else {
		sub, _ := fs.Sub(webFS, "web")
		staticFS = sub
	}
	static := s.Router.Group("/static")
	static.Use(func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache")
		c.Next()
	})
	static.StaticFS("/", http.FS(staticFS))

	// SPA catch-all: serve index.html for non-API routes
	indexBytes, _ := fs.ReadFile(staticFS, "index.html")
	s.Router.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "not found"})
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexBytes)
	})
}

func initNodes(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS nodes (
			id TEXT PRIMARY KEY,
			name TEXT,
			address TEXT,
			port INTEGER DEFAULT 22,
			user TEXT,
			password TEXT,
			ssh_key TEXT,
			status TEXT DEFAULT 'unknown',
			groups TEXT DEFAULT '[]',
			labels TEXT DEFAULT '{}',
			proxy_jump TEXT DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

func initSettings(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)
	`)
	return err
}

func getOrCreateJWTSecret(ctx context.Context, db *sql.DB, dbPath string) (string, error) {
	var secret string
	err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'jwt_secret'`).Scan(&secret)
	if err == nil {
		return secret, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}

	// Derive secret from db path + random salt
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	hash := sha256.Sum256(append([]byte(dbPath), salt...))
	secret = hex.EncodeToString(hash[:])

	_, err = db.ExecContext(ctx,
		`INSERT INTO settings (key, value) VALUES ('jwt_secret', ?)`, secret)
	if err != nil {
		return "", err
	}
	return secret, nil
}

func ensureAdmin(ctx context.Context, users *store.UserStore, auth *service.AuthService) (*AdminCredentials, error) {
	count, err := users.Count(ctx)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, nil
	}

	password := generatePassword(12)
	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, err
	}

	err = users.Create(ctx, &model.User{
		Username:     "admin",
		PasswordHash: hash,
		Role:         model.RoleAdmin,
		DisplayName:  "Administrator",
	})
	if err != nil {
		return nil, err
	}

	return &AdminCredentials{Username: "admin", Password: password}, nil
}

func generatePassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	pwd := make([]byte, length)
	for i := range pwd {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			// fallback to a simpler approach
			pwd[i] = charset[i%len(charset)]
			continue
		}
		pwd[i] = charset[n.Int64()]
	}
	return string(pwd)
}

// devStaticFS resolves the web/ directory for dev mode by walking up from
// the current working directory until it finds go.mod (project root).
func (s *Server) devStaticFS() fs.FS {
	wd, err := os.Getwd()
	if err != nil {
		return os.DirFS("web")
	}
	dir := wd
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return os.DirFS(filepath.Join(dir, "cmd", "plugins", "serve", "web"))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	// fallback: try CWD/web (works if user cd'd into cmd/plugins/serve/)
	return os.DirFS(filepath.Join(wd, "web"))
}

func (s *Server) Start() error {
	if s.Router == nil {
		if _, err := s.Init(); err != nil {
			return err
		}
	}
	return s.Router.Run(s.Config.ListenAddr)
}

func copySamplePlaybooks(dir string) {
	samples := map[string]string{
		"example/ping-test.yaml": `name: ping_test
description: Test node connectivity via ping
hosts: []
tasks:
  - name: Ping localhost
    command: ping -c 1 127.0.0.1
`,
	}
	for relPath, content := range samples {
		p := filepath.Join(dir, relPath)
		os.MkdirAll(filepath.Dir(p), 0755)
		os.WriteFile(p, []byte(content), 0644)
	}
}

func ensureDBDir(dbPath string) error {
	dir := filepath.Dir(dbPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.MkdirAll(dir, 0755)
	}
	return nil
}

// sqliteDSN 在 DSN 中注入 PRAGMA,确保连接池的每个连接都带上
// WAL / busy_timeout。仅靠启动时 Exec PRAGMA 只作用于单个池化连接,
// 并发写(shell 流式落库)时其他连接会立即报 "database is locked"。
func sqliteDSN(dbPath string) string {
	if strings.Contains(dbPath, "?") {
		return dbPath
	}
	return dbPath + "?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL"
}

func (s *Server) ResetAdmin() (*AdminCredentials, error) {
	if err := ensureDBDir(s.Config.DBPath); err != nil {
		return nil, fmt.Errorf("ensure db dir: %w", err)
	}
	db, err := sql.Open("sqlite", sqliteDSN(s.Config.DBPath))
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	users := store.NewUserStore(db)
	if err := users.Init(context.Background()); err != nil {
		return nil, fmt.Errorf("init user store: %w", err)
	}

	if err := initSettings(context.Background(), db); err != nil {
		return nil, fmt.Errorf("init settings: %w", err)
	}

	secret, err := getOrCreateJWTSecret(context.Background(), db, s.Config.DBPath)
	if err != nil {
		return nil, fmt.Errorf("jwt secret: %w", err)
	}
	auth := service.NewAuthService(secret)

	password := generatePassword(12)
	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user, err := users.FindByUsername(context.Background(), "admin")
	if err == nil {
		if err := users.Delete(context.Background(), user.ID); err != nil {
			return nil, fmt.Errorf("delete old admin: %w", err)
		}
	}

	err = users.Create(context.Background(), &model.User{
		Username:     "admin",
		PasswordHash: hash,
		Role:         model.RoleAdmin,
		DisplayName:  "Administrator",
	})
	if err != nil {
		return nil, fmt.Errorf("recreate admin: %w", err)
	}

	return &AdminCredentials{Username: "admin", Password: password}, nil
}
