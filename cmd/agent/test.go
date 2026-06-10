package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/fastclaw-ai/weclaw/agent"
	"github.com/fastclaw-ai/weclaw/config"
	"github.com/spf13/cobra"
)

var testMessageFlag string

var testCmd = &cobra.Command{
	Use:   "test <name>",
	Short: "Test agent connectivity",
	Long: `Start the agent and send a test message to verify connectivity.

Example:
  weclaw agent test claude --message "Hello, are you working?"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		name := args[0]
		agCfg, ok := cfg.Agents[name]
		if !ok {
			return fmt.Errorf("agent %q not found", name)
		}

		fmt.Printf("Testing agent %q (type=%s)...\n", name, agCfg.Type)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		ag, err := agent.Build(ctx, agCfg, name)
		if err != nil {
			return fmt.Errorf("failed to start agent: %w", err)
		}

		fmt.Printf("Agent started: %s\n", ag.Info())

		reply, err := ag.Chat(ctx, "test-user", testMessageFlag)
		if err != nil {
			return fmt.Errorf("chat failed: %w", err)
		}

		fmt.Printf("\nTest message: %s\n", testMessageFlag)
		fmt.Printf("Reply: %s\n", reply)
		fmt.Println("\nTest passed!")
		return nil
	},
}

func init() {
	testCmd.Flags().StringVar(&testMessageFlag, "message", "ping", "Test message to send")
}
