// Package kindle implements the "Send to Kindle" SMTP transport.
//
// The portal queues a kindle_send_log row when the user clicks Send; the
// scheduled kindle_send_retrier picks up queued rows, fetches the EPUB from
// the streaming cache, and emails it as an attachment to the user's
// <kindle-id>@kindle.com address.
//
// Wire format mirrors gomail.v2 (RFC 2822 message + MIME multipart). The
// retrier converts errors into a per-row attempt counter (capped at 3 in the
// scheduler) so transient SMTP issues don't permanently fail a send.
package kindle

import (
	"context"
	"errors"
	"fmt"

	"gopkg.in/gomail.v2"
)

// SMTPConfig is the JSON shape stored in backend_config.kindle_smtp_config.
type SMTPConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	TLS      string `json:"tls"` // starttls | implicit | none
}

// Sender composes and dispatches Kindle emails.
type Sender struct {
	cfg    SMTPConfig
	dialer dialerFunc
}

// dialerFunc is replaceable in tests to avoid real SMTP.
type dialerFunc func(host string, port int, username, password string) gomailDialer

type gomailDialer interface {
	DialAndSend(m ...*gomail.Message) error
}

// New returns a Sender backed by gomail.v2.
func New(cfg SMTPConfig) *Sender {
	return &Sender{cfg: cfg, dialer: defaultDialer}
}

func defaultDialer(host string, port int, username, password string) gomailDialer {
	d := gomail.NewDialer(host, port, username, password)
	return d
}

// Send dials SMTP and emails attachmentPath as a MIME attachment named
// attachmentName. Subject becomes the email subject line. to is the
// recipient's @kindle.com address.
func (s *Sender) Send(ctx context.Context, to, subject, attachmentPath, attachmentName string) error {
	if s.cfg.Host == "" || s.cfg.Port == 0 {
		return errors.New("smtp host/port missing")
	}
	if to == "" {
		return errors.New("recipient required")
	}
	from := s.cfg.From
	if from == "" {
		from = s.cfg.Username
	}
	m := gomail.NewMessage()
	m.SetHeader("From", from)
	// SetAddressHeader (not SetHeader) so a malformed/crafted recipient can't
	// inject extra SMTP headers via CR/LF — defense in depth behind the
	// handler's validateKindleAddress check.
	m.SetAddressHeader("To", to, "")
	if subject == "" {
		subject = "Your Continuum ebook"
	}
	m.SetHeader("Subject", subject)
	m.SetBody("text/plain", "Sent from Continuum.")
	if attachmentPath != "" {
		opts := []gomail.FileSetting{}
		if attachmentName != "" {
			opts = append(opts, gomail.Rename(attachmentName))
		}
		m.Attach(attachmentPath, opts...)
	}
	d := s.dialer(s.cfg.Host, s.cfg.Port, s.cfg.Username, s.cfg.Password)
	// Context cancellation is best-effort: gomail.v2 doesn't accept a
	// context, but the caller can wrap the call in a goroutine if needed.
	errCh := make(chan error, 1)
	go func() { errCh <- d.DialAndSend(m) }()
	select {
	case <-ctx.Done():
		return fmt.Errorf("context cancelled: %w", ctx.Err())
	case err := <-errCh:
		return err
	}
}
