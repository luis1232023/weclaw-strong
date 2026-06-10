package session

import (
	"fmt"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent sessions (requires running weclaw)",
	Long:  "This command requires weclaw to be running. Use /session list in WeChat instead.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("not implemented: use /session list in WeChat, or query the API directly")
	},
}
