// Package mediatoken mints HS256 JWTs the ebooks portal embeds in cover and
// file URLs. Backends verify with the shared secret (their
// stream_signing_secret), so a token leaked from a URL only grants access to
// one book/file for a short window, instead of the user's full bearer.
package mediatoken

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Audience matches the value backend plugins verify against. Distinct from
// the audiobook audience so tokens can't cross media types.
const Audience = "ebook_backend"

// CoverFileIdx is the sentinel claim value for cover tokens.
const CoverFileIdx = -1

// FileFileIdx is the claim value for an ebook's primary file (single-file
// per book in the current ebook contract).
const FileFileIdx = 0

// DefaultTTL is the time-to-live for minted media tokens. Short enough that
// a leaked URL stops working quickly; long enough that the SPA can hand the
// URL to the browser and the browser can fetch + cache + retry without
// race-condition failures.
const DefaultTTL = 15 * time.Minute

// ErrSecretUnconfigured is returned when minting is attempted with an empty
// signing secret — caller should treat this as misconfigured.
var ErrSecretUnconfigured = errors.New("media signing secret not configured")

// Mint produces a signed token bound to userID + bookID + fileIdx.
func Mint(secret, userID, bookID string, fileIdx int) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", ErrSecretUnconfigured
	}
	if userID == "" {
		return "", errors.New("userID required")
	}
	if bookID == "" {
		return "", errors.New("bookID required")
	}
	key := decodeSecret(secret)
	now := time.Now()
	claims := jwt.MapClaims{
		"aud":      Audience,
		"sub":      userID,
		"book_id":  bookID,
		"file_idx": fileIdx,
		"iat":      now.Unix(),
		"exp":      now.Add(DefaultTTL).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(key)
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}
	return signed, nil
}

func decodeSecret(secret string) []byte {
	if b, err := base64.StdEncoding.DecodeString(secret); err == nil && len(b) > 0 {
		return b
	}
	if b, err := base64.RawStdEncoding.DecodeString(secret); err == nil && len(b) > 0 {
		return b
	}
	return []byte(secret)
}
