package nodes

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/textinput"
	"gopkg.in/yaml.v3"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

type ImportExportModel struct {
	op        string // "export" | "import"
	format    string // "yaml" | "json"
	path      textinput.Model
	overwrite bool
	error     string
}

func NewImportExportModel() *ImportExportModel {
	ti := textinput.New()
	ti.Placeholder = "./nodes.yaml"
	ti.Width = 40
	ti.CharLimit = 256
	ti.Focus()
	return &ImportExportModel{op: "export", format: "yaml", path: ti}
}

type nodeFile struct {
	Version string             `json:"version" yaml:"version"`
	Nodes   []*common.NodeInfo `json:"nodes" yaml:"nodes"`
}

func (m Model) doExport(path, format string) error {
	nodes, err := m.store.List()
	if err != nil {
		return err
	}
	nf := nodeFile{Version: "1.0", Nodes: nodes}
	var data []byte
	if format == "json" {
		data, err = json.MarshalIndent(nf, "", "  ")
	} else {
		data, err = yaml.Marshal(nf)
	}
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (m Model) doImport(path string, overwrite bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var nf nodeFile
	if err := yaml.Unmarshal(data, &nf); err != nil {
		if jsonErr := json.Unmarshal(data, &nf); jsonErr != nil {
			return fmt.Errorf("解析导入文件失败: %v", err)
		}
	}
	for _, node := range nf.Nodes {
		if node.ID == "" || node.Name == "" || node.Address == "" {
			continue
		}
		_, exists := m.store.Get(node.ID)
		if exists == nil && !overwrite {
			continue
		}
		if exists != nil {
			if err := m.store.Add(node); err != nil {
				return err
			}
		} else if err := m.store.Update(node); err != nil {
			return err
		}
	}
	return m.store.Save()
}
