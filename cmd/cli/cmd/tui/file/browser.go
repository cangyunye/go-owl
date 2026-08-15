package file

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type BrowserEntry struct {
	Name  string
	IsDir bool
}

// FileBrowser 本地文件浏览器: 目录列表/排序/导航/路径跳转。
// 纯数据与导航, 按键处理与视图由 FileModel 负责。
type FileBrowser struct {
	dir        string
	entries    []BrowserEntry
	cursor     int
	showHidden bool
	err        string
}

// NewFileBrowser 以 startDir 为起点创建浏览器; startDir 为空时回退当前工作目录
func NewFileBrowser(startDir string) *FileBrowser {
	if startDir == "" {
		if wd, err := os.Getwd(); err == nil {
			startDir = wd
		} else {
			startDir = "."
		}
	}
	b := &FileBrowser{dir: startDir}
	b.reload()
	return b
}

func (b *FileBrowser) reload() {
	entries, err := os.ReadDir(b.dir)
	if err != nil {
		b.err = err.Error()
		b.entries = nil
		b.cursor = 0
		return
	}
	b.err = ""
	b.entries = b.entries[:0]
	for _, e := range entries {
		if !b.showHidden && strings.HasPrefix(e.Name(), ".") {
			continue
		}
		b.entries = append(b.entries, BrowserEntry{Name: e.Name(), IsDir: e.IsDir()})
	}
	// 目录在前, 各自按字母序
	sort.Slice(b.entries, func(i, j int) bool {
		if b.entries[i].IsDir != b.entries[j].IsDir {
			return b.entries[i].IsDir
		}
		return b.entries[i].Name < b.entries[j].Name
	})
	b.clamp()
}

func (b *FileBrowser) clamp() {
	if b.cursor >= len(b.entries) {
		b.cursor = len(b.entries) - 1
	}
	if b.cursor < 0 {
		b.cursor = 0
	}
}

func (b *FileBrowser) Up() { b.cursor--; b.clamp() }

func (b *FileBrowser) Down() { b.cursor++; b.clamp() }

// Enter 光标项: 目录→进入并返回 (path,false,nil); 文件→返回绝对路径 (path,true,nil)
func (b *FileBrowser) Enter() (string, bool, error) {
	if len(b.entries) == 0 {
		return "", false, nil
	}
	e := b.entries[b.cursor]
	path := filepath.Join(b.dir, e.Name)
	if e.IsDir {
		b.dir = path
		b.cursor = 0
		b.reload()
		return path, false, nil
	}
	return path, true, nil
}

// Parent 返回上级目录; 已在根目录时返回 false
func (b *FileBrowser) Parent() bool {
	parent := filepath.Dir(b.dir)
	if parent == b.dir {
		return false
	}
	b.dir = parent
	b.cursor = 0
	b.reload()
	return true
}

// Jump 跳转到指定路径: 目录→进入; 文件→作为当前选中项; 无效路径返回 error 且保持原目录
func (b *FileBrowser) Jump(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		b.dir = path
		b.cursor = 0
		b.reload()
		return nil
	}
	// 文件: 进入其所在目录并把光标指向该文件
	dir := filepath.Dir(path)
	name := filepath.Base(path)
	b.dir = dir
	b.reload()
	for i, e := range b.entries {
		if e.Name == name {
			b.cursor = i
			break
		}
	}
	return nil
}

func (b *FileBrowser) ToggleHidden() {
	b.showHidden = !b.showHidden
	b.reload()
}

func (b *FileBrowser) currentPath() string {
	if len(b.entries) == 0 {
		return b.dir
	}
	return filepath.Join(b.dir, b.entries[b.cursor].Name)
}
