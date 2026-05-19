package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewServeAgent_Defaults(t *testing.T) {
	a := NewServeAgent(ServeAgentConfig{
		Name:    "test",
		Command: "opencode",
	})
	if a.host != "127.0.0.1" {
		t.Errorf("default host = %q, want 127.0.0.1", a.host)
	}
	if a.port != 4096 {
		t.Errorf("default port = %d, want 4096", a.port)
	}
	if a.name != "test" {
		t.Errorf("name = %q, want test", a.name)
	}
	if a.baseURL != "" {
		t.Errorf("baseURL should be empty before Start, got %q", a.baseURL)
	}
}

func TestNewServeAgent_CustomConfig(t *testing.T) {
	a := NewServeAgent(ServeAgentConfig{
		Name:    "my-agent",
		Command: "my-binary",
		Host:    "0.0.0.0",
		Port:    9090,
		Model:   "gpt-4",
		Cwd:     "/workspace",
	})
	if a.host != "0.0.0.0" {
		t.Errorf("host = %q, want 0.0.0.0", a.host)
	}
	if a.port != 9090 {
		t.Errorf("port = %d, want 9090", a.port)
	}
	if a.model != "gpt-4" {
		t.Errorf("model = %q, want gpt-4", a.model)
	}
	if a.cwd != "/workspace" {
		t.Errorf("cwd = %q, want /workspace", a.cwd)
	}
}

func TestServeAgent_Info_BeforeStart(t *testing.T) {
	a := NewServeAgent(ServeAgentConfig{
		Name:    "test-agent",
		Command: "test-cmd",
		Model:   "test-model",
	})
	info := a.Info()
	if info.Name != "test-agent" {
		t.Errorf("Name = %q, want test-agent", info.Name)
	}
	if info.Type != "serve" {
		t.Errorf("Type = %q, want serve", info.Type)
	}
	if info.Model != "test-model" {
		t.Errorf("Model = %q, want test-model", info.Model)
	}
	if info.Command != "" {
		t.Errorf("Command = %q, want empty (not started)", info.Command)
	}
	if info.PID != 0 {
		t.Errorf("PID = %d, want 0 (no process)", info.PID)
	}
}

func TestServeAgent_SetCwd(t *testing.T) {
	a := NewServeAgent(ServeAgentConfig{Name: "test", Command: "test"})
	if a.cwd != "" {
		t.Errorf("initial cwd = %q, want empty", a.cwd)
	}
	a.SetCwd("/some/path")
	if a.cwd != "/some/path" {
		t.Errorf("after SetCwd, cwd = %q, want /some/path", a.cwd)
	}
}

func TestServeAgent_Chat_CreatesSessionOnFirstMessage(t *testing.T) {
	var callLog []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			callLog = append(callLog, "createSession")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{"id": "sess-1"})
		case r.Method == http.MethodPost && len(r.URL.Path) > 9 && r.URL.Path[:9] == "/session/":
			callLog = append(callLog, "sendMessage")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"info": map[string]string{"role": "assistant", "id": "msg-1", "finish": "stop"},
				"parts": []map[string]string{
					{"type": "text", "text": "你好，我是 AI"},
				},
			})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	a := NewServeAgent(ServeAgentConfig{Name: "test", Command: "test"})
	a.baseURL = ts.URL
	a.client = ts.Client()

	reply, err := a.Chat(context.Background(), "conv-1", "hello")
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if reply != "你好，我是 AI" {
		t.Errorf("reply = %q, want '你好，我是 AI'", reply)
	}
	if len(callLog) != 2 {
		t.Fatalf("expected 2 API calls, got %d: %v", len(callLog), callLog)
	}
	if callLog[0] != "createSession" {
		t.Errorf("first call = %q, want createSession", callLog[0])
	}
	if callLog[1] != "sendMessage" {
		t.Errorf("second call = %q, want sendMessage", callLog[1])
	}
}

func TestServeAgent_Chat_ReusesExistingSession(t *testing.T) {
	var sendMsgCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && len(r.URL.Path) > 9 && r.URL.Path[:9] == "/session/" {
			sendMsgCount++
			json.NewEncoder(w).Encode(map[string]interface{}{
				"info": map[string]string{"role": "assistant", "id": "msg-2", "finish": "stop"},
				"parts": []map[string]string{
					{"type": "text", "text": "ok"},
				},
			})
			return
		}
		t.Errorf("unexpected request: %s %s (expected only sendMessage)", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	a := NewServeAgent(ServeAgentConfig{Name: "test", Command: "test"})
	a.baseURL = ts.URL
	a.client = ts.Client()
	a.sessions["conv-1"] = "existing-session"

	reply, err := a.Chat(context.Background(), "conv-1", "hello again")
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if reply != "ok" {
		t.Errorf("reply = %q, want ok", reply)
	}
	if sendMsgCount != 1 {
		t.Errorf("expected 1 sendMessage, got %d", sendMsgCount)
	}
}

func TestServeAgent_Chat_ErrorPropagation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer ts.Close()

	a := NewServeAgent(ServeAgentConfig{Name: "test", Command: "test"})
	a.baseURL = ts.URL
	a.client = ts.Client()

	_, err := a.Chat(context.Background(), "conv-1", "hello")
	if err == nil {
		t.Fatal("expected error from server, got nil")
	}
}

func TestServeAgent_Chat_EmptySessionID(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": ""})
	}))
	defer ts.Close()

	a := NewServeAgent(ServeAgentConfig{Name: "test", Command: "test"})
	a.baseURL = ts.URL
	a.client = ts.Client()

	_, err := a.Chat(context.Background(), "conv-1", "hello")
	if err == nil {
		t.Fatal("expected error for empty session ID, got nil")
	}
}

func TestServeAgent_ResetSession_CreatesNewAndDeletesOld(t *testing.T) {
	var created, deleted []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			created = append(created, "sess-new")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{"id": "sess-new"})
		case r.Method == http.MethodDelete && len(r.URL.Path) > 9 && r.URL.Path[:9] == "/session/":
			deleted = append(deleted, r.URL.Path[9:])
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	a := NewServeAgent(ServeAgentConfig{Name: "test", Command: "test"})
	a.baseURL = ts.URL
	a.client = ts.Client()
	a.sessions["conv-1"] = "sess-old"

	sid, err := a.ResetSession(context.Background(), "conv-1")
	if err != nil {
		t.Fatalf("ResetSession failed: %v", err)
	}
	if sid != "sess-new" {
		t.Errorf("returned session = %q, want sess-new", sid)
	}
	if len(deleted) != 1 || deleted[0] != "sess-old" {
		t.Errorf("deleted sessions = %v, want [sess-old]", deleted)
	}
	if len(created) != 1 {
		t.Errorf("created sessions = %v, want [sess-new]", created)
	}
	got := a.sessions["conv-1"]
	if got != "sess-new" {
		t.Errorf("stored session = %q, want sess-new", got)
	}
}

func TestServeAgent_ResetSession_NoExistingSession(t *testing.T) {
	var created int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/session" {
			created++
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{"id": "sess-1"})
			return
		}
		t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	a := NewServeAgent(ServeAgentConfig{Name: "test", Command: "test"})
	a.baseURL = ts.URL
	a.client = ts.Client()

	sid, err := a.ResetSession(context.Background(), "conv-1")
	if err != nil {
		t.Fatalf("ResetSession failed: %v", err)
	}
	if sid != "sess-1" {
		t.Errorf("returned session = %q, want sess-1", sid)
	}
	if created != 1 {
		t.Errorf("expected 1 creation, got %d", created)
	}
}

func TestServeAgent_Stop_ClearsState(t *testing.T) {
	a := NewServeAgent(ServeAgentConfig{Name: "test", Command: "test"})
	a.started = true
	a.sessions["conv-1"] = "session-1"
	a.sessions["conv-2"] = "session-2"

	a.Stop()
	if a.started {
		t.Error("expected started=false after Stop")
	}
	if len(a.sessions) != 0 {
		t.Errorf("expected sessions cleared, got %d entries", len(a.sessions))
	}
}

func TestServeAgent_Stop_NoProcess(t *testing.T) {
	a := NewServeAgent(ServeAgentConfig{Name: "test", Command: "test"})
	a.started = true

	a.Stop()
	if a.started {
		t.Error("expected started=false after Stop")
	}
}

func TestServeAgent_Start_AlreadyStarted(t *testing.T) {
	a := NewServeAgent(ServeAgentConfig{Name: "test", Command: "test"})
	a.started = true

	err := a.Start(context.Background())
	if err == nil {
		t.Fatal("expected error for already started agent, got nil")
	}
}

func TestServeAgent_ResetSession_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("overloaded"))
	}))
	defer ts.Close()

	a := NewServeAgent(ServeAgentConfig{Name: "test", Command: "test"})
	a.baseURL = ts.URL
	a.client = ts.Client()

	_, err := a.ResetSession(context.Background(), "conv-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestServeAgent_ListSessions_WithActiveStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/session":
			if r.URL.RawQuery != "limit=5" {
				t.Errorf("unexpected query: %s", r.URL.RawQuery)
			}
			json.NewEncoder(w).Encode([]map[string]any{
				{
					"id": "sess-active", "title": "Active Session", "agent": "build",
					"model": map[string]string{"id": "gpt-4"},
					"cost": 0.05,
					"tokens": map[string]int{"input": 100, "output": 50},
					"time": map[string]int64{"updated": 1779097321998},
				},
				{
					"id": "sess-done", "title": "Done Session", "agent": "plan",
					"model": map[string]string{"id": "claude-3"},
					"cost": 0.12,
					"tokens": map[string]int{"input": 500, "output": 200},
					"time": map[string]int64{"updated": 1779000000000},
				},
			})
		case "/session/status":
			json.NewEncoder(w).Encode(map[string]any{
				"sess-active": map[string]any{},
			})
		default:
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	a := NewServeAgent(ServeAgentConfig{Name: "test", Command: "test"})
	a.baseURL = ts.URL
	a.client = ts.Client()

	sessions, err := a.ListSessions(context.Background(), 5)
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	if sessions[0].ID != "sess-active" {
		t.Errorf("sessions[0].ID = %q, want sess-active", sessions[0].ID)
	}
	if !sessions[0].IsActive {
		t.Errorf("sessions[0] should be active")
	}
	if sessions[0].Title != "Active Session" {
		t.Errorf("title = %q", sessions[0].Title)
	}
	if sessions[0].Cost != 0.05 {
		t.Errorf("cost = %f", sessions[0].Cost)
	}

	if sessions[1].ID != "sess-done" {
		t.Errorf("sessions[1].ID = %q, want sess-done", sessions[1].ID)
	}
	if sessions[1].IsActive {
		t.Errorf("sessions[1] should not be active")
	}
}

func TestServeAgent_ListSessions_Empty(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]any{})
	}))
	defer ts.Close()

	a := NewServeAgent(ServeAgentConfig{Name: "test", Command: "test"})
	a.baseURL = ts.URL
	a.client = ts.Client()

	sessions, err := a.ListSessions(context.Background(), 5)
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if sessions != nil {
		t.Errorf("expected nil for empty list, got %v", sessions)
	}
}

func TestServeAgent_ListSessions_StatusAPIFails(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		switch r.URL.Path {
		case "/session":
			json.NewEncoder(w).Encode([]map[string]any{
				{
					"id": "sess-1", "title": "Test", "agent": "build",
					"model": map[string]string{"id": "gpt-4"},
					"tokens": map[string]int{"input": 0, "output": 0},
					"time": map[string]int64{"updated": 1779097321998},
				},
			})
		case "/session/status":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer ts.Close()

	a := NewServeAgent(ServeAgentConfig{Name: "test", Command: "test"})
	a.baseURL = ts.URL
	a.client = ts.Client()

	sessions, err := a.ListSessions(context.Background(), 5)
	if err != nil {
		t.Fatalf("ListSessions should not fail on status error, got: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].IsActive {
		t.Errorf("expected IsActive=false when status API fails")
	}
}

func TestServeAgent_ListSessions_APIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer ts.Close()

	a := NewServeAgent(ServeAgentConfig{Name: "test", Command: "test"})
	a.baseURL = ts.URL
	a.client = ts.Client()

	_, err := a.ListSessions(context.Background(), 5)
	if err == nil {
		t.Fatal("expected error from bad gateway, got nil")
	}
}

func TestServeAgent_Start_StderrOnTimeout(t *testing.T) {
	a := NewServeAgent(ServeAgentConfig{
		Name:    "test",
		Command: "cmd",
		Args:    []string{"/c", "echo server error msg 1>&2 & exit /b 1"},
		Port:    19999,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := a.Start(ctx)
	if err == nil {
		t.Fatal("expected error from start with fake command")
	}
	if !strings.Contains(err.Error(), "server error msg") {
		t.Errorf("expected stderr in error, got: %v", err)
	}
}

func TestServeAgent_SessionListerInterface(t *testing.T) {
	a := NewServeAgent(ServeAgentConfig{Name: "test", Command: "test"})
	var _ SessionLister = a
}

func TestServeAgent_Chat_NoSystemPromptInSession(t *testing.T) {
	var gotSystemPrompt bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/session" {
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			_, gotSystemPrompt = body["systemPrompt"]
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{"id": "sess-1"})
			return
		}
		if r.Method == http.MethodPost && len(r.URL.Path) > 9 && r.URL.Path[:9] == "/session/" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"info": map[string]string{"role": "assistant", "id": "msg-3", "finish": "stop"},
				"parts": []map[string]string{
					{"type": "text", "text": "ok"},
				},
			})
			return
		}
		t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	a := NewServeAgent(ServeAgentConfig{
		Name:         "test",
		Command:      "test",
		SystemPrompt: "你是助手",
	})
	a.baseURL = ts.URL
	a.client = ts.Client()

	_, err := a.Chat(context.Background(), "conv-1", "hello")
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if gotSystemPrompt {
		t.Errorf("systemPrompt should not be in createSession body")
	}
}

func TestSendMessage_BodyFormatDiagnostic(t *testing.T) {
	var capturedBody map[string]interface{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		t.Logf("=== CAPTURED REQUEST BODY ===")
		for k, v := range capturedBody {
			t.Logf("  %s: %+v (type: %T)", k, v, v)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"info": map[string]string{"role": "assistant", "id": "msg-d", "finish": "stop"},
			"parts": []map[string]string{{"type": "text", "text": "ok"}},
		})
	}))
	defer ts.Close()

	a := NewServeAgent(ServeAgentConfig{Name: "test", Command: "test", SystemPrompt: "你是一个助手"})
	a.baseURL = ts.URL
	a.client = ts.Client()
	a.sessions["conv-d"] = "sess-d"

	reply, err := a.Chat(context.Background(), "conv-d", "你好")
	t.Logf("reply: %q, err: %v", reply, err)

	if capturedBody == nil {
		t.Fatal("capturedBody is nil, request was not captured")
	}
	parts, ok := capturedBody["parts"]
	if !ok {
		t.Fatal("missing 'parts' in request body")
	}
	t.Logf("parts value: %+v", parts)
}
