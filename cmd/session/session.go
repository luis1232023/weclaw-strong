package session

import (
	"github.com/spf13/cobra"
)

// SessionCmd is the parent command for session management.
var SessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage agent sessions",
	Long:  "List and switch conversation sessions for agents.",
}

func init() {
	SessionCmd.AddCommand(listCmd)
	SessionCmd.AddCommand(switchCmd)
}
