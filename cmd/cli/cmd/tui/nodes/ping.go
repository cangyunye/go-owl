package nodes

import (
	"net"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

const pingTimeout = 3 * time.Second

type PingResult struct {
	Node    *common.NodeInfo
	Success bool
	Latency time.Duration
	Err     error
}

type PingDoneMsg struct {
	Results []PingResult
}

// pingDial 可注入的 TCP 拨号器(测试替换)
var pingDial = func(addr string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("tcp", addr, timeout)
}

func runPing(nodes []*common.NodeInfo, timeout time.Duration, dial func(string, time.Duration) (net.Conn, error)) []PingResult {
	results := make([]PingResult, 0, len(nodes))
	for _, n := range nodes {
		addr := net.JoinHostPort(n.Address, strconv.Itoa(n.Port))
		start := time.Now()
		conn, err := dial(addr, timeout)
		latency := time.Since(start)
		r := PingResult{Node: n, Latency: latency}
		if err == nil {
			conn.Close()
			r.Success = true
		} else {
			r.Err = err
		}
		results = append(results, r)
	}
	return results
}

type PingModel struct {
	nodes   []*common.NodeInfo
	results []PingResult
	loading bool
}

func NewPingModel(nodes []*common.NodeInfo) *PingModel {
	return &PingModel{nodes: nodes, loading: true}
}

func (m *PingModel) Start() tea.Cmd {
	return func() tea.Msg {
		results := runPing(m.nodes, pingTimeout, pingDial)
		return PingDoneMsg{Results: results}
	}
}
