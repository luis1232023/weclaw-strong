package configcmd

import (
	"github.com/spf13/cobra"
)

// ConfigCmd is the parent command for configuration management.
var ConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage weclaw configuration",
	Long:  "View and modify weclaw configuration.",
}

func init() {
	ConfigCmd.AddCommand(showCmd)
}
