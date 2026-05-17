package server

import "testing"

func TestValidateKindleAddress(t *testing.T) {
	good := []string{
		"alice@kindle.com",
		"Bob.Reader+tag@kindle.com",
		"x@kindle.cn",
		"y@free.kindle.com",
		"  spaced@kindle.com  ",
	}
	for _, in := range good {
		if got, err := validateKindleAddress(in); err != nil {
			t.Errorf("validateKindleAddress(%q) unexpected error: %v", in, err)
		} else if got == "" {
			t.Errorf("validateKindleAddress(%q) returned empty address", in)
		}
	}

	bad := []string{
		"",
		"not-an-email",
		"attacker@evil.com",                          // open-relay: non-kindle domain
		"a@kindle.com.evil.com",                      // suffix trick
		"a@kindle.com\r\nBcc: victim@evil.com",       // CRLF header injection
		"a@kindle.com\nSubject: x",                   // LF header injection
		"Name <a@kindle.com>",                        // display-name form not allowed
		"a@kindle.com, b@kindle.com",                 // multiple recipients
		"@kindle.com",
	}
	for _, in := range bad {
		if got, err := validateKindleAddress(in); err == nil {
			t.Errorf("validateKindleAddress(%q) = %q, want error", in, got)
		}
	}
}
