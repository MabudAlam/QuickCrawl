package renderer

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// wsClient is a minimal WebSocket client implementation.
type wsClient struct {
	conn    net.Conn
	r       *bufio.Reader
	w       *bufio.Writer
	readMu  sync.Mutex
	writeMu sync.Mutex
}

func dialWebSocket(rawURL string, timeout time.Duration) (*wsClient, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "ws" && scheme != "wss" {
		return nil, fmt.Errorf("unsupported websocket scheme: %s", u.Scheme)
	}

	host := u.Host
	if !strings.Contains(host, ":") {
		if scheme == "wss" {
			host += ":443"
		} else {
			host += ":80"
		}
	}

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

	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		_ = conn.Close()
		return nil, err
	}
	secKey := base64.StdEncoding.EncodeToString(key)

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

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodGet})
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	defer resp.Body.Close()

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

func computeWebSocketAccept(key string) string {
	h := sha1.New()
	_, _ = h.Write([]byte(key))
	_, _ = h.Write([]byte("258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func (c *wsClient) Close() error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.Close()
}

func (c *wsClient) sendWebSocketFrame(opcode byte, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	mask := make([]byte, 4)
	if _, err := rand.Read(mask); err != nil {
		return err
	}

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

	if _, err := c.w.Write(header); err != nil {
		return err
	}

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

func (c *wsClient) sendPongFrame(payload []byte) error {
	return c.sendWebSocketFrame(0xA, payload)
}

func (c *wsClient) readTextMessage() (string, error) {
	var buf []byte
	for {
		fin, opcode, payload, err := c.readWebSocketFrame()
		if err != nil {
			return "", err
		}
		switch opcode {
		case 0x1, 0x0:
			buf = append(buf, payload...)
			if fin {
				return string(buf), nil
			}
		case 0x8:
			return "", io.EOF
		case 0x9:
			_ = c.sendPongFrame(payload)
		case 0xA:
			continue
		default:
			continue
		}
	}
}

func (c *wsClient) readWebSocketFrame() (bool, byte, []byte, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	hdr := make([]byte, 2)
	if _, err := io.ReadFull(c.r, hdr); err != nil {
		return false, 0, nil, err
	}
	fin := hdr[0]&0x80 != 0
	opcode := hdr[0] & 0x0f
	masked := hdr[1]&0x80 != 0
	length := int(hdr[1] & 0x7f)

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

	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(c.r, maskKey[:]); err != nil {
			return false, 0, nil, err
		}
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(c.r, payload); err != nil {
		return false, 0, nil, err
	}

	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}

	return fin, opcode, payload, nil
}
