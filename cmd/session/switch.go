package session

import (
	"fmt"

	"github.com/spf13/cobra"
)

var switchCmd = &cobra.Command{
	Use:   "switch <session-id>",
	Short: "Switch to a session (requires running weclaw)",
	Long:  "This command requires weclaw to be running. Use /session switch in WeChat instead.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("not implemented: use /session switch in WeChat")
	},
}
