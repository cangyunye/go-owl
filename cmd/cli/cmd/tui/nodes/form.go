package nodes

import (
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

type FormMode int

const (
	FormAdd FormMode = iota
	FormEdit
)

type FormField struct {
	key      string
	label    string
	input    textinput.Model
	required bool
	editable bool
}

type FormModel struct {
	mode           FormMode
	base           *common.NodeInfo
	fields         []*FormField
	cursor         int
	error          string
	original       map[string]string
	confirmDiscard bool
}

func NewFormModel(mode FormMode, node *common.NodeInfo) *FormModel {
	f := &FormModel{mode: mode, base: node, original: map[string]string{}}
	var base common.NodeInfo
	if node != nil {
		base = *node
	}
	portVal := ""
	if base.Port > 0 {
		portVal = strconv.Itoa(base.Port)
	}
	specs := []struct {
		key, label string
		req        bool
		val        string
		editable   bool
	}{
		{"id", "ID", mode == FormAdd, base.ID, mode == FormAdd},
		{"name", "Name", true, base.Name, true},
		{"address", "Address", true, base.Address, true},
		{"port", "Port", false, portVal, true},
		{"user", "User", false, base.User, true},
		{"password", "Password", false, base.Password, true},
		{"ssh_key", "SSHKey", false, base.SSHKey, true},
		{"proxy_jump", "ProxyJump", false, base.ProxyJump, true},
		{"groups", "Groups", false, strings.Join(base.Groups, ","), true},
		{"labels", "Labels", false, sortedLabels(base.Labels), true},
		{"status", "Status", false, base.Status, true},
	}
	for _, s := range specs {
		ti := textinput.New()
		ti.SetValue(s.val)
		ti.Placeholder = s.label
		ti.Width = 30
		ti.CharLimit = 256
		ti.Blur()
		f.fields = append(f.fields, &FormField{key: s.key, label: s.label, input: ti, required: s.req, editable: s.editable})
		f.original[s.key] = s.val
	}
	f.cursor = f.firstEditable()
	return f
}

func (f *FormModel) firstEditable() int {
	for i, fd := range f.fields {
		if fd.editable {
			return i
		}
	}
	return 0
}

func (f *FormModel) IsDirty() bool {
	for _, fd := range f.fields {
		if fd.input.Value() != f.original[fd.key] {
			return true
		}
	}
	return false
}

// move 在可编辑字段间移动并首尾回卷(跳过只读字段)
func (f *FormModel) move(d int) {
	if f.editableCount() == 0 {
		return
	}
	for i := 0; i < len(f.fields); i++ {
		f.cursor = (f.cursor + d + len(f.fields)) % len(f.fields)
		if f.fields[f.cursor].editable {
			return
		}
	}
}

func (f *FormModel) editableCount() int {
	n := 0
	for _, fd := range f.fields {
		if fd.editable {
			n++
		}
	}
	return n
}

func (f *FormModel) validate() string {
	for _, fd := range f.fields {
		if !fd.editable {
			continue
		}
		if fd.required && strings.TrimSpace(fd.input.Value()) == "" {
			return fd.label + " 不能为空"
		}
		if fd.key == "port" {
			v := strings.TrimSpace(fd.input.Value())
			if v != "" {
				p, err := strconv.Atoi(v)
				if err != nil || p < 1 || p > 65535 {
					return "Port 必须是 1-65535 的整数"
				}
			}
		}
		if fd.key == "status" {
			v := strings.TrimSpace(fd.input.Value())
			if v != "" && v != "online" && v != "offline" {
				return "Status 必须是 online 或 offline"
			}
		}
	}
	return ""
}

func (f *FormModel) focusFirstInvalid() {
	for i, fd := range f.fields {
		if !fd.editable {
			continue
		}
		if fd.required && strings.TrimSpace(fd.input.Value()) == "" {
			f.cursor = i
			return
		}
		if fd.key == "port" {
			v := strings.TrimSpace(fd.input.Value())
			if v != "" {
				if p, err := strconv.Atoi(v); err != nil || p < 1 || p > 65535 {
					f.cursor = i
					return
				}
			}
		}
		if fd.key == "status" {
			v := strings.TrimSpace(fd.input.Value())
			if v != "" && v != "online" && v != "offline" {
				f.cursor = i
				return
			}
		}
	}
}

func (f *FormModel) nodeID() string {
	if f.base != nil {
		return f.base.ID
	}
	return f.value("id")
}

func (f *FormModel) value(key string) string {
	for _, fd := range f.fields {
		if fd.key == key {
			return strings.TrimSpace(fd.input.Value())
		}
	}
	return ""
}

func (f *FormModel) toNode() *common.NodeInfo {
	now := time.Now().Format(time.RFC3339)
	port := 22
	if v := f.value("port"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p >= 1 {
			port = p
		}
	}
	n := &common.NodeInfo{
		ID:        f.value("id"),
		Name:      f.value("name"),
		Address:   f.value("address"),
		Port:      port,
		User:      f.value("user"),
		Password:  f.value("password"),
		SSHKey:    f.value("ssh_key"),
		ProxyJump: f.value("proxy_jump"),
		Groups:    splitTrim(f.value("groups"), ","),
		Labels:    parseLabels(f.value("labels")),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if f.mode == FormEdit && f.base != nil {
		n.CreatedAt = f.base.CreatedAt
		if port == 22 && f.base.Port != 22 && f.value("port") == "" {
			n.Port = f.base.Port
		}
	}
	if st := f.value("status"); st != "" {
		n.Status = st
	} else if f.mode == FormAdd {
		n.Status = "offline"
	} else if f.base != nil {
		n.Status = f.base.Status
	} else {
		n.Status = "offline"
	}
	return n
}

func parseLabels(s string) map[string]string {
	out := map[string]string{}
	for _, pair := range strings.Split(s, ",") {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			if k != "" {
				out[k] = v
			}
		}
	}
	return out
}

func splitTrim(s, sep string) []string {
	out := []string{}
	for _, p := range strings.Split(s, sep) {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
