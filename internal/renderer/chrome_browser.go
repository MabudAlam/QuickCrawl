package renderer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

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
	ID        *int64  `json:"id,omitempty"`
	Method    string  `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	SessionID string  `json:"sessionId,omitempty"`
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

func NewBrowser(ctx context.Context, urlstr string, opts ...BrowserOption) (*Browser, error) {
	b := &Browser{
		LostConnection:    make(chan struct{}),
		closingGracefully: make(chan struct{}),
		dialTimeout:       10 * time.Second,
		newTabQueue:       make(chan *Target),
		cmdQueue:          make(chan *cdpOutMessage, 32),
		logf:              log.Printf,
		errf:              func(format string, v ...any) { log.Printf("ERROR: "+format, v...) },
	}

	for _, o := range opts {
		o(b)
	}

	if b.errf == nil {
		b.errf = b.logf
	}

	conn, err := connectCDP(urlstr, b.dialTimeout)
	if err != nil {
		return nil, fmt.Errorf("could not dial %q: %w", urlstr, err)
	}
	b.conn = conn

	go b.run(ctx)
	return b, nil
}

func (b *Browser) Process() interface{} {
	return nil
}

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

func (b *Browser) Close() {
	close(b.closingGracefully)
	b.conn.Close()
}

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