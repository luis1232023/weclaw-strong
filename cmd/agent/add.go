package agent

import (
	"fmt"

	"github.com/fastclaw-ai/weclaw/config"
	"github.com/spf13/cobra"
)

var (
	addTypeFlag    string
	addCommandFlag string
	addModelFlag   string
	addAliasesFlag []string
	addCwdFlag     string
)

var addCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a new agent",
	Long: `Add a new agent to the configuration.

Examples:
  weclaw agent add claude --type acp --command /usr/local/bin/claude
  weclaw agent add gpt4 --type http --command https://api.openai.com/v1/chat/completions --model gpt-4`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		name := args[0]
		if _, exists := cfg.Agents[name]; exists {
			return fmt.Errorf("agent %q already exists, use 'remove' first or choose a different name", name)
		}

		cfg.Agents[name] = config.AgentConfig{
			Type:    addTypeFlag,
			Command: addCommandFlag,
			Model:   addModelFlag,
			Aliases: addAliasesFlag,
			Cwd:     addCwdFlag,
		}

		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		fmt.Printf("Agent %q added (type=%s). Restart weclaw to apply.\n", name, addTypeFlag)
		return nil
	},
}

func init() {
	addCmd.Flags().StringVar(&addTypeFlag, "type", "", "Agent type: acp, cli, http, serve (required)")
	addCmd.Flags().StringVar(&addCommandFlag, "command", "", "Binary path or endpoint URL (required)")
	addCmd.Flags().StringVar(&addModelFlag, "model", "", "Model name")
	addCmd.Flags().StringSliceVar(&addAliasesFlag, "alias", nil, "Aliases (can specify multiple)")
	addCmd.Flags().StringVar(&addCwdFlag, "cwd", "", "Working directory")
	addCmd.MarkFlagRequired("type")
	addCmd.MarkFlagRequired("command")
}
