package file

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/internal/control/transfer"
)

type FieldKind int

const (
	KindText FieldKind = iota
	KindBool
)

type AdvancedField struct {
	key     string
	label   string
	kind    FieldKind
	input   textinput.Model
	checked bool
}

type AdvancedForm struct {
	fields []*AdvancedField
	cursor int
	error  string
}

// newAdvancedForm 仅暴露有效选项: --mode/--overwrite 为死选项, 不搬入
func newAdvancedForm() *AdvancedForm {
	f := &AdvancedForm{}
	specs := []struct {
		key, label, def string
		kind            FieldKind
		checked         bool
	}{
		{"parallel", "parallel", "", KindBool, true},
		{"resume", "resume", "", KindBool, true},
		{"no-overwrite", "no-overwrite", "", KindBool, false},
		{"subdir", "subdir", "", KindBool, false},
		{"name-format", "name-format", "", KindText, false},
	}
	for _, s := range specs {
		ti := textinput.New()
		ti.Width = 20
		ti.CharLimit = 64
		ti.Blur()
		f.fields = append(f.fields, &AdvancedField{key: s.key, label: s.label, kind: s.kind, input: ti, checked: s.checked})
	}
	return f
}

func (f *AdvancedForm) value(key string) string {
	for _, fd := range f.fields {
		if fd.key == key {
			return strings.TrimSpace(fd.input.Value())
		}
	}
	return ""
}

func (f *AdvancedForm) isOn(key string) bool {
	for _, fd := range f.fields {
		if fd.key == key {
			return fd.checked
		}
	}
	return false
}

func (f *AdvancedForm) move(d int) {
	f.cursor = (f.cursor + d + len(f.fields)) % len(f.fields)
}

func (f *AdvancedForm) toggle(i int) {
	f.fields[i].checked = !f.fields[i].checked
}

func (f *AdvancedForm) uploadOpts() *transfer.UploadOptions {
	return &transfer.UploadOptions{
		Parallel:    f.isOn("parallel"),
		NoOverwrite: f.isOn("no-overwrite"),
		Resume:      f.isOn("resume"),
	}
}

func (f *AdvancedForm) downloadOpts() *transfer.DownloadOptions {
	return &transfer.DownloadOptions{
		Parallel:   f.isOn("parallel"),
		Subdir:     f.isOn("subdir"),
		NameFormat: f.value("name-format"),
		Resume:     f.isOn("resume"),
	}
}

func advancedSummary(f *AdvancedForm) string {
	parts := []string{}
	if f.isOn("parallel") {
		parts = append(parts, "parallel")
	} else {
		parts = append(parts, "串行")
	}
	if !f.isOn("resume") {
		parts = append(parts, "no-resume")
	}
	if f.isOn("no-overwrite") {
		parts = append(parts, "no-overwrite")
	}
	return strings.Join(parts, " ")
}

func (m FileModel) updateAdvanced(msg tea.Msg) (tea.Model, tea.Cmd) {
	f := m.advanced
	if f == nil {
		return m, nil
	}
	if m.mode == ModeInsert {
		if km, ok := msg.(tea.KeyMsg); ok && km.String() == "esc" {
			m.mode = ModeNormal
			f.fields[f.cursor].input.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		f.fields[f.cursor].input, cmd = f.fields[f.cursor].input.Update(msg)
		return m, cmd
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "up":
		f.move(-1)
	case "down":
		f.move(1)
	case "enter":
		if f.fields[f.cursor].kind == KindText {
			m.mode = ModeInsert
			f.fields[f.cursor].input.SetValue("")
			f.fields[f.cursor].input.Focus()
		}
	case " ":
		if f.fields[f.cursor].kind == KindBool {
			f.toggle(f.cursor)
		}
	case "s", "esc":
		m.pop()
		m.advanced = nil
	}
	return m, nil
}
