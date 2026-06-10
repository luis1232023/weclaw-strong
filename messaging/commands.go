package messaging

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fastclaw-ai/weclaw/agent"
)

// switchDefault switches the default agent. Starts it on demand if needed.
// The change is persisted to config file.
func (h *Handler) switchDefault(ctx context.Context, name string) string {
	ag, err := h.getAgent(ctx, name)
	if err != nil {
		log.Printf("[handler] failed to switch default to %q: %v", name, err)
		return fmt.Sprintf("Failed to switch to %q: %v", name, err)
	}

	h.mu.Lock()
	old := h.defaultName
	h.defaultName = name
	h.agents[name] = ag
	h.mu.Unlock()

	// Persist to config file
	if h.saveDefault != nil {
		if err := h.saveDefault(name); err != nil {
			log.Printf("[handler] failed to save default agent to config: %v", err)
		} else {
			log.Printf("[handler] saved default agent %q to config", name)
		}
	}

	info := ag.Info()
	log.Printf("[handler] switched default agent: %s -> %s (%s)", old, name, info)
	return fmt.Sprintf("switch to %s", name)
}

// resetDefaultSession resets the session for the given userID on the default agent.
func (h *Handler) resetDefaultSession(ctx context.Context, userID string) string {
	ag, _ := h.getDefaultAgent()
	if ag == nil {
		return "No agent running."
	}
	name := ag.Info().Name
	sessionID, err := ag.ResetSession(ctx, userID)
	if err != nil {
		log.Printf("[handler] reset session failed for %s: %v", userID, err)
		return fmt.Sprintf("Failed to reset session: %v", err)
	}
	if sessionID != "" {
		return fmt.Sprintf("已创建新的%s会话\n%s", name, sessionID)
	}
	return fmt.Sprintf("已创建新的%s会话", name)
}

// handleCwd handles the /cwd command. It updates the working directory for all running agents.
func (h *Handler) handleCwd(trimmed string) string {
	arg := strings.TrimSpace(strings.TrimPrefix(trimmed, "/cwd"))
	if arg == "" {
		// No path provided — show current cwd of default agent
		ag, _ := h.getDefaultAgent()
		if ag == nil {
			return "No agent running."
		}
		info := ag.Info()
		return fmt.Sprintf("cwd: (check agent config)\nagent: %s", info.Name)
	}

	// Expand ~ to home directory
	if arg == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			arg = home
		}
	} else if strings.HasPrefix(arg, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			arg = filepath.Join(home, arg[2:])
		}
	}

	// Resolve to absolute path
	absPath, err := filepath.Abs(arg)
	if err != nil {
		return fmt.Sprintf("Invalid path: %v", err)
	}

	// Verify directory exists
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Sprintf("Path not found: %s", absPath)
	}
	if !info.IsDir() {
		return fmt.Sprintf("Not a directory: %s", absPath)
	}

	// Update cwd on all running agents
	h.mu.RLock()
	agents := make(map[string]agent.Agent, len(h.agents))
	for name, ag := range h.agents {
		agents[name] = ag
	}
	h.mu.RUnlock()

	for name, ag := range agents {
		ag.SetCwd(absPath)
		log.Printf("[handler] updated cwd for agent %s: %s", name, absPath)
	}

	h.mu.Lock()
	for name := range agents {
		h.agentWorkDirs[name] = absPath
	}
	h.mu.Unlock()

	return fmt.Sprintf("cwd: %s", absPath)
}

// handleSessionCommand processes /session, /session list [N], and /session switch <id> commands.
func (h *Handler) handleSessionCommand(ctx context.Context, userID, trimmed string) string {
	parts := strings.Fields(trimmed)

	if len(parts) >= 3 && parts[1] == "switch" {
		return h.switchUserSession(ctx, userID, parts[2])
	}

	limit := 5
	if len(parts) >= 3 && parts[1] == "list" {
		if n, err := fmt.Sscanf(parts[2], "%d", &limit); err == nil && n == 1 && limit > 0 {
			if limit > 20 {
				limit = 20
			}
		}
	}

	ag, _ := h.getDefaultAgent()
	if ag == nil {
		return "No agent running."
	}

	lister, ok := ag.(agent.SessionLister)
	if !ok {
		return "Current agent does not support session listing."
	}

	sessions, err := lister.ListSessions(ctx, limit)
	if err != nil {
		return fmt.Sprintf("Failed to list sessions: %v", err)
	}

	if len(sessions) == 0 {
		return "No active sessions."
	}

	info := ag.Info()
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s 会话列表（最近 %d 条）\n\n", info.Name, len(sessions)))
	for i, s := range sessions {
		status := "完成"
		if s.IsActive {
			status = "进行中"
		}
		ago := formatDuration(time.Since(s.UpdatedAt))
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, s.Title))
		b.WriteString(fmt.Sprintf("   ID: %s\n", truncateMiddle(s.ID, 20)))
		b.WriteString(fmt.Sprintf("   状态: %s\n", status))
		b.WriteString(fmt.Sprintf("   最后活跃: %s\n", ago))
		if s.Cost > 0 {
			b.WriteString(fmt.Sprintf("   成本: $%.4f\n", s.Cost))
		}
		if i < len(sessions)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// switchUserSession switches the current user's session to the given session ID.
func (h *Handler) switchUserSession(ctx context.Context, userID, sessionID string) string {
	ag, _ := h.getDefaultAgent()
	if ag == nil {
		return "No agent running."
	}

	sw, ok := ag.(agent.SessionSwitch)
	if !ok {
		return "Current agent does not support session switching."
	}

	session, err := sw.SwitchSession(ctx, userID, sessionID)
	if err != nil {
		return fmt.Sprintf("切换失败: %v", err)
	}

	title := session.Title
	if title == "" {
		title = "(无标题)"
	}
	return fmt.Sprintf("切换成功\n%s\n%s", title, session.ID)
}

// formatDuration formats a duration into a human-readable relative time string.
func formatDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟前", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d 小时前", int(d.Hours()))
	default:
		return fmt.Sprintf("%d 天前", int(d.Hours()/24))
	}
}

// truncateMiddle truncates a string to the given length, keeping the start and end.
func truncateMiddle(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	half := maxLen / 2
	return s[:half] + "..." + s[len(s)-half:]
}

// buildStatus returns a short status string showing the current default agent.
func (h *Handler) buildStatus() string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.defaultName == "" {
		return "agent: none (echo mode)"
	}

	ag, ok := h.agents[h.defaultName]
	if !ok {
		return fmt.Sprintf("agent: %s (not started)", h.defaultName)
	}

	info := ag.Info()
	return fmt.Sprintf("agent: %s\ntype: %s\nmodel: %s", h.defaultName, info.Type, info.Model)
}

func buildHelpText() string {
	return `Available commands:
@agent or /agent - Switch default agent
@agent msg or /agent msg - Send to a specific agent
@a @b msg - Broadcast to multiple agents
/new or /clear - Start a new session
/cwd /path - Switch workspace directory
/info - Show current agent info
/session list [N] - List recent N sessions (default 5, max 20)
/session switch <id> - Switch to an existing session
/help - Show this help message

Aliases: /cc(claude) /cx(codex) /cs(cursor) /km(kimi) /gm(gemini) /oc(openclaw) /ocd(opencode) /ocs(opencode-serve) /pi(pi) /cp(copilot) /dr(droid) /if(iflow) /kr(kiro) /qw(qwen)`
}
