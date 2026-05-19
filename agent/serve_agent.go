package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ServeAgent starts a local "opencode serve" subprocess and communicates
// with it via its HTTP REST API (session-based).
type ServeAgent struct {
	name         string
	command      string
	args         []string
	host         string
	port         int
	model        string
	systemPrompt string
	cwd          string
	env          map[string]string

	mu       sync.Mutex
	cmd      *exec.Cmd
	started  bool
	baseURL  string
	sessions map[string]string // conversationID → sessionID
	client   *http.Client
}

// ServeAgentConfig holds configuration for the serve agent.
type ServeAgentConfig struct {
	Name, Command, Host, Model, SystemPrompt, Cwd string
	Args         []string
	Port         int
	Env          map[string]string
}

// NewServeAgent creates a new ServeAgent.
func NewServeAgent(cfg ServeAgentConfig) *ServeAgent {
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Port == 0 {
		cfg.Port = 4096
	}
	return &ServeAgent{
		name:         cfg.Name,
		command:      cfg.Command,
		args:         cfg.Args,
		host:         cfg.Host,
		port:         cfg.Port,
		model:        cfg.Model,
		systemPrompt: cfg.SystemPrompt,
		cwd:          cfg.Cwd,
		env:          cfg.Env,
		sessions:     make(map[string]string),
		client:       &http.Client{Timeout: 120 * time.Second},
	}
}

// Start launches the opencode serve subprocess and waits for it to become ready.
func (a *ServeAgent) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.started {
		return fmt.Errorf("serve agent already started")
	}

	args := make([]string, 0, len(a.args)+2)
	for i := 0; i < len(a.args); i++ {
		if a.args[i] == "--hostname" || a.args[i] == "--port" {
			i++
			continue
		}
		args = append(args, a.args[i])
	}
	args = append(args, "--hostname", a.host, "--port", fmt.Sprintf("%d", a.port))

	cmd := exec.CommandContext(ctx, a.command, args...)
	if a.cwd != "" {
		cmd.Dir = a.cwd
	}
	if len(a.env) > 0 {
		env, err := mergeEnv(nil, a.env)
		if err != nil {
			return fmt.Errorf("merge env: %w", err)
		}
		cmd.Env = env
	}

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start opencode serve: %w", err)
	}
	a.cmd = cmd
	a.started = true
	a.baseURL = fmt.Sprintf("http://%s:%d", a.host, a.port)

	// Monitor process exit in background
	processDone := make(chan struct{})
	go func() {
		cmd.Wait()
		close(processDone)
	}()

	// Wait for the server to become ready by checking TCP connectivity
	dialer := &net.Dialer{Timeout: 2 * time.Second}
	addr := fmt.Sprintf("%s:%d", a.host, a.port)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-processDone:
			a.stopLocked()
			stderrOut := strings.TrimSpace(stderrBuf.String())
			exitCode := cmd.ProcessState.ExitCode()
			if stderrOut != "" {
				return fmt.Errorf("%s exited with code %d before becoming ready: stderr: %s", a.name, exitCode, stderrOut)
			}
			return fmt.Errorf("%s exited with code %d before becoming ready", a.name, exitCode)
		default:
		}

		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err == nil {
			conn.Close()
			log.Printf("[serve-agent] %s ready at %s (pid=%d)", a.name, a.baseURL, cmd.Process.Pid)
			return nil
		}
		select {
		case <-time.After(500 * time.Millisecond):
		case <-processDone:
			a.stopLocked()
			stderrOut := strings.TrimSpace(stderrBuf.String())
			exitCode := cmd.ProcessState.ExitCode()
			if stderrOut != "" {
				return fmt.Errorf("%s exited with code %d before becoming ready: stderr: %s", a.name, exitCode, stderrOut)
			}
			return fmt.Errorf("%s exited with code %d before becoming ready", a.name, exitCode)
		case <-ctx.Done():
			a.stopLocked()
			return ctx.Err()
		}
	}

	a.stopLocked()
	stderrOut := strings.TrimSpace(stderrBuf.String())
	if stderrOut != "" {
		return fmt.Errorf("timeout waiting for %s to become ready at %s: stderr: %s", a.name, a.baseURL, stderrOut)
	}
	return fmt.Errorf("timeout waiting for %s to become ready at %s", a.name, a.baseURL)
}

// Stop kills the serve subprocess.
func (a *ServeAgent) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.stopLocked()
}

func (a *ServeAgent) stopLocked() {
	if a.cmd != nil && a.cmd.Process != nil {
		a.cmd.Process.Kill()
		a.cmd.Wait()
	}
	a.started = false
	a.sessions = make(map[string]string)
}

// Info returns metadata about this agent.
func (a *ServeAgent) Info() AgentInfo {
	pid := 0
	if a.cmd != nil && a.cmd.Process != nil {
		pid = a.cmd.Process.Pid
	}
	return AgentInfo{
		Name:    a.name,
		Type:    "serve",
		Model:   a.model,
		Command: a.baseURL,
		PID:     pid,
	}
}

// SetCwd changes the working directory for subsequent session creation.
func (a *ServeAgent) SetCwd(cwd string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cwd = cwd
}

// ResetSession deletes the existing session for conversationID and creates a new one.
func (a *ServeAgent) ResetSession(ctx context.Context, conversationID string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if oldID, ok := a.sessions[conversationID]; ok {
		a.deleteSession(ctx, oldID)
	}

	sessionID, err := a.createSession(ctx)
	if err != nil {
		return "", err
	}
	a.sessions[conversationID] = sessionID
	return sessionID, nil
}

// Chat sends a message and returns the response.
func (a *ServeAgent) Chat(ctx context.Context, conversationID string, message string) (string, error) {
	a.mu.Lock()

	sessionID, ok := a.sessions[conversationID]
	if !ok {
		var err error
		sessionID, err = a.createSession(ctx)
		if err != nil {
			a.mu.Unlock()
			return "", fmt.Errorf("create session: %w", err)
		}
		a.sessions[conversationID] = sessionID
	}
	a.mu.Unlock()

	reply, err := a.sendMessage(ctx, sessionID, message)
	if err != nil {
		return "", fmt.Errorf("send message: %w", err)
	}
	return reply, nil
}

// ChatAsync sends a message asynchronously (POST /session/:id/prompt_async)
// and returns immediately with a PendingReply for polling.
func (a *ServeAgent) ChatAsync(ctx context.Context, conversationID, message string) (PendingReply, error) {
	a.mu.Lock()
	sessionID, ok := a.sessions[conversationID]
	if !ok {
		var err error
		sessionID, err = a.createSession(ctx)
		if err != nil {
			a.mu.Unlock()
			return PendingReply{}, fmt.Errorf("create session: %w", err)
		}
		a.sessions[conversationID] = sessionID
	}
	a.mu.Unlock()

	if err := a.sendMessageAsync(ctx, sessionID, message); err != nil {
		return PendingReply{}, fmt.Errorf("async send: %w", err)
	}

	return PendingReply{
		SessionID:      sessionID,
		ConversationID: conversationID,
		SentAt:         time.Now(),
	}, nil
}

// PollReplies polls GET /session/:id/message?limit=1 every 500ms and calls
// callback when a finished assistant reply is found. Times out after 2 minutes.
func (a *ServeAgent) PollReplies(ctx context.Context, pr PendingReply, callback ReplyCallback) {
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		deadline := time.After(120 * time.Second)

		for {
			select {
			case <-ticker.C:
				reply, err := a.fetchLatestReply(ctx, pr.SessionID)
				if err != nil {
					log.Printf("[serve-agent] poll error for session %s: %v", pr.SessionID, err)
					continue
				}
				if reply != "" {
					callback(reply)
					return
				}
			case <-deadline:
				callback("请求超时，请重试")
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// createSession calls POST /session and returns the new session ID.
func (a *ServeAgent) createSession(ctx context.Context) (string, error) {
	body := map[string]interface{}{
		"title": "WeChat",
	}

	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal session body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/session", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("create session request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("create session HTTP: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read session response: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("create session HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse session response: %w", err)
	}
	if result.ID == "" {
		return "", fmt.Errorf("empty session ID in response")
	}
	return result.ID, nil
}

// sendMessage calls POST /session/:id/message and returns the assistant's reply.
func (a *ServeAgent) sendMessage(ctx context.Context, sessionID, message string) (string, error) {
	body := map[string]interface{}{
		"parts": []map[string]string{
			{"type": "text", "text": message},
		},
	}
	if a.systemPrompt != "" {
		body["system"] = a.systemPrompt
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal message body: %w", err)
	}

	url := fmt.Sprintf("%s/session/%s/message", a.baseURL, sessionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("create message request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send message HTTP: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read message response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		log.Printf("[serve-agent] sendMessage non-200 for session %s: status=%d body=%s",
			sessionID, resp.StatusCode, string(respBody))
		return "", fmt.Errorf("send message HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"parts"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse message response: %w", err)
	}
	var reply string
	for _, p := range result.Parts {
		if p.Type == "text" {
			reply += p.Text
		}
	}
	if reply == "" {
		return "", fmt.Errorf("no text part in response")
	}
	return reply, nil
}

// sendMessageAsync calls POST /session/:id/prompt_async and returns immediately (no wait).
func (a *ServeAgent) sendMessageAsync(ctx context.Context, sessionID, message string) error {
	body := map[string]interface{}{
		"parts": []map[string]string{
			{"type": "text", "text": message},
		},
	}
	if a.systemPrompt != "" {
		body["system"] = a.systemPrompt
	}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal async message body: %w", err)
	}

	url := fmt.Sprintf("%s/session/%s/prompt_async", a.baseURL, sessionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create async message request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("send async message HTTP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		log.Printf("[serve-agent] sendMessageAsync non-204 for session %s: status=%d body=%s",
			sessionID, resp.StatusCode, string(respBody))
		return fmt.Errorf("send async message HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// fetchLatestReply calls GET /session/:id/message?limit=1 and returns the
// latest assistant's text reply. Returns empty string if no finished reply yet.
func (a *ServeAgent) fetchLatestReply(ctx context.Context, sessionID string) (string, error) {
	url := fmt.Sprintf("%s/session/%s/message?limit=1", a.baseURL, sessionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("fetch messages request: %w", err)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch messages HTTP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("fetch messages HTTP %d: %s", resp.StatusCode, string(body))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read messages response: %w", err)
	}
	log.Printf("[serve-agent] fetchLatestReply raw for session %s: %s", sessionID, string(bodyBytes))

	var messages []struct {
		Info struct {
			Role   string `json:"role"`
			Finish string `json:"finish,omitempty"`
		} `json:"info"`
		Parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"parts"`
	}
	if err := json.Unmarshal(bodyBytes, &messages); err != nil {
		return "", fmt.Errorf("parse messages: %w", err)
	}

	for _, m := range messages {
		if m.Info.Role == "assistant" && m.Info.Finish != "" {
			var reply string
			for _, p := range m.Parts {
				if p.Type == "text" {
					reply += p.Text
				}
			}
			if reply != "" {
				return reply, nil
			}
		}
	}
	return "", nil
}

// deleteSession calls DELETE /session/:id to clean up.
func (a *ServeAgent) deleteSession(ctx context.Context, sessionID string) {
	url := fmt.Sprintf("%s/session/%s", a.baseURL, sessionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

// FetchSession calls GET /session/:id and returns the session details.
func (a *ServeAgent) FetchSession(ctx context.Context, sessionID string) (*Session, error) {
	url := fmt.Sprintf("%s/session/%s", a.baseURL, sessionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch session request: %w", err)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch session HTTP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch session HTTP %d: %s", resp.StatusCode, string(body))
	}

	var session Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, fmt.Errorf("parse session: %w", err)
	}
	return &session, nil
}

// SwitchSession validates a session ID exists and switches the current conversation to use it.
func (a *ServeAgent) SwitchSession(ctx context.Context, conversationID, sessionID string) (*Session, error) {
	session, err := a.FetchSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	a.mu.Lock()
	a.sessions[conversationID] = sessionID
	a.mu.Unlock()

	log.Printf("[serve-agent] switched conversation %s to session %s", conversationID, sessionID)
	return session, nil
}

// --- Session listing ---

type sessionRaw struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Agent  string `json:"agent"`
	Model  struct {
		ID string `json:"id"`
	} `json:"model"`
	Cost   float64 `json:"cost"`
	Tokens struct {
		Input  int `json:"input"`
		Output int `json:"output"`
	} `json:"tokens"`
	Time struct {
		Updated int64 `json:"updated"`
	} `json:"time"`
}

// ListSessions returns the most recent N sessions with active status.
func (a *ServeAgent) ListSessions(ctx context.Context, limit int) ([]SessionInfo, error) {
	sessions, err := a.fetchSessions(ctx, limit)
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, nil
	}

	activeSet, err := a.fetchActiveSessions(ctx)
	if err != nil {
		log.Printf("[serve-agent] failed to fetch active sessions, marking all as done: %v", err)
	}

	result := make([]SessionInfo, 0, len(sessions))
	for _, s := range sessions {
		_, active := activeSet[s.ID]
		result = append(result, SessionInfo{
			ID:        s.ID,
			Title:     s.Title,
			Agent:     s.Agent,
			ModelID:   s.Model.ID,
			Cost:      s.Cost,
			TokenIn:   s.Tokens.Input,
			TokenOut:  s.Tokens.Output,
			UpdatedAt: time.UnixMilli(s.Time.Updated),
			IsActive:  active,
		})
	}
	return result, nil
}

func (a *ServeAgent) fetchSessions(ctx context.Context, limit int) ([]sessionRaw, error) {
	url := fmt.Sprintf("%s/session?limit=%d", a.baseURL, limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create list request: %w", err)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list sessions HTTP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list sessions HTTP %d: %s", resp.StatusCode, string(body))
	}

	var sessions []sessionRaw
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		return nil, fmt.Errorf("parse sessions: %w", err)
	}
	return sessions, nil
}

func (a *ServeAgent) fetchActiveSessions(ctx context.Context) (map[string]struct{}, error) {
	url := a.baseURL + "/session/status"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create status request: %w", err)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("session status HTTP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("session status HTTP %d: %s", resp.StatusCode, string(body))
	}

	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("parse session status: %w", err)
	}

	set := make(map[string]struct{}, len(raw))
	for id := range raw {
		set[id] = struct{}{}
	}
	return set, nil
}
