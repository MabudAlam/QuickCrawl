package renderer

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// cdpEvent represents a Chrome DevTools Protocol event from the browser.
type cdpEvent struct {
	Method    string          // Event method name (e.g., "Network.responseReceived")
	Params    json.RawMessage // Event parameters
	SessionID string          // Session ID for target
}

// cdpConnection manages a WebSocket connection to a Chrome DevTools Protocol endpoint.
type cdpConnection struct {
	ws      *wsClient                   // WebSocket client
	mu      sync.Mutex                  // Protects nextID and pending
	nextID  uint64                      // Next message ID
	pending map[uint64]chan cdpResponse // Pending requests awaiting response
	events  chan cdpEvent               // Channel for incoming events
	closed  chan struct{}               // Close signal
}

// cdpResponse holds a response to a CDP request.
type cdpResponse struct {
	result json.RawMessage // Response data
	errMsg string          // Error message if failed
}

// rawCDPMessage represents a CDP protocol message (request, response, or event).
type rawCDPMessage struct {
	ID        *uint64         `json:"id,omitempty"`        // Message ID (for requests/responses)
	Method    string          `json:"method,omitempty"`    // Method name (for requests/events)
	Params    json.RawMessage `json:"params,omitempty"`    // Method parameters
	Result    json.RawMessage `json:"result,omitempty"`    // Response result
	Error     *rawCDPError    `json:"error,omitempty"`     // Error object
	SessionID string          `json:"sessionId,omitempty"` // Session ID
}

// rawCDPError represents a CDP protocol error.
type rawCDPError struct {
	Message string `json:"message"` // Error message
}

// connectCDP establishes a WebSocket connection to a CDP endpoint.
func connectCDP(wsURL string, timeout time.Duration) (*cdpConnection, error) {
	ws, err := dialWebSocket(wsURL, timeout)
	if err != nil {
		return nil, err
	}
	conn := &cdpConnection{
		ws:      ws,
		pending: make(map[uint64]chan cdpResponse),
		events:  make(chan cdpEvent, 1024),
		closed:  make(chan struct{}),
	}
	go conn.reader()
	return conn, nil
}

// Close closes the CDP connection and all pending requests.
func (c *cdpConnection) Close() {
	// Signal closure
	select {
	case <-c.closed:
		return
	default:
		close(c.closed)
	}
	_ = c.ws.Close()

	// Notify all pending requests of closure
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, ch := range c.pending {
		select {
		case ch <- cdpResponse{errMsg: "CDP connection closed"}:
		default:
		}
		delete(c.pending, id)
	}
}

// SendRecv sends a CDP command and waits for the response.
func (c *cdpConnection) SendRecv(method string, params map[string]any, sessionID string, timeout time.Duration) (json.RawMessage, error) {
	// Check if already closed
	select {
	case <-c.closed:
		return nil, fmt.Errorf("CDP connection closed")
	default:
	}

	// Allocate message ID and response channel
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan cdpResponse, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	// Build message payload
	payload := map[string]any{
		"id":     id,
		"method": method,
		"params": params,
	}
	if sessionID != "" {
		payload["sessionId"] = sessionID
	}
	body, _ := json.Marshal(payload)

	// Send the message
	if err := c.ws.sendWebSocketFrame(0x1, body); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}

	// Wait for response or timeout
	select {
	case resp := <-ch:
		if resp.errMsg != "" {
			return nil, errors.New(resp.errMsg)
		}
		return resp.result, nil
	case <-time.After(timeout):
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, errors.New(fmt.Sprintf("CDP %s timed out after %s", method, timeout))
	case <-c.closed:
		return nil, fmt.Errorf("CDP connection closed")
	}
}

// WaitForEvent waits for a specific CDP event to occur.
func (c *cdpConnection) WaitForEvent(method, sessionID string, timeout time.Duration) (cdpEvent, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	for {
		select {
		case ev, ok := <-c.events:
			if !ok {
				return cdpEvent{}, fmt.Errorf("CDP event channel closed")
			}
			// Check if this is the event we're waiting for
			if ev.Method == method && (sessionID == "" || ev.SessionID == sessionID) {
				return ev, nil
			}
		case <-deadline.C:
			return cdpEvent{}, fmt.Errorf("CDP event %s timed out after %s", method, timeout)
		case <-c.closed:
			return cdpEvent{}, fmt.Errorf("CDP connection closed")
		}
	}
}

// WaitForPageReady waits for the page to finish loading.
// Returns the HTTP status code and any error.
func (c *cdpConnection) WaitForPageReady(sessionID string, timeout time.Duration) (uint16, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	status := uint16(200) // Default to 200 OK
	for {
		select {
		case ev, ok := <-c.events:
			if !ok {
				return 0, errors.New("CDP event channel closed before load")
			}
			// Skip events for other sessions
			if ev.SessionID != sessionID {
				continue
			}
			switch ev.Method {
			case "Network.responseReceived":
				// Extract HTTP status from document responses
				var payload map[string]any
				if err := json.Unmarshal(ev.Params, &payload); err != nil {
					continue
				}
				// Only care about main document responses
				if typ, _ := payload["type"].(string); typ != "Document" {
					continue
				}
				if resp, ok := payload["response"].(map[string]any); ok {
					if rawStatus, ok := resp["status"].(float64); ok && rawStatus > 0 {
						status = uint16(rawStatus)
					}
				}
			case "Page.loadEventFired":
				// Page fully loaded
				return status, nil
			case "Inspector.targetCrashed":
				return 0, errors.New("Target crashed during render")
			}
		case <-deadline.C:
			return 0, errors.New(fmt.Sprintf("CDP Page.loadEventFired timed out after %s", timeout))
		case <-c.closed:
			return 0, errors.New("CDP connection closed")
		}
	}
}

// reader handles incoming WebSocket messages and dispatches them.
func (c *cdpConnection) reader() {
	defer close(c.events)
	for {
		msg, err := c.ws.readTextMessage()
		if err != nil {
			c.Close()
			return
		}

		var raw rawCDPMessage
		if err := json.Unmarshal([]byte(msg), &raw); err != nil {
			continue
		}

		if raw.ID != nil {
			// This is a response to one of our requests
			c.mu.Lock()
			ch, ok := c.pending[*raw.ID]
			if ok {
				delete(c.pending, *raw.ID)
			}
			c.mu.Unlock()

			if ok {
				resp := cdpResponse{result: raw.Result}
				if raw.Error != nil {
					resp.errMsg = raw.Error.Message
				}
				select {
				case ch <- resp:
				default:
				}
			}
			continue
		}

		if raw.Method != "" {
			// This is an event from the browser
			select {
			case c.events <- cdpEvent{Method: raw.Method, Params: raw.Params, SessionID: raw.SessionID}:
			default:
				// Event queue full, drop event
			}
		}
	}
}

// wsClient is a minimal WebSocket client implementation.
type wsClient struct {
	conn    net.Conn      // TCP connection
	r       *bufio.Reader // Buffered reader
	w       *bufio.Writer // Buffered writer
	readMu  sync.Mutex    // Protects read operations
	writeMu sync.Mutex    // Protects write operations
}

// dialWebSocket establishes a WebSocket connection to the given URL.
func dialWebSocket(rawURL string, timeout time.Duration) (*wsClient, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	// Validate scheme
	scheme := strings.ToLower(u.Scheme)
	if scheme != "ws" && scheme != "wss" {
		return nil, fmt.Errorf("unsupported websocket scheme: %s", u.Scheme)
	}

	// Determine host and port
	host := u.Host
	if !strings.Contains(host, ":") {
		if scheme == "wss" {
			host += ":443"
		} else {
			host += ":80"
		}
	}

	// Establish TCP connection
	dialer := &net.Dialer{Timeout: timeout}
	var conn net.Conn
	if scheme == "wss" {
		conn, err = tls.DialWithDialer(dialer, "tcp", host, &tls.Config{MinVersion: tls.VersionTLS12})
	} else {
		conn, err = dialer.Dial("tcp", host)
	}
	if err != nil {
		return nil, err
	}

	// Generate WebSocket handshake key
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		_ = conn.Close()
		return nil, err
	}
	secKey := base64.StdEncoding.EncodeToString(key)

	// Build and send handshake request
	path := u.RequestURI()
	if path == "" {
		path = "/"
	}
	req := fmt.Sprintf(
		"GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n",
		path, u.Host, secKey,
	)
	if _, err := io.WriteString(conn, req); err != nil {
		_ = conn.Close()
		return nil, err
	}

	// Read handshake response
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodGet})
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	defer resp.Body.Close()

	// Verify handshake
	if resp.StatusCode != http.StatusSwitchingProtocols {
		_ = conn.Close()
		return nil, fmt.Errorf("websocket handshake failed: %s", resp.Status)
	}

	expected := computeWebSocketAccept(secKey)
	if strings.TrimSpace(resp.Header.Get("Sec-Websocket-Accept")) != expected &&
		strings.TrimSpace(resp.Header.Get("Sec-WebSocket-Accept")) != expected {
		_ = conn.Close()
		return nil, fmt.Errorf("websocket handshake accept mismatch")
	}

	return &wsClient{
		conn: conn,
		r:    br,
		w:    bufio.NewWriter(conn),
	}, nil
}

// computeWebSocketAccept computes the expected WebSocket accept header value.
func computeWebSocketAccept(key string) string {
	h := sha1.New()
	_, _ = h.Write([]byte(key))
	_, _ = h.Write([]byte("258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// Close closes the underlying TCP connection.
func (c *wsClient) Close() error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.Close()
}

// sendWebSocketFrame sends a WebSocket frame with the given opcode and payload.
// The opcode should be 0x1 for text frames and 0xA for pong frames.
func (c *wsClient) sendWebSocketFrame(opcode byte, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	// Generate mask key (all frames from client must be masked)
	mask := make([]byte, 4)
	if _, err := rand.Read(mask); err != nil {
		return err
	}

	// Build frame header
	header := []byte{0x80 | opcode}
	length := len(payload)
	switch {
	case length < 126:
		header = append(header, 0x80|byte(length))
	case length <= 0xffff:
		header = append(header, 0x80|126, byte(length>>8), byte(length))
	default:
		header = append(header, 0x80|127)
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(length))
		header = append(header, ext[:]...)
	}

	// Write header
	if _, err := c.w.Write(header); err != nil {
		return err
	}

	// Write masked payload
	masked := make([]byte, len(payload))
	for i := range payload {
		masked[i] = payload[i] ^ mask[i%4]
	}
	if _, err := c.w.Write(mask); err != nil {
		return err
	}
	if _, err := c.w.Write(masked); err != nil {
		return err
	}
	return c.w.Flush()
}

// sendPongFrame sends a pong frame in response to a ping.
// This is used for WebSocket keepalive/heartbeat.
func (c *wsClient) sendPongFrame(payload []byte) error {
	return c.sendWebSocketFrame(0xA, payload)
}

// readTextMessage reads a text frame from the WebSocket and returns its content.
// It handles continuation frames and pong responses.
func (c *wsClient) readTextMessage() (string, error) {
	var buf []byte
	for {
		fin, opcode, payload, err := c.readWebSocketFrame()
		if err != nil {
			return "", err
		}
		switch opcode {
		case 0x1, 0x0: // Text or continuation frame
			buf = append(buf, payload...)
			if fin {
				return string(buf), nil
			}
		case 0x8: // Close frame
			return "", io.EOF
		case 0x9: // Ping - respond with pong
			_ = c.sendPongFrame(payload)
		case 0xA: // Pong - ignore
			continue
		default:
			continue
		}
	}
}

// readWebSocketFrame reads a single WebSocket frame and returns its fin flag,
// opcode, payload, and any error. It handles masking and extended length.
func (c *wsClient) readWebSocketFrame() (bool, byte, []byte, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	// Read frame header
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(c.r, hdr); err != nil {
		return false, 0, nil, err
	}
	fin := hdr[0]&0x80 != 0
	opcode := hdr[0] & 0x0f
	masked := hdr[1]&0x80 != 0
	length := int(hdr[1] & 0x7f)

	// Read extended length if needed
	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(c.r, ext[:]); err != nil {
			return false, 0, nil, err
		}
		length = int(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(c.r, ext[:]); err != nil {
			return false, 0, nil, err
		}
		length = int(binary.BigEndian.Uint64(ext[:]))
	}

	// Read mask key if present
	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(c.r, maskKey[:]); err != nil {
			return false, 0, nil, err
		}
	}

	// Read payload
	payload := make([]byte, length)
	if _, err := io.ReadFull(c.r, payload); err != nil {
		return false, 0, nil, err
	}

	// Unmask if needed
	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}

	return fin, opcode, payload, nil
}
