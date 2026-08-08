package i18n

import (
	"encoding/json"
	"fmt"
	"sync"

	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/cangyunye/go-owl/internal/locale"
)

// SourceLang 是源语言：en 缺失的 key 回退到此，此处再缺失则原样返回 key。
var SourceLang = language.Chinese

var (
	mu       sync.RWMutex
	printer  = message.NewPrinter(SourceLang)
	active   = SourceLang
	rawCat   = map[language.Tag]map[string]string{}
	regOnce  sync.Once
)

// Init 以给定语言初始化全局打印器，并注册内嵌目录（仅首次）。
// tag 会被规范化为基础语言（zh/en），与目录 key 对齐。
func Init(tag language.Tag) {
	regOnce.Do(registerCatalogs)
	SetLang(tag)
}

// SetLang 运行时切换语言。tag 会被规范化为基础语言（zh/en）。
func SetLang(tag language.Tag) {
	mu.Lock()
	defer mu.Unlock()
	active = locale.CanonicalLang(tag.String())
	printer = message.NewPrinter(active)
}

// ActiveLang 返回当前激活语言。
func ActiveLang() language.Tag {
	mu.RLock()
	defer mu.RUnlock()
	return active
}

func registerCatalogs() {
	for lang, msgs := range catalogs() {
		for key, str := range msgs {
			message.SetString(lang, key, str)
		}
		mu.Lock()
		rawCat[lang] = msgs
		mu.Unlock()
	}
}

// F 用原生 fmt 渲染数值，绕开 message.Printer 的本地化数字格式
// （否则 %d/%v 数值会被插入千位分隔符，如 2222 -> "2,222"）。
// 数字类参数应先用 F 转成字符串，再配合目录中的 %s 使用。
func F(v interface{}) string { return fmt.Sprintf("%v", v) }

// T 按当前语言查消息目录；缺失时回退源语言，源语言再缺失则返回 key 本身。
func T(key string, args ...interface{}) string {
	regOnce.Do(registerCatalogs)
	mu.RLock()
	defer mu.RUnlock()
	return printer.Sprintf(key, args...)
}

// Raw 返回当前语言下的原始消息模板（不格式化参数）。
// 用于需要保留 %w 包装的 fmt.Errorf 场景。
func Raw(key string) string {
	regOnce.Do(registerCatalogs)
	mu.RLock()
	defer mu.RUnlock()
	if m, ok := rawCat[active]; ok {
		if s, ok := m[key]; ok {
			return s
		}
	}
	if m, ok := rawCat[SourceLang]; ok {
		if s, ok := m[key]; ok {
			return s
		}
	}
	return key
}

// catalogs 载入内嵌语言文件，返回 map[语言名]消息表。
// 统一规范化为基础 tag（zh/en），与 resolveLang 返回值对齐。
func catalogs() map[language.Tag]map[string]string {
	out := map[language.Tag]map[string]string{}
	for name, data := range embeddedFiles() {
		var msgs map[string]string
		if err := json.Unmarshal(data, &msgs); err != nil {
			continue
		}
		out[locale.CanonicalLang(name)] = msgs
	}
	return out
}