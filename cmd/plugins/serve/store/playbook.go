package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"gopkg.in/yaml.v3"
)

type PlaybookStore struct {
	db *sql.DB
}

func NewPlaybookStore(db *sql.DB) *PlaybookStore {
	return &PlaybookStore{db: db}
}

func (s *PlaybookStore) Init(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS playbooks (
			name         TEXT PRIMARY KEY,
			description  TEXT DEFAULT '',
			file_path    TEXT NOT NULL,
			tasks_count  INTEGER DEFAULT 0,
			task_names   TEXT DEFAULT '[]',
			file_exists  INTEGER DEFAULT 1,
			updated_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

type playbookRow struct {
	name        string
	description string
	filePath    string
	tasksCount  int
	taskNames   string
	fileExists  int
	updatedAt   time.Time
}

func (s *PlaybookStore) scanRow(scanner interface {
	Scan(dest ...interface{}) error
}) (*model.Playbook, error) {
	var r playbookRow
	if err := scanner.Scan(&r.name, &r.description, &r.filePath, &r.tasksCount, &r.taskNames, &r.fileExists, &r.updatedAt); err != nil {
		return nil, err
	}
	pb := &model.Playbook{
		Name:        r.name,
		Description: r.description,
		FilePath:    r.filePath,
		TasksCount:  r.tasksCount,
		FileExists:  r.fileExists == 1,
		UpdatedAt:   r.updatedAt.Format(time.RFC3339),
	}
	json.Unmarshal([]byte(r.taskNames), &pb.TaskNames)
	if pb.TaskNames == nil {
		pb.TaskNames = []string{}
	}
	return pb, nil
}

func (s *PlaybookStore) List(ctx context.Context) ([]*model.Playbook, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT name, description, file_path, tasks_count, task_names, file_exists, updated_at FROM playbooks ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]*model.Playbook, 0)
	for rows.Next() {
		pb, err := s.scanRow(rows)
		if err != nil {
			continue
		}
		result = append(result, pb)
	}
	return result, nil
}

func (s *PlaybookStore) Get(ctx context.Context, name string) (*model.Playbook, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT name, description, file_path, tasks_count, task_names, file_exists, updated_at FROM playbooks WHERE name = ?`, name)
	return s.scanRow(row)
}

func (s *PlaybookStore) Upsert(ctx context.Context, pb *model.Playbook) error {
	taskNamesJSON, _ := json.Marshal(pb.TaskNames)
	fileExists := 0
	if pb.FileExists {
		fileExists = 1
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO playbooks (name, description, file_path, tasks_count, task_names, file_exists, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			description = excluded.description,
			file_path = excluded.file_path,
			tasks_count = excluded.tasks_count,
			task_names = excluded.task_names,
			file_exists = excluded.file_exists,
			updated_at = excluded.updated_at`,
		pb.Name, pb.Description, pb.FilePath, pb.TasksCount, string(taskNamesJSON), fileExists, time.Now().UTC())
	return err
}

func (s *PlaybookStore) Delete(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM playbooks WHERE name = ?`, name)
	return err
}

func (s *PlaybookStore) SyncFromDir(ctx context.Context, dir string) ([]*model.Playbook, error) {
	diskMap := make(map[string]*model.Playbook)

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, err
	}

	filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".yml") && !strings.HasSuffix(path, ".yaml") {
			return nil
		}
		pb := readPlaybookMeta(path)
		if pb != nil {
			diskMap[pb.Name] = pb
		}
		return nil
	})

	var results []*model.Playbook
	for _, pb := range diskMap {
		if err := s.Upsert(ctx, pb); err == nil {
			results = append(results, pb)
		}
	}

	existing, _ := s.List(ctx)
	for _, pb := range existing {
		if _, stillExists := diskMap[pb.Name]; !stillExists {
			pb.FileExists = false
			s.Upsert(ctx, pb)
		}
	}

	return results, nil
}

type playbookYAMLMeta struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Tasks       []any    `yaml:"tasks"`
}

func readPlaybookMeta(path string) *model.Playbook {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var meta playbookYAMLMeta
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil
	}
	name := meta.Name
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	var taskNames []string
	if len(meta.Tasks) > 0 {
		for _, t := range meta.Tasks {
			if m, ok := t.(map[string]interface{}); ok {
				for k := range m {
					taskNames = append(taskNames, k)
					break
				}
			}
		}
	}
	return &model.Playbook{
		Name:        name,
		Description: meta.Description,
		FilePath:    path,
		TasksCount:  len(meta.Tasks),
		TaskNames:   taskNames,
		FileExists:  true,
	}
}
