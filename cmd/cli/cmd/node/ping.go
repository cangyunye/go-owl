package node

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/i18n"
)

var pingAll bool
var pingTimeout time.Duration
var pingCount int

// NewPingCmd 创建 ping 命令
func NewPingCmd() *cobra.Command {
	pingCmd := &cobra.Command{
		Use:     "ping [node_id...]",
		Short:   i18n.T("node.ping.short"),
		Long:    i18n.T("node.ping.long"),
		Example: i18n.T("node.ping.example"),
		Run: func(cmd *cobra.Command, args []string) {
			runPing(args)
		},
	}

	pingCmd.Flags().BoolVarP(&pingAll, "all", "a", false, i18n.T("node.ping.flag_all"))
	pingCmd.Flags().DurationVarP(&pingTimeout, "timeout", "t", 3*time.Second, i18n.T("node.ping.flag_timeout"))
	pingCmd.Flags().IntVarP(&pingCount, "count", "n", 1, i18n.T("node.ping.flag_count"))

	return pingCmd
}

func runPing(nodeIDs []string) {
	store := common.GetNodeStore()

	var nodes []*common.NodeInfo
	var err error

	if pingAll {
		nodes, err = store.List()
		if err != nil {
			fmt.Fprintln(os.Stderr, i18n.T("node.ping.err_list", err))
			os.Exit(1)
		}
	} else if len(nodeIDs) > 0 {
		for _, id := range nodeIDs {
			node, err := store.Get(id)
			if err != nil {
				fmt.Fprintln(os.Stderr, i18n.T("node.ping.warn_not_found", id))
				continue
			}
			nodes = append(nodes, node)
		}
	} else {
		fmt.Fprintln(os.Stderr, i18n.T("node.ping.err_no_target"))
		fmt.Fprintln(os.Stderr, i18n.T("node.ping.err_help"))
		os.Exit(1)
	}

	if len(nodes) == 0 {
		fmt.Println(i18n.T("node.ping.no_nodes"))
		return
	}

	fmt.Print(i18n.T("node.ping.checking", i18n.F(len(nodes)), i18n.F(pingTimeout), i18n.F(pingCount)))

	// 有界并行探测：节点多且部分不可达时，串行探测会被超时线性放大
	//（默认 3s 超时 × count，100 个不可达节点 ≈ 5 分钟）。结果按输入序打印。
	results := pingNodesParallel(nodes, pingTimeout, pingCount)

	reachable := 0
	unreachable := 0

	for _, node := range nodes {
		latencies := results[node.ID]

		if len(latencies) > 0 {
			var total time.Duration
			for _, lat := range latencies {
				total += lat
			}
			avgLatency := total / time.Duration(len(latencies))
			minLatency := latencies[0]
			maxLatency := latencies[0]
			for _, lat := range latencies[1:] {
				if lat < minLatency {
					minLatency = lat
				}
				if lat > maxLatency {
					maxLatency = lat
				}
			}

			if pingCount > 1 {
				fmt.Print(i18n.T("node.ping.reachable", node.ID, node.Address))
				fmt.Print(i18n.T("node.ping.stats",
					i18n.F(len(latencies)), i18n.F(avgLatency.Round(time.Millisecond)),
					i18n.F(minLatency.Round(time.Millisecond)), i18n.F(maxLatency.Round(time.Millisecond))))
			} else {
				fmt.Print(i18n.T("node.ping.reachable_single", node.ID, node.Address, avgLatency.Round(time.Millisecond)))
			}
			reachable++
		} else {
			fmt.Print(i18n.T("node.ping.unreachable", node.ID, node.Address))
			unreachable++
		}
	}

	fmt.Print(i18n.T("node.ping.summary", i18n.F(reachable), i18n.F(unreachable), i18n.F(len(nodes))))
}

// pingNodeTCP 对单个节点做 count 次 TCP 探测，返回成功样本。
func pingNodeTCP(addr string, port int, timeout time.Duration, count int) (res struct {
	reachable bool
	latencies []time.Duration
}) {
	var success bool
	for i := 0; i < count; i++ {
		start := time.Now()
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(addr, strconv.Itoa(port)), timeout)
		latency := time.Since(start)

		if err == nil {
			conn.Close()
			res.latencies = append(res.latencies, latency)
			success = true
		}

		if i < count-1 && success {
			time.Sleep(100 * time.Millisecond)
		}
	}
	res.reachable = len(res.latencies) > 0
	return res
}

// pingNodesParallel 有界并行探测全部节点（默认并发 10），结果按键 node.ID 返回。
func pingNodesParallel(nodes []*common.NodeInfo, timeout time.Duration, count int) map[string][]time.Duration {
	const defaultConcurrency = 10
	conc := defaultConcurrency
	if len(nodes) < conc {
		conc = len(nodes)
	}

	type job struct {
		id      string
		address string
		port    int
	}

	jobs := make([]job, len(nodes))
	for i, n := range nodes {
		addr := n.Address
		if host, _, err := net.SplitHostPort(addr); err == nil {
			addr = host
		}
		jobs[i] = job{id: n.ID, address: addr, port: n.Port}
	}

	results := make(map[string][]time.Duration, len(nodes))
	var mu sync.Mutex
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup

	for _, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(j job) {
			defer wg.Done()
			defer func() { <-sem }()
			r := pingNodeTCP(j.address, j.port, timeout, count)
			mu.Lock()
			if r.reachable {
				results[j.id] = r.latencies
			}
			mu.Unlock()
		}(j)
	}
	wg.Wait()
	return results
}
