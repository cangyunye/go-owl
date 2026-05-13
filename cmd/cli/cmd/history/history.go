package history

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/cangyunye/go-owl/internal/history"
	"github.com/cangyunye/go-owl/internal/logger"
	"github.com/spf13/cobra"
)

var (
	taskID     string
	nodeID      string
	opType       string
	status        string
	lastDuration   string
	startTimeStr  string
	endTimeStr    string
	limit        int
	offset       int
	format         string
	outputFile  string
	verbose     bool
)

// NewHistoryCmd 创建history子命令
func NewHistoryCmd() *cobra.Command {
	historyCmd := &cobra.Command{
		Use:   "history",
		Short: "查看历史操作记录",
		Long: `查询和导出历史操作记录，支持按任务ID、节点、时间等条件筛选。

示例：
  owl history --task-id task-12345
  owl history --node-id node1 --last 24h
  owl history --op-type command --status completed
  owl history --last 7d --format json --output report.json`,
		Run: runHistory,
	}

	// 查询参数
	historyCmd.Flags().StringVar(&taskID, "task-id", "", "按任务ID筛选")
	historyCmd.Flags().StringVar(&nodeID, "node-id", "", "按节点ID筛选")
	historyCmd.Flags().StringVar(&opType, "op-type", "", "按操作类型筛选 (command, file_transfer, playbook, node_manage)")
	historyCmd.Flags().StringVar(&status, "status", "", "按状态筛选")
	historyCmd.Flags().StringVar(&startTimeStr, "start-time", "", "开始时间 (ISO格式，如 2024-01-01T00:00:00Z)")
	historyCmd.Flags().StringVar(&endTimeStr, "end-time", "", "结束时间 (ISO格式)")
	historyCmd.Flags().StringVar(&lastDuration, "last", "", "相对时间 (如 1h, 24h, 7d)")
	historyCmd.Flags().IntVar(&limit, "limit", 50, "结果数量限制 (默认 50，最大 1000)")
	historyCmd.Flags().IntVar(&offset, "offset", 0, "偏移量 (分页)")
	
	// 输出参数
	historyCmd.Flags().StringVar(&format, "format", "table", "输出格式 (table, json, yaml)")
	historyCmd.Flags().StringVar(&outputFile, "output", "", "输出到文件")
	historyCmd.Flags().BoolVar(&verbose, "verbose", false, "显示详细信息")

	return historyCmd
}

func runHistory(cmd *cobra.Command, args []string) {
	// 初始化日志和历史数据库
	logger.Init(nil)
	_, err := history.NewDB(history.DefaultConfig())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize history DB: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// 解析查询条件
	opts := &history.QueryOptions{
		TaskID:  taskID,
		Limit:   limit,
		Offset:  offset,
	}

	// 解析时间条件
	last, err := parseDuration(lastDuration)
	if err == nil && last > 0 {
		opts.StartTime = time.Now().Add(-last)
	}

	if startTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
			opts.StartTime = t
		}
	}
	if endTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
			opts.EndTime = t
		}
	}

	// 执行查询
	records, err := history.Query(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Query failed: %v\n", err)
		os.Exit(1)
	}

	// 输出结果
	w := cmd.OutOrStdout()
	if outputFile != "" {
		f, err := os.Create(outputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create output file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		w = f
	}

	switch format {
	case "json":
		json.NewEncoder(w).Encode(records)
	case "table":
		printTable(w, records)
	default:
		printTable(w, records)
	}
}

func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	var dur time.Duration
	var err error
	suffix := s[len(s)-1]
	switch suffix {
	case 'h', 'H':
		hours, err := strconv.ParseFloat(s[:len(s)-1], 64)
		if err == nil {
			dur = time.Duration(hours) * time.Hour
		}
	case 'd', 'D':
		days, err := strconv.ParseFloat(s[:len(s)-1], 64)
		if err == nil {
			dur = time.Duration(days) * 24 * time.Hour
		}
	default:
		dur, err = time.ParseDuration(s)
	}
	return dur, err
}

func printTable(w io.Writer, records []*history.Record) {
	writer := tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.FilterHTML)
	defer writer.Flush()

	fmt.Fprintln(writer, "TIME\tTASK ID\tOP TYPE\tTARGETS\tSTATUS")
	fmt.Fprintln(writer, "-----\t-------\t--------\t---\t------")
	for _, r := range records {
		if r.Operation != nil {
			targets := "[" + strings.Join(r.Operation.Targets, ",") + "]"
			fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n",
				r.Operation.CreatedAt.Format("2006-01-02 15:04:05"),
				r.Operation.TaskID,
				r.Operation.OpType,
				targets,
				r.Operation.Status,
			)
		}
	}
}
