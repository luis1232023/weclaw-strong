package messaging

import (
	"context"
	"strings"

	"github.com/fastclaw-ai/weclaw/ilink"
)

// CommandFunc is the signature for command handlers.
type CommandFunc func(ctx context.Context, h *Handler, msg ilink.WeixinMessage, args string) string

// CommandEntry holds a command's metadata and handler.
type CommandEntry struct {
	Name    string
	Handler CommandFunc
}

// builtinCommands is the global registry for built-in commands.
var builtinCommands = map[string]*CommandEntry{}

// RegisterCommand registers a built-in command.
// The name should not include the leading "/".
func RegisterCommand(name string, handler CommandFunc) {
	builtinCommands[name] = &CommandEntry{
		Name:    name,
		Handler: handler,
	}
}

// DispatchBuiltin tries to dispatch a built-in command.
// Returns (reply, handled) where handled is true if the command was found.
func DispatchBuiltin(ctx context.Context, h *Handler, msg ilink.WeixinMessage, trimmed string) (string, bool) {
	name := extractCommandName(trimmed)
	entry, ok := builtinCommands[name]
	if !ok {
		return "", false
	}
	reply := entry.Handler(ctx, h, msg, trimmed)
	return reply, true
}

// extractCommandName extracts the command name from text like "/info" or "/session list".
// Returns empty string if text doesn't start with "/".
func extractCommandName(trimmed string) string {
	if !strings.HasPrefix(trimmed, "/") {
		return ""
	}
	after := trimmed[1:]
	if idx := strings.IndexByte(after, ' '); idx >= 0 {
		return after[:idx]
	}
	return after
}

// init registers all built-in commands.
func init() {
	RegisterCommand("info", cmdInfo)
	RegisterCommand("help", cmdHelp)
	RegisterCommand("new", cmdNew)
	RegisterCommand("clear", cmdNew) // alias for /new
	RegisterCommand("cwd", cmdCwd)
	RegisterCommand("session", cmdSession)
}

// cmdInfo handles /info command.
func cmdInfo(ctx context.Context, h *Handler, msg ilink.WeixinMessage, args string) string {
	return h.buildStatus()
}

// cmdHelp handles /help command.
func cmdHelp(ctx context.Context, h *Handler, msg ilink.WeixinMessage, args string) string {
	return buildHelpText()
}

// cmdNew handles /new and /clear commands.
func cmdNew(ctx context.Context, h *Handler, msg ilink.WeixinMessage, args string) string {
	return h.resetDefaultSession(ctx, msg.FromUserID)
}

// cmdCwd handles /cwd command.
func cmdCwd(ctx context.Context, h *Handler, msg ilink.WeixinMessage, args string) string {
	return h.handleCwd(args)
}

// cmdSession handles /session command.
func cmdSession(ctx context.Context, h *Handler, msg ilink.WeixinMessage, args string) string {
	return h.handleSessionCommand(ctx, msg.FromUserID, args)
}
