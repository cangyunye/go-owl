package async

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	common "github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/control/async"
	"github.com/cangyunye/go-owl/internal/i18n"
)

var (
	asyncPollInterval time.Duration
)

func NewAsyncCmd() *cobra.Command {
	asyncCmd := &cobra.Command{
		Use:   "async",
		Short: i18n.T("async.cmd.short"),
		Long:  i18n.T("async.cmd.long"),
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
		Short: i18n.T("async.list.short"),
		Run: func(cmd *cobra.Command, args []string) {
			manager := async.NewAsyncTaskManager(nil)
			tasks := manager.ListTasks()

			if len(tasks) == 0 {
				fmt.Println(i18n.T("async.list.no_tasks"))
				return
			}

			fmt.Printf("%s %s %s %s\n",
				common.PadRight(i18n.T("async.list.col_task_id"), 36), common.PadRight(i18n.T("async.list.col_node"), 15),
				common.PadRight(i18n.T("async.list.col_status"), 10), common.PadRight(i18n.T("async.list.col_start_time"), 20))
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
		Short: i18n.T("async.status.short"),
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			taskID := args[0]
			manager := async.NewAsyncTaskManager(nil)
			task := manager.GetTask(taskID)

			if task == nil {
				fmt.Printf("%s", i18n.T("async.status.not_found", taskID))
				return
			}

			fmt.Printf("%s", i18n.T("async.status.task_id", task.ID))
			fmt.Printf("%s", i18n.T("async.status.node", task.NodeID))
			fmt.Printf("%s", i18n.T("async.status.command", task.Command))
			fmt.Printf("%s", i18n.T("async.status.status", task.Status))
			fmt.Printf("%s", i18n.T("async.status.start_time", task.StartTime.Format("2006-01-02 15:04:05")))

			if !task.EndTime.IsZero() {
				fmt.Printf("%s", i18n.T("async.status.end_time", task.EndTime.Format("2006-01-02 15:04:05")))
				fmt.Printf("%s", i18n.T("async.status.duration", i18n.F(task.Duration())))
			}

			if task.Pid > 0 {
				fmt.Printf("%s", i18n.T("async.status.pid", i18n.F(task.Pid)))
			}

			if task.ExitCode != 0 {
				fmt.Printf("%s", i18n.T("async.status.exit_code", i18n.F(task.ExitCode)))
			}

			if task.Error != nil {
				fmt.Printf("%s", i18n.T("async.status.error", task.Error))
			}
		},
	}
}

func NewWaitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wait <task-id>",
		Short: i18n.T("async.wait.short"),
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			taskID := args[0]
			manager := async.NewAsyncTaskManager(nil)
			task := manager.GetTask(taskID)

			if task == nil {
				fmt.Printf("%s", i18n.T("async.status.not_found", taskID))
				return
			}

			if task.IsCompleted() {
				fmt.Printf("%s", i18n.T("async.wait.completed", taskID, task.Status))
				return
			}

			fmt.Printf("%s", i18n.T("async.wait.waiting", taskID))

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
						fmt.Println(i18n.T("async.wait.removed"))
						return
					}

					if task.IsCompleted() {
						fmt.Printf("%s", i18n.T("async.wait.done", taskID, task.Status))
						if task.Error != nil {
							fmt.Printf("%s", i18n.T("async.status.error", task.Error))
						}
						return
					}

					fmt.Printf("%s", i18n.T("async.wait.status_running", task.Status))
				}
			}
		},
	}

	cmd.Flags().DurationVar(&asyncPollInterval, "poll-interval", 10*time.Second, i18n.T("async.wait.flag_poll_interval"))
	return cmd
}

func NewCancelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cancel <task-id>",
		Short: i18n.T("async.cancel.short"),
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			taskID := args[0]
			manager := async.NewAsyncTaskManager(nil)

			err := manager.CancelTask(taskID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s", i18n.T("async.cancel.failed", err))
				os.Exit(1)
			}

			fmt.Printf("%s", i18n.T("async.cancel.done", taskID))
		},
	}
}

func NewCleanupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cleanup",
		Short: i18n.T("async.cleanup.short"),
		Run: func(cmd *cobra.Command, args []string) {
			manager := async.NewAsyncTaskManager(nil)
			manager.CleanupCompletedTasks()
			fmt.Println(i18n.T("async.cleanup.done"))
		},
	}
}