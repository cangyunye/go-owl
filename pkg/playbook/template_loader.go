package playbook

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/cangyunye/go-owl/pkg/playbook/builtin"
)

type TemplateEntry struct {
	Name     string
	Category string
	Source   string
	Path     string
	Meta     *TemplateMeta
	Content  []byte
}

func DefaultUserTemplatePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".owl", "templates")
	}
	return filepath.Join(home, ".owl", "templates")
}

func LoadTemplates(userPath string) ([]*TemplateEntry, error) {
	entries := make(map[string]*TemplateEntry)

	if err := loadFromFS(builtin.Templates, "builtin", entries); err != nil {
		return nil, err
	}

	if info, err := os.Stat(userPath); err == nil && info.IsDir() {
		if err := loadFromDir(userPath, entries); err != nil {
			return nil, err
		}
	}

	result := make([]*TemplateEntry, 0, len(entries))
	for _, e := range entries {
		result = append(result, e)
	}
	return result, nil
}

func GetTemplate(name string, userPath string) (*TemplateEntry, error) {
	entries, err := LoadTemplates(userPath)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.Name == name {
			return e, nil
		}
	}
	return nil, fmt.Errorf("模板不存在: %s", name)
}

func templateNameFromPath(path string) string {
	path = filepath.ToSlash(path)
	return strings.TrimSuffix(path, filepath.Ext(path))
}

func categoryFromPath(path string) string {
	path = filepath.ToSlash(path)
	if i := strings.Index(path, "/"); i >= 0 {
		return path[:i]
	}
	return path
}

func isYAML(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}

func loadFromFS(fsys fs.FS, source string, entries map[string]*TemplateEntry) error {
	return fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !isYAML(path) {
			return nil
		}
		content, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		meta, err := ParseTemplateMeta(content)
		if err != nil {
			return err
		}
		name := templateNameFromPath(path)
		entries[name] = &TemplateEntry{
			Name:     name,
			Category: categoryFromPath(path),
			Source:   source,
			Path:     path,
			Meta:     meta,
			Content:  content,
		}
		return nil
	})
}

func loadFromDir(dir string, entries map[string]*TemplateEntry) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !isYAML(path) {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		meta, err := ParseTemplateMeta(content)
		if err != nil {
			return err
		}
		name := templateNameFromPath(rel)
		entries[name] = &TemplateEntry{
			Name:     name,
			Category: categoryFromPath(rel),
			Source:   "user",
			Path:     path,
			Meta:     meta,
			Content:  content,
		}
		return nil
	})
}
