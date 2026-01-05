package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "tasks",
	Short: "Lightweight task management for agents",
	Long:  `A CLI for managing tasks, epics, and dependencies. Designed for AI agents to track work across sessions.`,
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize the tasks database",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: implement
		fmt.Println("tasks init - not yet implemented")
		return nil
	},
}

var addCmd = &cobra.Command{
	Use:   "add <title>",
	Short: "Create a new task",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: implement
		fmt.Printf("tasks add %q - not yet implemented\n", args[0])
		return nil
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: implement
		fmt.Println("tasks list - not yet implemented")
		return nil
	},
}

var readyCmd = &cobra.Command{
	Use:   "ready",
	Short: "Show tasks ready for work (unblocked)",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: implement
		fmt.Println("tasks ready - not yet implemented")
		return nil
	},
}

var showCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show task details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: implement
		fmt.Printf("tasks show %s - not yet implemented\n", args[0])
		return nil
	},
}

var startCmd = &cobra.Command{
	Use:   "start <id>",
	Short: "Start working on a task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: implement
		fmt.Printf("tasks start %s - not yet implemented\n", args[0])
		return nil
	},
}

var doneCmd = &cobra.Command{
	Use:   "done [id]",
	Short: "Mark a task as done",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: implement
		fmt.Println("tasks done - not yet implemented")
		return nil
	},
}

var blockCmd = &cobra.Command{
	Use:   "block <reason>",
	Short: "Mark current task as blocked",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: implement
		fmt.Println("tasks block - not yet implemented")
		return nil
	},
}

var logCmd = &cobra.Command{
	Use:   "log <message>",
	Short: "Add a log entry to current task",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: implement
		fmt.Println("tasks log - not yet implemented")
		return nil
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show project status overview",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: implement
		fmt.Println("tasks status - not yet implemented")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(readyCmd)
	rootCmd.AddCommand(showCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(doneCmd)
	rootCmd.AddCommand(blockCmd)
	rootCmd.AddCommand(logCmd)
	rootCmd.AddCommand(statusCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
