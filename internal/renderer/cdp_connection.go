package renderer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/utils"
)

type cdpEvent struct {
	Method    string
	Params    json.RawMessage
	SessionID string
}

type cdpConnection struct {
	ws          *wsClient
	mu          sync.Mutex
	nextID      uint64
	nextSubID   uint64
	pending     map[uint64]chan cdpResponse
	events      chan cdpEvent
	subscribers map[uint64]chan cdpEvent
	closed      chan struct{}
}

type cdpResponse struct {
	result json.RawMessage
	errMsg string
}

type rawCDPMessage struct {
	ID        *uint64         `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *rawCDPError    `json:"error,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
}

type rawCDPError struct {
	Message string `json:"message"`
}

type BrowserOption func(*Browser)

func WithBrowserLogf(f func(string, ...any)) BrowserOption {
	return func(b *Browser) { b.logf = f }
}

func WithBrowserErrorf(f func(string, ...any)) BrowserOption {
	return func(b *Browser) { b.errf = f }
}

func WithBrowserDebugf(f func(string, ...any)) BrowserOption {
	return func(b *Browser) { b.dbgf = f }
}

func WithDialTimeout(d time.Duration) BrowserOption {
	return func(b *Browser) { b.dialTimeout = d }
}

type Browser struct {
	next              int64
	LostConnection    chan struct{}
	closingGracefully chan struct{}
	dialTimeout       time.Duration
	pages             map[string]*Target
	listenersMu       sync.RWMutex
	listeners         []cancelableListener
	conn              *cdpConnection
	newTabQueue       chan *Target
	cmdQueue          chan *cdpOutMessage
	logf              func(string, ...any)
	errf              func(string, ...any)
	dbgf              func(string, ...any)
}

type cdpOutMessage struct {
	ID     int64
	Method string
	Params json.RawMessage
}

type cancelableListener struct {
	ctx context.Context
	fn  func(any)
}

type Target struct {
	browser      *Browser
	TargetID     string
	SessionID    string
	messageQueue chan *cdpMessage
	execContexts map[string]string
	cur          string
	logf         func(string, ...any)
	errf         func(string, ...any)
}

type cdpMessage struct {
	ID        *int64          `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
}

func runListeners(listeners []cancelableListener, ev any) []cancelableListener {
	var result []cancelableListener
	for _, l := range listeners {
		select {
		case <-l.ctx.Done():
		default:
			l.fn(ev)
			result = append(result, l)
		}
	}
	return result
}

// dialCDPConnection opens a WebSocket connection to a browser's CDP endpoint.
func dialCDPConnection(wsURL string, timeout time.Duration) (*cdpConnection, error) {
	resolvedWSURL, err := resolveBrowserWSURL(wsURL, timeout)
	if err != nil {
		return nil, err
	}

	ws, err := dialWebSocket(resolvedWSURL, timeout)
	if err != nil {
		return nil, err
	}
	conn := &cdpConnection{
		ws:          ws,
		pending:     make(map[uint64]chan cdpResponse),
		events:      make(chan cdpEvent, 1024),
		subscribers: make(map[uint64]chan cdpEvent),
		closed:      make(chan struct{}),
	}
	go conn.reader()
	return conn, nil
}

// resolveBrowserWSURL converts a configured CDP endpoint into the concrete
// browser-level WebSocket URL Chrome expects. Chrome often exposes a root
// HTTP endpoint (for example `ws://127.0.0.1:9222/`) and publishes the real
// websocket under `/json/version`.
func resolveBrowserWSURL(wsURL string, timeout time.Duration) (string, error) {
	trimmed := strings.TrimSpace(wsURL)
	if trimmed == "" {
		return "", fmt.Errorf("empty CDP websocket URL")
	}
	if strings.Contains(trimmed, "/devtools/") || isBrowserlessDirectWS(trimmed) {
		return trimmed, nil
	}

	if ws, err := dialWebSocket(trimmed, 3*time.Second); err == nil {
		_ = ws.Close()
		return trimmed, nil
	}

	httpURL := strings.TrimRight(strings.NewReplacer("ws://", "http://", "wss://", "https://").Replace(trimmed), "/")
	httpURL += "/json/version"

	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodGet, httpURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Host", req.URL.Host)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("CDP discovery failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("CDP discovery failed: %s", resp.Status)
	}

	var payload struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("CDP discovery parse error: %w", err)
	}
	if strings.TrimSpace(payload.WebSocketDebuggerURL) == "" {
		return "", fmt.Errorf("no webSocketDebuggerUrl in /json/version")
	}

	rewritten, err := rewriteResolvedWSHost(payload.WebSocketDebuggerURL, trimmed)
	if err != nil {
		return "", err
	}
	return rewritten, nil
}

// rewriteResolvedWSHost keeps the host:port from the configured endpoint
// while preserving the browser-provided `/devtools/browser/...` path.
func rewriteResolvedWSHost(discovered, configured string) (string, error) {
	confURL, err := url.Parse(configured)
	if err != nil {
		return "", err
	}
	discURL, err := url.Parse(discovered)
	if err != nil {
		return "", err
	}
	discURL.Scheme = confURL.Scheme
	discURL.Host = confURL.Host
	return discURL.String(), nil
}

// isBrowserlessDirectWS reports whether the configured endpoint already
// points at a browserless-style direct websocket path instead of Chrome's
// /json/version discovery endpoint.
func isBrowserlessDirectWS(wsURL string) bool {
	if strings.Contains(wsURL, "token=") {
		return true
	}
	return strings.Contains(wsURL, "/chromium") ||
		strings.Contains(wsURL, "/firefox") ||
		strings.Contains(wsURL, "/webkit")
}

// Subscribe returns a channel that receives CDP events for this connection.
// Callers should stop reading when the connection is closed.
func (c *cdpConnection) Subscribe() <-chan cdpEvent {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := c.nextSubID
	c.nextSubID++
	ch := make(chan cdpEvent, 256)
	c.subscribers[id] = ch
	return ch
}

// Close shuts down the CDP connection and fails any in-flight requests.
func (c *cdpConnection) Close() {
	select {
	case <-c.closed:
		return
	default:
		close(c.closed)
	}
	_ = c.ws.Close()

	c.mu.Lock()
	defer c.mu.Unlock()
	for id, ch := range c.pending {
		select {
		case ch <- cdpResponse{errMsg: "CDP connection closed"}:
		default:
		}
		delete(c.pending, id)
	}
	c.subscribers = map[uint64]chan cdpEvent{}
}

// SendRecv sends one CDP command and waits for the matching response.
func (c *cdpConnection) SendRecv(method string, params map[string]any, sessionID string, timeout time.Duration) (json.RawMessage, error) {
	select {
	case <-c.closed:
		return nil, fmt.Errorf("CDP connection closed")
	default:
	}

	c.mu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan cdpResponse, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	payload := map[string]any{
		"id":     id,
		"method": method,
		"params": params,
	}
	if sessionID != "" {
		payload["sessionId"] = sessionID
	}
	body, _ := json.Marshal(payload)

	if err := c.ws.sendWebSocketFrame(0x1, body); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}

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

// WaitForEvent blocks until the requested CDP event arrives or the timeout expires.
func (c *cdpConnection) WaitForEvent(method, sessionID string, timeout time.Duration) (cdpEvent, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	events := c.Subscribe()

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return cdpEvent{}, fmt.Errorf("CDP event channel closed")
			}
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

// WaitForPageReady waits for the main document load event and returns the HTTP status code observed.
func (c *cdpConnection) WaitForPageReady(events <-chan cdpEvent, sessionID string, timeout time.Duration) (uint16, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	status := uint16(200)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return 0, errors.New("CDP event channel closed before load")
			}
			if ev.SessionID != sessionID {
				continue
			}
			switch ev.Method {
			case "Network.responseReceived":
				var payload map[string]any
				if err := json.Unmarshal(ev.Params, &payload); err != nil {
					continue
				}
				if typ, _ := payload["type"].(string); typ != "Document" {
					continue
				}
				if resp, ok := payload["response"].(map[string]any); ok {
					if rawStatus, ok := resp["status"].(float64); ok && rawStatus > 0 {
						status = uint16(rawStatus)
					}
				}
			case "Page.loadEventFired":
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

// CreateBrowserContext creates an isolated browser context on the CDP backend.
func (c *cdpConnection) CreateBrowserContext(timeout time.Duration) (string, error) {
	res, err := c.SendRecv("Target.createBrowserContext", map[string]any{}, "", timeout)
	if err != nil {
		return "", err
	}
	ctxID := utils.ExtractJSONStringField(res, "browserContextId")
	if ctxID == "" {
		return "", errors.New("Target.createBrowserContext: missing browserContextId")
	}
	return ctxID, nil
}

// DisposeBrowserContext disposes an isolated browser context.
func (c *cdpConnection) DisposeBrowserContext(ctxID string, timeout time.Duration) error {
	_, err := c.SendRecv("Target.disposeBrowserContext", map[string]any{"browserContextId": ctxID}, "", timeout)
	return err
}

// CloseTarget closes a CDP target and ignores the returned payload.
func (c *cdpConnection) CloseTarget(targetID string, timeout time.Duration) error {
	_, err := c.SendRecv("Target.closeTarget", map[string]any{"targetId": targetID}, "", timeout)
	return err
}

// reader continuously reads CDP messages from the WebSocket and routes them to
// pending command responses or event consumers.
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
			ev := cdpEvent{Method: raw.Method, Params: raw.Params, SessionID: raw.SessionID}
			select {
			case c.events <- ev:
			default:
			}

			c.mu.Lock()
			subscribers := make([]chan cdpEvent, 0, len(c.subscribers))
			for _, ch := range c.subscribers {
				subscribers = append(subscribers, ch)
			}
			c.mu.Unlock()
			for _, ch := range subscribers {
				select {
				case ch <- ev:
				default:
				}
			}
			continue
		}
	}
}

// Process is part of the browser executor interface used by the legacy target pipeline.
func (b *Browser) Process() interface{} {
	return nil
}

// newExecutorForTarget registers a new target session with the browser event loop.
func (b *Browser) newExecutorForTarget(ctx context.Context, targetID, sessionID string) (*Target, error) {
	if targetID == "" {
		return nil, errors.New("empty target ID")
	}
	if sessionID == "" {
		return nil, errors.New("empty session ID")
	}

	t := &Target{
		browser:      b,
		TargetID:     targetID,
		SessionID:    sessionID,
		messageQueue: make(chan *cdpMessage, 1024),
		execContexts: make(map[string]string),
		cur:          targetID,
		logf:         b.logf,
		errf:         b.errf,
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case b.newTabQueue <- t:
	}

	return t, nil
}

// Execute sends a CDP command through the browser event loop and optionally decodes the result.
func (b *Browser) Execute(ctx context.Context, method string, params, res any) error {
	return b.execute(ctx, method, params, res)
}

func (b *Browser) execute(ctx context.Context, method string, params, res any) error {
	id := atomic.AddInt64(&b.next, 1)

	lctx, cancel := context.WithCancel(ctx)
	ch := make(chan *cdpMessage, 1)

	fn := func(ev any) {
		if msg, ok := ev.(*cdpMessage); ok && msg.ID != nil && *msg.ID == id {
			select {
			case <-ctx.Done():
			case ch <- msg:
			}
			cancel()
		}
	}

	b.listenersMu.Lock()
	b.listeners = append(b.listeners, cancelableListener{lctx, fn})
	b.listenersMu.Unlock()

	var buf []byte
	var err error
	if params != nil {
		buf, err = json.Marshal(params)
		if err != nil {
			return err
		}
	}

	cmd := &cdpOutMessage{
		ID:     id,
		Method: method,
		Params: buf,
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case b.cmdQueue <- cmd:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case msg := <-ch:
		switch {
		case msg == nil:
			return errors.New("channel closed")
		case res != nil && len(msg.Result) > 0 && string(msg.Result) != "null":
			return json.Unmarshal(msg.Result, res)
		}
	}

	return nil
}

// run owns the browser's internal event loop and dispatches CDP messages.
func (b *Browser) run(ctx context.Context) {
	defer b.conn.Close()

	incomingQueue := make(chan *cdpMessage, 1)
	delTabQueue := make(chan string, 1)

	go func() {
		defer close(b.LostConnection)
		for {
			msg := &cdpMessage{}
			if err := b.conn.readMessage(ctx, msg); err != nil {
				b.errf("%s", err)
				return
			}

			switch {
			case msg.SessionID != "" && (msg.Method != "" || msg.ID != nil):
				select {
				case <-ctx.Done():
					return
				case incomingQueue <- msg:
				}
			case msg.Method != "":
				b.listenersMu.Lock()
				b.listeners = runListeners(b.listeners, msg)
				b.listenersMu.Unlock()
			case msg.ID != nil:
				b.listenersMu.Lock()
				b.listeners = runListeners(b.listeners, msg)
				b.listenersMu.Unlock()
			default:
				b.errf("ignoring malformed message: %#v", msg)
			}
		}
	}()

	b.pages = make(map[string]*Target, 32)

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-b.cmdQueue:
			body, err := json.Marshal(msg)
			if err != nil {
				b.errf("%s", err)
				continue
			}
			if err := b.conn.ws.sendWebSocketFrame(0x1, body); err != nil {
				b.errf("%s", err)
				continue
			}
		case t := <-b.newTabQueue:
			if _, ok := b.pages[t.SessionID]; ok {
				b.errf("executor for %q already exists", t.SessionID)
			}
			b.pages[t.SessionID] = t
		case sessionID := <-delTabQueue:
			delete(b.pages, sessionID)
		case m := <-incomingQueue:
			page, ok := b.pages[m.SessionID]
			if !ok {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case page.messageQueue <- m:
			}
		case <-b.LostConnection:
			return
		}
	}
}

// Close stops the browser event loop and closes the underlying CDP connection.
func (b *Browser) Close() {
	close(b.closingGracefully)
	b.conn.Close()
}

// readMessage reads one CDP payload into the provided target message.
func (c *cdpConnection) readMessage(ctx context.Context, msg *cdpMessage) error {
	timeout := time.After(30 * time.Second)

	c.mu.Lock()
	id := c.nextID
	c.nextID++
	c.mu.Unlock()

	body, _ := json.Marshal(map[string]any{"id": id})
	if err := c.ws.sendWebSocketFrame(0x1, body); err != nil {
		return err
	}

	c.mu.Lock()
	ch := make(chan cdpResponse, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case resp := <-ch:
		if resp.errMsg != "" {
			return errors.New(resp.errMsg)
		}
		if len(resp.result) > 0 {
			if err := json.Unmarshal(resp.result, msg); err != nil {
				return err
			}
		}
		return nil
	case <-timeout:
		return errors.New("read timeout")
	}
}
