// SPDX-License-Identifier: MIT
// Purpose: test for the LSP framing fix — verifies that interleaved
// notifications (window/logMessage, $/progress) between request/response
// frames don't break the response reader.
package lsp

import (
	"bufio"
	"encoding/json"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

const notifBodyShort = `{"jsonrpc":"2.0","method":"window/logMessage","params":{"type":1,"message":"hi"}}`
const notifBodyLong = `{"jsonrpc":"2.0","method":"window/logMessage","params":{"type":1,"message":"1"}}`
const respBody = `{"jsonrpc":"2.0","id":1,"result":null}`
const respBodyOK = `{"jsonrpc":"2.0","id":42,"result":"ok"}`

func frame(headers ...string) string {
	var b strings.Builder
	for _, h := range headers {
		b.WriteString(h)
		b.WriteString("\r\n")
	}
	b.WriteString("\r\n")
	return b.String()
}

func TestLSPFrameReaderSingleResponse(t *testing.T) {
	r := bufio.NewReader(strings.NewReader(frame("Content-Length: 38") + respBody))
	deadline := time.Now().Add(2 * time.Second)
	f, err := readRawLSPFrame(r, deadline)
	if err != nil {
		t.Fatalf("readRawLSPFrame: %v", err)
	}
	if !strings.Contains(string(f), `"id"`) {
		t.Errorf("expected response with id, got: %s", string(f))
	}
}

func TestLSPFrameReaderResponseAfterNotification(t *testing.T) {
	notif := "Content-Length: " + intToStr(len(notifBodyShort)) + "\r\n\r\n" + notifBodyShort
	resp := "Content-Length: 38\r\n\r\n" + respBody
	r := bufio.NewReader(strings.NewReader(notif + resp))
	c := &Client{stdout: r}
	deadline := time.Now().Add(2 * time.Second)
	f, err := c.readLSPFrame(deadline)
	if err != nil {
		t.Fatalf("readLSPFrame: %v", err)
	}
	if f.ID == nil {
		t.Errorf("expected response, got: %v", f)
	}
}

func TestLSPFrameReaderNotificationAfterResponse(t *testing.T) {
	resp := "Content-Length: 38\r\n\r\n" + respBody
	notif := "Content-Length: 84\r\n\r\n" + `{"jsonrpc":"2.0","method":"window/logMessage","params":{"type":1,"message":"after"}}`
	r := bufio.NewReader(strings.NewReader(resp + notif))
	deadline := time.Now().Add(2 * time.Second)
	f, err := readRawLSPFrame(r, deadline)
	if err != nil {
		t.Fatalf("readRawLSPFrame: %v", err)
	}
	if !strings.Contains(string(f), `"id"`) {
		t.Errorf("expected response first, got: %s", string(f))
	}
}

func TestLSPFrameReaderManyInterleaved(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 3; i++ {
		body := strings.Replace(notifBodyLong, `"1"`, `"`+intToStr(i+1)+`"`, 1)
		b.WriteString("Content-Length: ")
		b.WriteString(intToStr(len(body)))
		b.WriteString("\r\n\r\n")
		b.WriteString(body)
	}
	resp := "Content-Length: 39\r\n\r\n" + respBodyOK
	b.WriteString(resp)
	r := bufio.NewReader(strings.NewReader(b.String()))
	c := &Client{stdout: r}
	deadline := time.Now().Add(2 * time.Second)
	f, err := c.readLSPFrame(deadline)
	if err != nil {
		t.Fatalf("readLSPFrame: %v", err)
	}
	if id, ok := f.ID.(float64); !ok || id != 42 {
		t.Errorf("expected response id=42 after 3 notifications, got: %v", f.ID)
	}
}

func TestLSPFrameReaderNoContentLength(t *testing.T) {
	r := bufio.NewReader(strings.NewReader(""))
	deadline := time.Now().Add(500 * time.Millisecond)
	_, err := readRawLSPFrame(r, deadline)
	if err == nil {
		t.Error("expected error for no Content-Length")
	}
}

func TestLSPFrameReaderInvalidContentLength(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("Content-Length: notanumber\n\n"))
	deadline := time.Now().Add(500 * time.Millisecond)
	_, err := readRawLSPFrame(r, deadline)
	if err == nil {
		t.Error("expected error for invalid Content-Length")
	}
}

func TestLSPNotificationsViaHandler(t *testing.T) {
	var mu sync.Mutex
	notifications := []string{}
	handler := func(method string, params json.RawMessage) {
		mu.Lock()
		defer mu.Unlock()
		notifications = append(notifications, method)
	}

	notif := "Content-Length: " + intToStr(len(notifBodyShort)) + "\r\n\r\n" + notifBodyShort
	resp := "Content-Length: 51\r\n\r\n" + `{"jsonrpc":"2.0","id":1,"result":{"value":"hello"}}`
	body := notif + resp

	ln, client := makeFakeLSPServer(t, body)
	defer ln.Close()

	c := &Client{
		stdout:              bufio.NewReader(client),
		notificationHandler: handler,
	}

	deadline := time.Now().Add(2 * time.Second)
	frame, err := c.readLSPFrame(deadline)
	if err != nil {
		t.Fatalf("readLSPFrame: %v", err)
	}
	if id, ok := frame.ID.(float64); !ok || id != 1 {
		t.Errorf("expected response id=1, got: %v", frame.ID)
	}
	if result := string(frame.Result); !strings.Contains(result, "hello") {
		t.Errorf("expected result, got: %s", result)
	}

	mu.Lock()
	n := len(notifications)
	mu.Unlock()
	if n != 1 {
		t.Errorf("expected 1 notification, got: %d", n)
	}
}

func makeFakeLSPServer(t *testing.T, body string) (net.Listener, net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	connCh := make(chan net.Conn, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		connCh <- conn
		_, _ = conn.Write([]byte(body))
		_ = conn.Close()
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		ln.Close()
		t.Fatal(err)
	}
	<-connCh
	return ln, conn
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
