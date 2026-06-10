package agent

import (
	"encoding/json"
	"fmt"

	"github.com/fastclaw-ai/weclaw/config"
	"github.com/spf13/cobra"
)

var infoCmd = &cobra.Command{
	Use:   "info <name>",
	Short: "Show details of an agent",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		ag, ok := cfg.Agents[args[0]]
		if !ok {
			return fmt.Errorf("agent %q not found", args[0])
		}

		// Pretty print as JSON
		data, err := json.MarshalIndent(ag, "", "  ")
		if err != nil {
			return err
		}

		fmt.Printf("Agent: %s\n", args[0])
		if args[0] == cfg.DefaultAgent {
			fmt.Println("Status: default")
		}
		fmt.Println("Config:")
		fmt.Println(string(data))
		return nil
	},
}
