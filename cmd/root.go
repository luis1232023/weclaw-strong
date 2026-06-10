package cmd

import (
	"fmt"
	"os"

	"github.com/fastclaw-ai/weclaw/cmd/agent"
	"github.com/fastclaw-ai/weclaw/cmd/configcmd"
	"github.com/fastclaw-ai/weclaw/cmd/session"
	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags.
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:     "weclaw",
	Short:   "WeChat AI agent bridge",
	Long:    "weclaw bridges WeChat messages to AI agents via the iLink API.",
	Version: Version,
	RunE:    runStart, // default command is start
}

func init() {
	rootCmd.AddCommand(agent.AgentCmd)
	rootCmd.AddCommand(session.SessionCmd)
	rootCmd.AddCommand(configcmd.ConfigCmd)
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
