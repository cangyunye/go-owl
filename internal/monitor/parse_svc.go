package monitor

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseSystemctlShow 解析 `systemctl show <units> -p Id -p LoadState
// -p ActiveState -p NRestarts` 输出（多 unit 时按块顺序、块间空行分隔），
// 产出 svc.active.<unit>（active=1，其余 0）与 svc.restarts.<unit>
// （systemd NRestarts，开机以来累计重启次数）。
// LoadState=not-found 的 unit（发行版差异导致 unit 不存在）跳过，避免把
// "unit 不存在"误报为"服务停止"；同名 unit 块（如别名 sshd→ssh）后者覆盖前者。
func ParseSystemctlShow(raw, nodeID string, ts int64) ([]Sample, error) {
	type unitState struct {
		load, active string
		restarts     float64
	}
	units := make(map[string]*unitState)
	var order []string
	cur := ""
	curState := (*unitState)(nil)

	flush := func() {
		if cur == "" || curState == nil {
			return
		}
		if _, seen := units[cur]; !seen {
			order = append(order, cur)
		}
		units[cur] = curState
		cur = ""
		curState = nil
	}

	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("systemctl: 非法行: %q", line)
		}
		v = strings.TrimSpace(v)
		switch k {
		case "Id":
			if v != cur {
				flush()
				cur = v
				curState = &unitState{}
			}
		case "LoadState":
			if curState != nil {
				curState.load = v
			}
		case "ActiveState":
			if curState != nil {
				curState.active = v
			}
		case "NRestarts":
			if curState != nil {
				if n, err := strconv.ParseFloat(v, 64); err == nil {
					curState.restarts = n
				}
			}
		}
	}
	flush()

	samples := make([]Sample, 0, len(units)*2)
	for _, id := range order {
		st := units[id]
		if st.load == "not-found" {
			continue
		}
		name := strings.TrimSuffix(id, ".service")
		active := 0.0
		if st.active == "active" {
			active = 1
		}
		samples = append(samples,
			Sample{NodeID: nodeID, Metric: "svc.active." + name, TS: ts, Value: active},
			Sample{NodeID: nodeID, Metric: "svc.restarts." + name, TS: ts, Value: st.restarts},
		)
	}
	return samples, nil
}
