package server

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestWriteItemsEncodesNilSliceAsEmptyArray(t *testing.T) {
	rec := httptest.NewRecorder()

	var rows []string
	writeItems(rec, 200, rows)

	var body struct {
		Items []string `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Items == nil {
		t.Fatal("items encoded as null, want empty array")
	}
	if len(body.Items) != 0 {
		t.Fatalf("items length = %d, want 0", len(body.Items))
	}
}
