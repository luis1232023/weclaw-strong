package agent

import (
	"fmt"

	"github.com/fastclaw-ai/weclaw/config"
	"github.com/spf13/cobra"
)

var removeCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove an agent",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		name := args[0]
		if _, ok := cfg.Agents[name]; !ok {
			return fmt.Errorf("agent %q not found", name)
		}
		delete(cfg.Agents, name)
		if cfg.DefaultAgent == name {
			cfg.DefaultAgent = ""
			fmt.Printf("Warning: removed agent was the default, no default agent set now\n")
		}
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		fmt.Printf("Agent %q removed.\n", name)
		return nil
	},
}
