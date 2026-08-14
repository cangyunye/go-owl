package nodes

type ColumnsModel struct {
	order    []string
	checked  []bool
	snapshot []bool
	cursor   int
}

func columnKeys() []string {
	keys := make([]string, len(columnDefs))
	for i, c := range columnDefs {
		keys[i] = c.Key
	}
	return keys
}

func NewColumnsModel(selected []string) *ColumnsModel {
	cm := &ColumnsModel{order: columnKeys()}
	cm.checked = make([]bool, len(cm.order))
	for i, k := range cm.order {
		for _, s := range selected {
			if k == s {
				cm.checked[i] = true
			}
		}
	}
	cm.snapshot = append([]bool(nil), cm.checked...)
	return cm
}

func (cm *ColumnsModel) selected() []string {
	out := []string{}
	for i, k := range cm.order {
		if cm.checked[i] {
			out = append(out, k)
		}
	}
	if len(out) == 0 {
		return append([]string(nil), defaultColumnKeys...)
	}
	return out
}

func (cm *ColumnsModel) toggle(i int) { cm.checked[i] = !cm.checked[i] }

func (cm *ColumnsModel) selectAll() {
	for i := range cm.checked {
		cm.checked[i] = true
	}
}

func (cm *ColumnsModel) reset() {
	cm.checked = make([]bool, len(cm.order))
	for i, k := range cm.order {
		for _, d := range defaultColumnKeys {
			if k == d {
				cm.checked[i] = true
			}
		}
	}
}

func (cm *ColumnsModel) restoreSnapshot() {
	cm.checked = append([]bool(nil), cm.snapshot...)
}
