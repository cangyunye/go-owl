package node

import (
	"fmt"
	"net"
	"os"
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

	fmt.Print(i18n.T("node.ping.checking", len(nodes), pingTimeout, pingCount))

	reachable := 0
	unreachable := 0

	for _, node := range nodes {
		addr := node.Address
		if host, _, err := net.SplitHostPort(addr); err == nil {
			addr = host
		}

		var latencies []time.Duration
		success := false

		for i := 0; i < pingCount; i++ {
			start := time.Now()
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", addr, node.Port), pingTimeout)
			latency := time.Since(start)

			if err == nil {
				conn.Close()
				latencies = append(latencies, latency)
				success = true
			}

			if i < pingCount-1 && success {
				time.Sleep(100 * time.Millisecond)
			}
		}

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
					len(latencies), avgLatency.Round(time.Millisecond),
					minLatency.Round(time.Millisecond), maxLatency.Round(time.Millisecond)))
			} else {
				fmt.Print(i18n.T("node.ping.reachable_single", node.ID, node.Address, avgLatency.Round(time.Millisecond)))
			}
			reachable++
		} else {
			fmt.Print(i18n.T("node.ping.unreachable", node.ID, node.Address))
			unreachable++
		}
	}

	fmt.Print(i18n.T("node.ping.summary", reachable, unreachable, len(nodes)))
}
