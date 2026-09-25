package mailer

import (
	"bytes"
	"context"
	"net"
	"net/mail"
	"strings"
	"testing"
	"time"
)

// A server that accepts the TCP connection but never writes the initial SMTP greeting
// reproduces the exact class of hang this test guards against: a port/TLS-mode mismatch
// (e.g. STARTTLS spoken to an implicit-TLS port) or an unresponsive server, where the
// connection succeeds but the conversation never progresses. Before defaultSMTPTimeout was
// added, Send had no deadline past the initial dial and would block here forever.
func TestSMTP_Send_timesOutOnUnresponsiveServer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() }) //nolint:errcheck

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		// Accept the connection and hold it open — no greeting, no response, ever.
		<-t.Context().Done()
		conn.Close() //nolint:errcheck
	}()

	host, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}

	s := NewSMTP(host, port, "", "", false, false, "from@test.local", "Test")

	// A short ctx deadline, not defaultSMTPTimeout's 30s, keeps this test fast — Send picks
	// whichever deadline is earlier, so this proves ctx is actually honoured end-to-end, not
	// just at the dial step.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	start := time.Now()
	err = s.Send(ctx, Message{To: []string{"to@test.local"}, Subject: "test", Text: "body"})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Send succeeded against an unresponsive server; want a timeout error")
	}
	if elapsed > 5*time.Second {
		t.Errorf("Send took %v to fail; want it bounded by the ctx deadline (300ms), not hanging", elapsed)
	}
}

// A deadline is set even when ctx carries none of its own — defaultSMTPTimeout is the
// fallback, not an optional extra.
func TestSMTP_Send_appliesDefaultTimeoutWithNoCtxDeadline(t *testing.T) {
	orig := defaultSMTPTimeout
	defaultSMTPTimeout = 300 * time.Millisecond
	t.Cleanup(func() { defaultSMTPTimeout = orig })

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() }) //nolint:errcheck

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		<-t.Context().Done()
		conn.Close() //nolint:errcheck
	}()

	host, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}

	s := NewSMTP(host, port, "", "", false, false, "from@test.local", "Test")

	start := time.Now()
	// No deadline on this ctx at all — must still fall back to defaultSMTPTimeout.
	err = s.Send(context.Background(), Message{To: []string{"to@test.local"}, Subject: "test", Text: "body"})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Send succeeded against an unresponsive server; want a timeout error")
	}
	if elapsed > 5*time.Second {
		t.Errorf("Send took %v to fail; want it bounded by defaultSMTPTimeout (300ms), not hanging", elapsed)
	}
}

// Without Date and Message-ID the receiving server adds its own after DKIM signing,
// breaking a signature that covers them (Outlook: "dkim=fail", message junked).
func TestBuildRaw_setsDateAndMessageID(t *testing.T) {
	s := NewSMTP("localhost", "25", "", "", false, false, "agenda@example.com", "Example")
	raw := func() *mail.Message {
		m, err := mail.ReadMessage(bytes.NewReader(s.buildRaw(Message{To: []string{"to@test.local"}, Subject: "hola", Text: "cuerpo"})))
		if err != nil {
			t.Fatalf("ReadMessage: %v", err)
		}
		return m
	}
	a, b := raw(), raw()
	if d, err := a.Header.Date(); err != nil || time.Since(d) > time.Minute {
		t.Errorf("Date header = %q (%v); want a current RFC 5322 date", a.Header.Get("Date"), err)
	}
	id := a.Header.Get("Message-Id")
	if !strings.HasPrefix(id, "<") || !strings.HasSuffix(id, "@example.com>") {
		t.Errorf("Message-ID = %q; want <…@example.com> (the sender's domain)", id)
	}
	if id == b.Header.Get("Message-Id") {
		t.Errorf("two messages share Message-ID %q; want unique", id)
	}
}
