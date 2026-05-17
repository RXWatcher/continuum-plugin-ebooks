package runtime

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestConfigRedaction(t *testing.T) {
	cfg := Config{
		DatabaseURL:      "postgres://u:sup3rsecret@db/x",
		KindleSMTPConfig: json.RawMessage(`{"password":"SMTPSECRET"}`),
		OpdsRealm:        "MyLibrary",
	}
	if s := cfg.String(); strings.Contains(s, "sup3rsecret") || strings.Contains(s, "SMTPSECRET") {
		t.Fatalf("String leaked a secret: %s", s)
	}
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("cfg", "config", cfg)
	out := buf.String()
	if strings.Contains(out, "sup3rsecret") || strings.Contains(out, "SMTPSECRET") {
		t.Fatalf("slog leaked a secret: %s", out)
	}
	if !strings.Contains(out, "MyLibrary") {
		t.Fatalf("redaction hid the non-secret realm: %s", out)
	}
}

func TestSnapshot_RawMessageIsolated(t *testing.T) {
	s := New(nil, func(Config) error { return nil })
	s.mu.Lock()
	s.cfg = Config{
		KindleSMTPConfig: json.RawMessage(`{"a":1}`),
		PathRemappings:   json.RawMessage(`[{"x":1}]`),
	}
	s.mu.Unlock()

	snap := s.Snapshot()
	snap.KindleSMTPConfig[0] = 'X'
	snap.PathRemappings[0] = 'X'

	again := s.Snapshot()
	if again.KindleSMTPConfig[0] != '{' || again.PathRemappings[0] != '[' {
		t.Fatalf("Snapshot aliases backing arrays: smtp=%s remap=%s",
			again.KindleSMTPConfig, again.PathRemappings)
	}
}
