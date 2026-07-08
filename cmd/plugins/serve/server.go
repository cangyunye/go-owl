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
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/handler"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
)

//go:embed web/index.html web/css/* web/js/* web/js/pages/*
var webFS embed.FS

type Config struct {
	DBPath     string
	ListenAddr string
	DevMode    bool
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

	authHandler      *handler.AuthHandler
	userHandler      *handler.UserHandler
	nodeHandler      *handler.NodeHandler
	settingsHandler  *handler.SettingsHandler
	execHandler      *handler.ExecHandler
	playbookHandler  *handler.PlaybookHandler
	transferHandler  *handler.TransferHandler
	wsHub            *handler.WSHub
}

func NewServer(cfg *Config) *Server {
	return &Server{Config: cfg}
}

func (s *Server) Init() (*AdminCredentials, error) {
	db, err := sql.Open("sqlite", s.Config.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	s.DB = db

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
	playbookRunStore := store.NewPlaybookRunStore(db)
	if err := playbookRunStore.Init(context.Background()); err != nil {
		return nil, fmt.Errorf("init playbook run store: %w", err)
	}
	s.transferHandler = handler.NewTransferHandler(db, s.Tasks)
	s.playbookHandler = handler.NewPlaybookHandler(db, playbookStore, playbookRunStore, s.wsHub)
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

	auth := s.Router.Group("/api/v1", s.authHandler.AuthMiddleware())
	{
		auth.GET("/me", s.authHandler.Me)

		reader := auth.Group("", s.authHandler.RBACMiddleware(model.RoleViewer, model.RoleEditor, model.RoleOperator, model.RoleAdmin))
		{
			reader.GET("/nodes", s.nodeHandler.List)
			reader.GET("/nodes/search", s.nodeHandler.Search)
			reader.GET("/nodes/filters", s.nodeHandler.Filters)
			reader.GET("/nodes/:id", s.nodeHandler.Get)
		}

		reader.GET("/tasks", s.execHandler.List)
		reader.GET("/tasks/:id", s.execHandler.Get)

		writer := auth.Group("", s.authHandler.RBACMiddleware(model.RoleEditor, model.RoleOperator, model.RoleAdmin))
		{
			writer.POST("/nodes", s.nodeHandler.Create)
			writer.PUT("/nodes/:id", s.nodeHandler.Update)
		}

		operator := auth.Group("", s.authHandler.RBACMiddleware(model.RoleOperator, model.RoleAdmin))
		{
			operator.POST("/exec", s.execHandler.Create)
			operator.POST("/transfer", s.transferHandler.Create)
			operator.GET("/transfers", s.transferHandler.List)
			operator.GET("/playbooks", s.playbookHandler.List)
			operator.GET("/playbooks/:name", s.playbookHandler.Get)
			operator.POST("/playbooks/:name/run", s.playbookHandler.Run)
			operator.GET("/playbook/runs", s.playbookHandler.RunList)
			operator.GET("/playbook/runs/:id", s.playbookHandler.RunGet)
			operator.GET("/playbook/settings/path", s.playbookHandler.GetSettingsPath)
		}

		admin := auth.Group("", s.authHandler.RBACMiddleware(model.RoleAdmin))
		{
			admin.DELETE("/nodes/:id", s.nodeHandler.Delete)
			admin.DELETE("/tasks/:id", s.execHandler.Cancel)
			admin.GET("/settings", s.settingsHandler.List)
			admin.GET("/settings/:key", s.settingsHandler.Get)
			admin.PUT("/settings/:key", s.settingsHandler.Set)
			admin.GET("/users", s.userHandler.List)
			admin.GET("/users/:id", s.userHandler.Get)
			admin.POST("/users", s.userHandler.Create)
			admin.PUT("/users/:id", s.userHandler.Update)
			admin.DELETE("/users/:id", s.userHandler.Delete)
			admin.POST("/playbook/refresh", s.playbookHandler.Refresh)
			admin.DELETE("/playbook/runs/:id", s.playbookHandler.RunCancel)
		}
	}

	// Static files
	var staticFS fs.FS
	if s.Config.DevMode {
		wd, _ := os.Getwd()
		staticFS = os.DirFS(filepath.Join(wd, "web"))
	} else {
		sub, _ := fs.Sub(webFS, "web")
		staticFS = sub
	}
	s.Router.StaticFS("/static", http.FS(staticFS))

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

func (s *Server) Start() error {
	if s.Router == nil {
		if _, err := s.Init(); err != nil {
			return err
		}
	}
	return s.Router.Run(s.Config.ListenAddr)
}

func (s *Server) ResetAdmin() (*AdminCredentials, error) {
	db, err := sql.Open("sqlite", s.Config.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	users := store.NewUserStore(db)
	if err := users.Init(context.Background()); err != nil {
		return nil, fmt.Errorf("init user store: %w", err)
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