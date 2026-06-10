package agent

import (
	"github.com/spf13/cobra"
)

// AgentCmd is the parent command for agent management.
var AgentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage AI agents",
	Long:  "List, add, remove, configure, and test AI agents.",
}

func init() {
	AgentCmd.AddCommand(listCmd)
	AgentCmd.AddCommand(infoCmd)
	AgentCmd.AddCommand(addCmd)
	AgentCmd.AddCommand(removeCmd)
	AgentCmd.AddCommand(setDefaultCmd)
	AgentCmd.AddCommand(testCmd)
}
