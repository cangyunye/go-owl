package async

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	common "github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/control/async"
)

var (
	asyncPollInterval time.Duration
)

func NewAsyncCmd() *cobra.Command {
	asyncCmd := &cobra.Command{
		Use:   "async",
		Short: "管理异步任务",
		Long: `管理异步执行的任务，包括查看状态、等待完成、取消任务等操作。

示例：
  owl async list
  owl async status <task-id>
  owl async wait <task-id>
  owl async cancel <task-id>
  owl async cleanup`,
	}

	asyncCmd.AddCommand(NewListCmd())
	asyncCmd.AddCommand(NewStatusCmd())
	asyncCmd.AddCommand(NewWaitCmd())
	asyncCmd.AddCommand(NewCancelCmd())
	asyncCmd.AddCommand(NewCleanupCmd())

	return asyncCmd
}

func NewListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "列出所有异步任务",
		Run: func(cmd *cobra.Command, args []string) {
			manager := async.NewAsyncTaskManager(nil)
			tasks := manager.ListTasks()

			if len(tasks) == 0 {
				fmt.Println("没有正在运行的异步任务")
				return
			}

			fmt.Printf("%s %s %s %s\n",
				common.PadRight("任务ID", 36), common.PadRight("节点", 15),
				common.PadRight("状态", 10), common.PadRight("启动时间", 20))
			fmt.Println(strings.Repeat("-", 86))

			for _, task := range tasks {
				fmt.Printf("%s %s %s %s\n",
					common.PadRight(task.ID, 36),
					common.PadRight(task.NodeID, 15),
					common.PadRight(string(task.Status), 10),
					common.PadRight(task.StartTime.Format("2006-01-02 15:04:05"), 20))
			}
		},
	}
}

func NewStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <task-id>",
		Short: "查看任务状态",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			taskID := args[0]
			manager := async.NewAsyncTaskManager(nil)
			task := manager.GetTask(taskID)

			if task == nil {
				fmt.Printf("未找到任务: %s\n", taskID)
				return
			}

			fmt.Printf("任务 ID: %s\n", task.ID)
			fmt.Printf("节点: %s\n", task.NodeID)
			fmt.Printf("命令: %s\n", task.Command)
			fmt.Printf("状态: %s\n", task.Status)
			fmt.Printf("启动时间: %s\n", task.StartTime.Format("2006-01-02 15:04:05"))

			if !task.EndTime.IsZero() {
				fmt.Printf("结束时间: %s\n", task.EndTime.Format("2006-01-02 15:04:05"))
				fmt.Printf("执行时长: %v\n", task.Duration())
			}

			if task.Pid > 0 {
				fmt.Printf("进程 PID: %d\n", task.Pid)
			}

			if task.ExitCode != 0 {
				fmt.Printf("退出码: %d\n", task.ExitCode)
			}

			if task.Error != nil {
				fmt.Printf("错误: %v\n", task.Error)
			}
		},
	}
}

func NewWaitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wait <task-id>",
		Short: "等待任务完成",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			taskID := args[0]
			manager := async.NewAsyncTaskManager(nil)
			task := manager.GetTask(taskID)

			if task == nil {
				fmt.Printf("未找到任务: %s\n", taskID)
				return
			}

			if task.IsCompleted() {
				fmt.Printf("任务 %s 已完成，状态: %s\n", taskID, task.Status)
				return
			}

			fmt.Printf("等待任务 %s 完成...\n", taskID)

			pollInterval := asyncPollInterval
			if pollInterval == 0 {
				pollInterval = 10 * time.Second
			}

			ticker := time.NewTicker(pollInterval)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					task = manager.GetTask(taskID)
					if task == nil {
						fmt.Println("任务已被移除")
						return
					}

					if task.IsCompleted() {
						fmt.Printf("任务 %s 完成，状态: %s\n", taskID, task.Status)
						if task.Error != nil {
							fmt.Printf("错误: %v\n", task.Error)
						}
						return
					}

					fmt.Printf("状态: %s (运行中...)\n", task.Status)
				}
			}
		},
	}

	cmd.Flags().DurationVar(&asyncPollInterval, "poll-interval", 10*time.Second, "轮询间隔")
	return cmd
}

func NewCancelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cancel <task-id>",
		Short: "取消任务",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			taskID := args[0]
			manager := async.NewAsyncTaskManager(nil)

			err := manager.CancelTask(taskID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "取消任务失败: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("任务 %s 已取消\n", taskID)
		},
	}
}

func NewCleanupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cleanup",
		Short: "清理已完成的任务",
		Run: func(cmd *cobra.Command, args []string) {
			manager := async.NewAsyncTaskManager(nil)
			manager.CleanupCompletedTasks()
			fmt.Println("已清理已完成的任务")
		},
	}
}