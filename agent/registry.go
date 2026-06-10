package agent

import (
	"context"
	"fmt"
	"log"

	"github.com/fastclaw-ai/weclaw/config"
)

// Builder is a function that creates an agent from config.
type Builder func(ctx context.Context, cfg config.AgentConfig, name string) (Agent, error)

var builders = map[string]Builder{}

// RegisterBuilder registers an agent builder for the given type.
func RegisterBuilder(typ string, b Builder) {
	builders[typ] = b
}

// Build creates an agent using the registered builder for the config type.
func Build(ctx context.Context, cfg config.AgentConfig, name string) (Agent, error) {
	b, ok := builders[cfg.Type]
	if !ok {
		return nil, fmt.Errorf("unknown agent type %q", cfg.Type)
	}
	return b(ctx, cfg, name)
}

// init registers all built-in agent builders.
func init() {
	RegisterBuilder("acp", buildACPAgent)
	RegisterBuilder("cli", buildCLIAgent)
	RegisterBuilder("http", buildHTTPAgent)
	RegisterBuilder("serve", buildServeAgent)
}

func buildACPAgent(ctx context.Context, cfg config.AgentConfig, name string) (Agent, error) {
	ag := NewACPAgent(ACPAgentConfig{
		Command:      cfg.Command,
		Args:         cfg.Args,
		Cwd:          cfg.Cwd,
		Env:          cfg.Env,
		Model:        cfg.Model,
		SystemPrompt: cfg.SystemPrompt,
	})
	if err := ag.Start(ctx); err != nil {
		return nil, fmt.Errorf("start ACP agent: %w", err)
	}
	log.Printf("[agent] started ACP agent: %s (command=%s, model=%s)", name, cfg.Command, cfg.Model)
	return ag, nil
}

func buildCLIAgent(ctx context.Context, cfg config.AgentConfig, name string) (Agent, error) {
	ag := NewCLIAgent(CLIAgentConfig{
		Name:         name,
		Command:      cfg.Command,
		Args:         cfg.Args,
		Cwd:          cfg.Cwd,
		Env:          cfg.Env,
		Model:        cfg.Model,
		SystemPrompt: cfg.SystemPrompt,
	})
	log.Printf("[agent] created CLI agent: %s (command=%s, model=%s)", name, cfg.Command, cfg.Model)
	return ag, nil
}

func buildHTTPAgent(ctx context.Context, cfg config.AgentConfig, name string) (Agent, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("HTTP agent requires endpoint")
	}
	ag := NewHTTPAgent(HTTPAgentConfig{
		Endpoint:     cfg.Endpoint,
		APIKey:       cfg.APIKey,
		Headers:      cfg.Headers,
		Model:        cfg.Model,
		SystemPrompt: cfg.SystemPrompt,
		MaxHistory:   cfg.MaxHistory,
	})
	log.Printf("[agent] created HTTP agent: %s (endpoint=%s, model=%s)", name, cfg.Endpoint, cfg.Model)
	return ag, nil
}

func buildServeAgent(ctx context.Context, cfg config.AgentConfig, name string) (Agent, error) {
	if cfg.Command == "" {
		return nil, fmt.Errorf("serve agent requires command")
	}
	ag := NewServeAgent(ServeAgentConfig{
		Name:         name,
		Command:      cfg.Command,
		Args:         cfg.Args,
		Host:         cfg.Host,
		Port:         cfg.Port,
		Cwd:          cfg.Cwd,
		Env:          cfg.Env,
		Model:        cfg.Model,
		SystemPrompt: cfg.SystemPrompt,
	})
	if err := ag.Start(ctx); err != nil {
		return nil, fmt.Errorf("start serve agent: %w", err)
	}
	log.Printf("[agent] started serve agent: %s (url=%s, model=%s)", name, ag.Info().Command, cfg.Model)
	return ag, nil
}
