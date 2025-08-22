package tlq

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name     string
		opts     []Option
		wantHost string
		wantPort int
	}{
		{
			name:     "default configuration",
			opts:     []Option{},
			wantHost: DefaultHost,
			wantPort: DefaultPort,
		},
		{
			name:     "custom host and port",
			opts:     []Option{WithHost("custom.host"), WithPort(8080)},
			wantHost: "custom.host",
			wantPort: 8080,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(tt.opts...)
			if !strings.Contains(client.baseURL, tt.wantHost) {
				t.Errorf("NewClient() baseURL = %v, want host %v", client.baseURL, tt.wantHost)
			}
		})
	}
}

func TestClient_HealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hello" {
			t.Errorf("Expected path /hello, got %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("Expected method GET, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		config:     &Config{MaxRetries: 1, RetryDelay: time.Millisecond},
	}

	ctx := context.Background()
	err := client.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
}

func TestClient_AddMessage(t *testing.T) {
	expectedMessage := &Message{
		ID:         "test-id",
		Body:       "test message",
		State:      "Ready",
		RetryCount: 0,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/add" {
			t.Errorf("Expected path /add, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("Expected method POST, got %s", r.Method)
		}

		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		if payload["body"] != "test message" {
			t.Errorf("Expected body 'test message', got %s", payload["body"])
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expectedMessage)
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		config:     &Config{MaxRetries: 1, RetryDelay: time.Millisecond},
	}

	ctx := context.Background()
	message, err := client.AddMessage(ctx, "test message")
	if err != nil {
		t.Fatalf("AddMessage() error = %v", err)
	}

	if message.ID != expectedMessage.ID {
		t.Errorf("AddMessage() ID = %v, want %v", message.ID, expectedMessage.ID)
	}
	if message.Body != expectedMessage.Body {
		t.Errorf("AddMessage() Body = %v, want %v", message.Body, expectedMessage.Body)
	}
}

func TestClient_AddMessage_ExceedsMaxSize(t *testing.T) {
	client := NewClient()
	ctx := context.Background()

	largeBody := strings.Repeat("a", MaxMessageSize+1)
	_, err := client.AddMessage(ctx, largeBody)

	if err == nil {
		t.Error("AddMessage() expected error for oversized message, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Errorf("AddMessage() error = %v, want error about maximum size", err)
	}
}

func TestClient_GetMessages(t *testing.T) {
	expectedMessages := []*Message{
		{ID: "msg1", Body: "body1", State: "Ready", RetryCount: 0},
		{ID: "msg2", Body: "body2", State: "Ready", RetryCount: 0},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/get" {
			t.Errorf("Expected path /get, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("Expected method POST, got %s", r.Method)
		}

		var payload map[string]int
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		if payload["count"] != 2 {
			t.Errorf("Expected count 2, got %d", payload["count"])
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expectedMessages)
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		config:     &Config{MaxRetries: 1, RetryDelay: time.Millisecond},
	}

	ctx := context.Background()
	messages, err := client.GetMessages(ctx, 2)
	if err != nil {
		t.Fatalf("GetMessages() error = %v", err)
	}

	if len(messages) != 2 {
		t.Errorf("GetMessages() returned %d messages, want 2", len(messages))
	}

	for i, msg := range messages {
		if msg.ID != expectedMessages[i].ID {
			t.Errorf("GetMessages()[%d] ID = %v, want %v", i, msg.ID, expectedMessages[i].ID)
		}
	}
}

func TestClient_GetMessages_InvalidCount(t *testing.T) {
	client := NewClient()
	ctx := context.Background()

	_, err := client.GetMessages(ctx, 0)
	if err == nil {
		t.Error("GetMessages() expected error for count <= 0, got nil")
	}

	_, err = client.GetMessages(ctx, -1)
	if err == nil {
		t.Error("GetMessages() expected error for count <= 0, got nil")
	}
}

func TestClient_DeleteMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/delete" {
			t.Errorf("Expected path /delete, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("Expected method POST, got %s", r.Method)
		}

		var payload map[string][]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		expectedIDs := []string{"msg1", "msg2"}
		if len(payload["ids"]) != len(expectedIDs) {
			t.Errorf("Expected %d IDs, got %d", len(expectedIDs), len(payload["ids"]))
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		config:     &Config{MaxRetries: 1, RetryDelay: time.Millisecond},
	}

	ctx := context.Background()
	err := client.DeleteMessages(ctx, []string{"msg1", "msg2"})
	if err != nil {
		t.Fatalf("DeleteMessages() error = %v", err)
	}
}

func TestClient_DeleteMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string][]string
		json.NewDecoder(r.Body).Decode(&payload)

		if len(payload["ids"]) != 1 || payload["ids"][0] != "msg1" {
			t.Errorf("Expected single ID 'msg1', got %v", payload["ids"])
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		config:     &Config{MaxRetries: 1, RetryDelay: time.Millisecond},
	}

	ctx := context.Background()
	err := client.DeleteMessage(ctx, "msg1")
	if err != nil {
		t.Fatalf("DeleteMessage() error = %v", err)
	}
}

func TestClient_DeleteMessages_EmptyList(t *testing.T) {
	client := NewClient()
	ctx := context.Background()

	err := client.DeleteMessages(ctx, []string{})
	if err == nil {
		t.Error("DeleteMessages() expected error for empty list, got nil")
	}
}

func TestClient_RetryMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/retry" {
			t.Errorf("Expected path /retry, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("Expected method POST, got %s", r.Method)
		}

		var payload map[string][]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		expectedIDs := []string{"msg1", "msg2"}
		if len(payload["ids"]) != len(expectedIDs) {
			t.Errorf("Expected %d IDs, got %d", len(expectedIDs), len(payload["ids"]))
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		config:     &Config{MaxRetries: 1, RetryDelay: time.Millisecond},
	}

	ctx := context.Background()
	err := client.RetryMessages(ctx, []string{"msg1", "msg2"})
	if err != nil {
		t.Fatalf("RetryMessages() error = %v", err)
	}
}

func TestClient_RetryMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string][]string
		json.NewDecoder(r.Body).Decode(&payload)

		if len(payload["ids"]) != 1 || payload["ids"][0] != "msg1" {
			t.Errorf("Expected single ID 'msg1', got %v", payload["ids"])
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		config:     &Config{MaxRetries: 1, RetryDelay: time.Millisecond},
	}

	ctx := context.Background()
	err := client.RetryMessage(ctx, "msg1")
	if err != nil {
		t.Fatalf("RetryMessage() error = %v", err)
	}
}

func TestClient_RetryMessages_EmptyList(t *testing.T) {
	client := NewClient()
	ctx := context.Background()

	err := client.RetryMessages(ctx, []string{})
	if err == nil {
		t.Error("RetryMessages() expected error for empty list, got nil")
	}
}

func TestClient_PurgeQueue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/purge" {
			t.Errorf("Expected path /purge, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("Expected method POST, got %s", r.Method)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		config:     &Config{MaxRetries: 1, RetryDelay: time.Millisecond},
	}

	ctx := context.Background()
	err := client.PurgeQueue(ctx)
	if err != nil {
		t.Fatalf("PurgeQueue() error = %v", err)
	}
}

func TestClient_RetryLogic(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		config:     &Config{MaxRetries: 3, RetryDelay: time.Millisecond},
	}

	ctx := context.Background()
	err := client.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}

	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		config:     &Config{MaxRetries: 3, RetryDelay: time.Millisecond},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := client.HealthCheck(ctx)
	if err == nil {
		t.Error("Expected context cancellation error, got nil")
	}
}

