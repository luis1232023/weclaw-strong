package agent

import (
	"fmt"

	"github.com/fastclaw-ai/weclaw/config"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured agents",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		if len(cfg.Agents) == 0 {
			fmt.Println("No agents configured.")
			return nil
		}
		fmt.Printf("%-16s %-8s %-12s %s\n", "NAME", "TYPE", "MODEL", "COMMAND")
		fmt.Println("------------------------------------------------")
		for name, ag := range cfg.Agents {
			marker := ""
			if name == cfg.DefaultAgent {
				marker = " *"
			}
			command := ag.Command
			if ag.Type == "http" {
				command = ag.Endpoint
			}
			fmt.Printf("%-16s %-8s %-12s %s%s\n", name, ag.Type, ag.Model, command, marker)
		}
		fmt.Println("\n  * = default agent")
		return nil
	},
}
