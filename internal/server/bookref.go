package server

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

func encodeBookRef(libraryID int64, backendBookID string) string {
	if libraryID <= 0 {
		return backendBookID
	}
	return fmt.Sprintf("%d:%s", libraryID, base64.RawURLEncoding.EncodeToString([]byte(backendBookID)))
}

func decodeBookRef(ref string) (int64, string, bool) {
	left, right, ok := strings.Cut(ref, ":")
	if !ok {
		return 0, ref, false
	}
	id, err := strconv.ParseInt(left, 10, 64)
	if err != nil || id <= 0 {
		return 0, ref, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(right)
	if err != nil {
		return 0, ref, false
	}
	return id, string(raw), true
}
