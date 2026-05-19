package messaging

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fastclaw-ai/weclaw/agent"
)

func newTestHandler() *Handler {
	return &Handler{agents: make(map[string]agent.Agent)}
}

func TestParseCommand_NoPrefix(t *testing.T) {
	h := newTestHandler()
	names, msg := h.parseCommand("hello world")
	if len(names) != 0 {
		t.Errorf("expected nil names, got %v", names)
	}
	if msg != "hello world" {
		t.Errorf("expected full text, got %q", msg)
	}
}

func TestParseCommand_SlashWithAgent(t *testing.T) {
	h := newTestHandler()
	names, msg := h.parseCommand("/claude explain this code")
	if len(names) != 1 || names[0] != "claude" {
		t.Errorf("expected [claude], got %v", names)
	}
	if msg != "explain this code" {
		t.Errorf("expected 'explain this code', got %q", msg)
	}
}

func TestParseCommand_AtPrefix(t *testing.T) {
	h := newTestHandler()
	names, msg := h.parseCommand("@claude explain this code")
	if len(names) != 1 || names[0] != "claude" {
		t.Errorf("expected [claude], got %v", names)
	}
	if msg != "explain this code" {
		t.Errorf("expected 'explain this code', got %q", msg)
	}
}

func TestParseCommand_MultiAgent(t *testing.T) {
	h := newTestHandler()
	names, msg := h.parseCommand("@cc @cx hello")
	if len(names) != 2 || names[0] != "claude" || names[1] != "codex" {
		t.Errorf("expected [claude codex], got %v", names)
	}
	if msg != "hello" {
		t.Errorf("expected 'hello', got %q", msg)
	}
}

func TestParseCommand_MultiAgentDedup(t *testing.T) {
	h := newTestHandler()
	names, msg := h.parseCommand("@cc @cc hello")
	if len(names) != 1 || names[0] != "claude" {
		t.Errorf("expected [claude] (deduped), got %v", names)
	}
	if msg != "hello" {
		t.Errorf("expected 'hello', got %q", msg)
	}
}

func TestParseCommand_SwitchOnly(t *testing.T) {
	h := newTestHandler()
	names, msg := h.parseCommand("/claude")
	if len(names) != 1 || names[0] != "claude" {
		t.Errorf("expected [claude], got %v", names)
	}
	if msg != "" {
		t.Errorf("expected empty message, got %q", msg)
	}
}

func TestParseCommand_Alias(t *testing.T) {
	h := newTestHandler()
	names, msg := h.parseCommand("/cc write a function")
	if len(names) != 1 || names[0] != "claude" {
		t.Errorf("expected [claude] from /cc alias, got %v", names)
	}
	if msg != "write a function" {
		t.Errorf("expected 'write a function', got %q", msg)
	}
}

func TestParseCommand_CustomAlias(t *testing.T) {
	h := newTestHandler()
	h.customAliases = map[string]string{"ai": "claude", "c": "claude"}
	names, msg := h.parseCommand("/ai hello")
	if len(names) != 1 || names[0] != "claude" {
		t.Errorf("expected [claude] from custom alias, got %v", names)
	}
	if msg != "hello" {
		t.Errorf("expected 'hello', got %q", msg)
	}
}

func TestResolveAlias(t *testing.T) {
	h := newTestHandler()
	tests := map[string]string{
		"cc":  "claude",
		"cx":  "codex",
		"oc":  "openclaw",
		"cs":  "cursor",
		"km":  "kimi",
		"gm":  "gemini",
		"ocd": "opencode",
		"ocs": "opencode-serve",
		"pi":  "pi",
		"cp":  "copilot",
		"dr":  "droid",
		"if":  "iflow",
		"kr":  "kiro",
		"qw":  "qwen",
	}
	for alias, want := range tests {
		got := h.resolveAlias(alias)
		if got != want {
			t.Errorf("resolveAlias(%q) = %q, want %q", alias, got, want)
		}
	}
	if got := h.resolveAlias("unknown"); got != "unknown" {
		t.Errorf("resolveAlias(unknown) = %q, want %q", got, "unknown")
	}
	h.customAliases = map[string]string{"cc": "custom-claude"}
	if got := h.resolveAlias("cc"); got != "custom-claude" {
		t.Errorf("resolveAlias(cc) with custom = %q, want custom-claude", got)
	}
}

func TestBuildHelpText(t *testing.T) {
	text := buildHelpText()
	if text == "" {
		t.Error("help text is empty")
	}
	if !strings.Contains(text, "/info") {
		t.Error("help text should mention /info")
	}
	if !strings.Contains(text, "/help") {
		t.Error("help text should mention /help")
	}
	if !strings.Contains(text, "/session") {
		t.Error("help text should mention /session")
	}
}

// --- Mock agent for session command tests ---

type mockSessionAgent struct {
	name     string
	sessions []agent.SessionInfo
	listErr  error
}

func (m *mockSessionAgent) Chat(_ context.Context, _, _ string) (string, error) {
	return "", nil
}
func (m *mockSessionAgent) ResetSession(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (m *mockSessionAgent) Info() agent.AgentInfo {
	return agent.AgentInfo{Name: m.name, Type: "serve"}
}
func (m *mockSessionAgent) SetCwd(_ string) {}
func (m *mockSessionAgent) ListSessions(_ context.Context, _ int) ([]agent.SessionInfo, error) {
	return m.sessions, m.listErr
}

func TestHandleSessionCommand_NoAgent(t *testing.T) {
	h := newTestHandler()
	reply := h.handleSessionCommand(nil, "user-1", "/session list")
	if !strings.Contains(reply, "No agent") {
		t.Errorf("unexpected reply: %q", reply)
	}
}

func TestHandleSessionCommand_UnsupportedAgent(t *testing.T) {
	h := newTestHandler()
	h.defaultName = "dumb"
	h.agents["dumb"] = &dumbAgent{}
	reply := h.handleSessionCommand(nil, "user-1", "/session list")
	if !strings.Contains(reply, "not support") {
		t.Errorf("unexpected reply: %q", reply)
	}
}

type dumbAgent struct{}

func (d *dumbAgent) Chat(_ context.Context, _, _ string) (string, error) { return "", nil }
func (d *dumbAgent) ResetSession(_ context.Context, _ string) (string, error) { return "", nil }
func (d *dumbAgent) Info() agent.AgentInfo { return agent.AgentInfo{Name: "dumb"} }
func (d *dumbAgent) SetCwd(_ string) {}

func TestHandleSessionCommand_ListSessions(t *testing.T) {
	h := newTestHandler()
	mock := &mockSessionAgent{
		name: "opencode-serve",
		sessions: []agent.SessionInfo{
			{ID: "sess-1", Title: "Test One", IsActive: true},
			{ID: "sess-2", Title: "Test Two", IsActive: false, Cost: 0.05},
		},
	}
	h.defaultName = "opencode-serve"
	h.agents["opencode-serve"] = mock

	reply := h.handleSessionCommand(nil, "user-1", "/session list")
	if !strings.Contains(reply, "会话列表") {
		t.Errorf("should contain header, got: %s", reply)
	}
	if !strings.Contains(reply, "Test One") {
		t.Errorf("should contain session title, got: %s", reply)
	}
	if !strings.Contains(reply, "进行中") {
		t.Errorf("should indicate active session, got: %s", reply)
	}
	if !strings.Contains(reply, "完成") {
		t.Errorf("should indicate completed session, got: %s", reply)
	}
}

func TestHandleSessionCommand_EmptySessions(t *testing.T) {
	h := newTestHandler()
	mock := &mockSessionAgent{name: "test", sessions: []agent.SessionInfo{}}
	h.defaultName = "test"
	h.agents["test"] = mock

	reply := h.handleSessionCommand(nil, "user-1", "/session list")
	if !strings.Contains(reply, "No active") {
		t.Errorf("unexpected reply: %q", reply)
	}
}

func TestHandleSessionCommand_APIError(t *testing.T) {
	h := newTestHandler()
	mock := &mockSessionAgent{name: "test", listErr: errors.New("API error")}
	h.defaultName = "test"
	h.agents["test"] = mock

	reply := h.handleSessionCommand(nil, "user-1", "/session list")
	if !strings.Contains(reply, "Failed") {
		t.Errorf("expected error message, got: %s", reply)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "刚刚"},
		{2 * time.Minute, "2 分钟前"},
		{30 * time.Minute, "30 分钟前"},
		{2 * time.Hour, "2 小时前"},
		{23 * time.Hour, "23 小时前"},
		{25 * time.Hour, "1 天前"},
		{72 * time.Hour, "3 天前"},
	}
	for _, tc := range tests {
		got := formatDuration(tc.d)
		if got != tc.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestTruncateMiddle(t *testing.T) {
	tests := []struct {
		s      string
		maxLen int
		want   string
	}{
		{"short", 20, "short"},
		{"12345678901234567890", 20, "12345678901234567890"},
		{"123456789012345678901", 20, "1234567890...2345678901"},
		{"abcdefghijklmnopqrstu", 10, "abcde...qrstu"},
	}
	for _, tc := range tests {
		got := truncateMiddle(tc.s, tc.maxLen)
		if got != tc.want {
			t.Errorf("truncateMiddle(%q, %d) = %q, want %q", tc.s, tc.maxLen, got, tc.want)
		}
	}
}
