package i18n

import (
	"embed"
)

//go:embed locale/*.json
var localeFS embed.FS

// embeddedFiles 返回 文件名(去后缀)->内容 的映射。
func embeddedFiles() map[string][]byte {
	files := map[string][]byte{}
	entries, _ := localeFS.ReadDir("locale")
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := localeFS.ReadFile("locale/" + e.Name())
		if err != nil {
			continue
		}
		name := e.Name()
		if i := len(name) - 5; i > 0 && name[i:] == ".json" {
			name = name[:i]
		}
		files[name] = data
	}
	return files
}