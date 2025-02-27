package solver

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestTokenIdURLEscapingWithQueryEscape(t *testing.T) {
	tests := []struct {
		name     string
		tokenId  string
		expected string
	}{
		{
			name:     "Complex token ID with forward slash",
			tokenId:  "M0Elwz5/odEcC2fYQJ750BjcKKhjrQTyDtjTpnZOaQY",
			expected: "M0Elwz5/odEcC2fYQJ750BjcKKhjrQTyDtjTpnZOaQY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			escaped := url.QueryEscape(tt.tokenId)
			if escaped != tt.expected {
				t.Errorf("url.QueryEscape(%q) = %q, want %q", tt.tokenId, escaped, tt.expected)
			}

			// Verify we can unescape back to original
			unescaped, err := url.QueryUnescape(escaped)
			if err != nil {
				t.Errorf("Failed to unescape: %v", err)
			}
			if unescaped != tt.tokenId {
				t.Errorf("url.QueryUnescape(%q) = %q, want %q", escaped, unescaped, tt.tokenId)
			}
		})
	}
}

func TestWebSocketPingPong(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client := NewClient("", "", "")
		client.HandleWebSocket(w, r)
	}))
	defer server.Close()

	// Convert http URL to ws URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect to the WebSocket server
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Could not connect to WebSocket server: %v", err)
	}
	defer ws.Close()

	// Read the initial connection message
	var connectMsg WSMessage
	err = ws.ReadJSON(&connectMsg)
	if err != nil {
		t.Fatalf("Failed to read connection message: %v", err)
	}

	// Verify connection message
	if connectMsg.Type != "connected" {
		t.Errorf("Expected message type 'connected', got '%s'", connectMsg.Type)
	}
	if connectMsg.Data != "Successfully connected to WebSocket" {
		t.Errorf("Unexpected connection message: %v", connectMsg.Data)
	}

	// Send ping message
	pingMsg := WSMessage{
		Type: "ping",
		Data: "ping",
	}
	err = ws.WriteJSON(pingMsg)
	if err != nil {
		t.Fatalf("Failed to send ping message: %v", err)
	}

	// Read pong response with timeout
	done := make(chan bool)
	var pongMsg WSMessage
	go func() {
		err := ws.ReadJSON(&pongMsg)
		if err != nil {
			t.Errorf("Failed to read pong message: %v", err)
			done <- false
			return
		}
		done <- true
	}()

	// Wait for response with timeout
	select {
	case success := <-done:
		if !success {
			return
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for pong response")
		return
	}

	// Verify pong response
	if pongMsg.Type != "pong" {
		t.Errorf("Expected message type 'pong', got '%s'", pongMsg.Type)
	}
	if pongMsg.Data != "pong" {
		t.Errorf("Expected pong data 'pong', got '%v'", pongMsg.Data)
	}

	// Test unknown message type
	unknownMsg := WSMessage{
		Type: "unknown",
		Data: "test",
	}
	err = ws.WriteJSON(unknownMsg)
	if err != nil {
		t.Fatalf("Failed to send unknown message: %v", err)
	}

	// Read error response
	var errorMsg WSMessage
	err = ws.ReadJSON(&errorMsg)
	if err != nil {
		t.Fatalf("Failed to read error message: %v", err)
	}

	// Verify error response
	if errorMsg.Type != "error" {
		t.Errorf("Expected message type 'error', got '%s'", errorMsg.Type)
	}
	if errorMsg.Error != "Unknown message type" {
		t.Errorf("Expected error 'Unknown message type', got '%s'", errorMsg.Error)
	}
}
