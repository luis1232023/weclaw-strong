package agent

import (
	"fmt"

	"github.com/fastclaw-ai/weclaw/config"
	"github.com/spf13/cobra"
)

var setDefaultCmd = &cobra.Command{
	Use:   "set-default <name>",
	Short: "Set the default agent",
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
		cfg.DefaultAgent = name
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		fmt.Printf("Default agent set to %q.\n", name)
		return nil
	},
}
