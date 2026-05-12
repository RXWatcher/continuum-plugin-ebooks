package kindle

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/gomail.v2"
)

type fakeDialer struct {
	last *gomail.Message
	err  error
}

func (f *fakeDialer) DialAndSend(m ...*gomail.Message) error {
	if len(m) > 0 {
		f.last = m[0]
	}
	return f.err
}

func TestSender_AttachesFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "book.epub")
	if err := writeFile(path, []byte("epubbytes")); err != nil {
		t.Fatal(err)
	}
	fd := &fakeDialer{}
	s := New(SMTPConfig{Host: "smtp.example.com", Port: 587, Username: "u", Password: "p", From: "f@x.com"})
	s.dialer = func(host string, port int, username, password string) gomailDialer { return fd }
	if err := s.Send(context.Background(), "alice@kindle.com", "Hello", path, "book.epub"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if fd.last == nil {
		t.Fatal("no message")
	}
	var buf strings.Builder
	if _, err := fd.last.WriteTo(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "To: alice@kindle.com") {
		t.Errorf("missing To header: %q", firstFewLines(out))
	}
	if !strings.Contains(out, "Subject: Hello") {
		t.Errorf("missing subject")
	}
	if !strings.Contains(out, `filename="book.epub"`) && !strings.Contains(out, `filename=book.epub`) {
		t.Errorf("missing attachment filename")
	}
}

func TestSender_MissingConfigErrors(t *testing.T) {
	s := New(SMTPConfig{})
	if err := s.Send(context.Background(), "a@k.com", "s", "/dev/null", "x.epub"); err == nil {
		t.Error("expected error for missing host/port")
	}
}

func TestSender_MissingRecipientErrors(t *testing.T) {
	s := New(SMTPConfig{Host: "x", Port: 25})
	if err := s.Send(context.Background(), "", "s", "/dev/null", "x.epub"); err == nil {
		t.Error("expected error for empty recipient")
	}
}

func TestSender_PropagatesDialerError(t *testing.T) {
	want := errors.New("smtp boom")
	fd := &fakeDialer{err: want}
	s := New(SMTPConfig{Host: "h", Port: 587, From: "f"})
	s.dialer = func(host string, port int, username, password string) gomailDialer { return fd }
	tmp := t.TempDir()
	path := filepath.Join(tmp, "b.epub")
	if err := writeFile(path, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := s.Send(context.Background(), "a@k.com", "s", path, "b.epub"); err == nil || !strings.Contains(err.Error(), "smtp boom") {
		t.Errorf("got %v", err)
	}
}

func writeFile(path string, b []byte) error {
	return writeFileImpl(path, b)
}

func firstFewLines(s string) string {
	if i := strings.IndexByte(s, '\n'); i > 0 && i < 200 {
		return s[:i]
	}
	if len(s) > 200 {
		return s[:200]
	}
	return s
}
