package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnnotationRoutesPreserveReadestFields(t *testing.T) {
	srv := newReaderConfigTestServer(t)
	handler := srv.Handler()

	body := []byte(`{
		"cfi_range":"epubcfi(/6/8!/4/2,/1:0,/1:12)",
		"kind":"annotation",
		"color":"yellow",
		"selected_text":"selected",
		"note_text":"reader note",
		"readest_type":"annotation",
		"xpointer0":"/html/body/p[1]",
		"xpointer1":"/html/body/p[1]/text()[1]",
		"page":42,
		"style":"squiggly",
		"metadata_json":{"source":"readest"}
	}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/me/books/book-1/annotations", bytes.NewReader(body))
	req.Header.Set("X-Silo-User-Id", "user-1")
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/me/books/book-1/annotations", nil)
	req.Header.Set("X-Silo-User-Id", "user-1")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rec.Code, rec.Body.String())
	}
	var listed struct {
		Items []struct {
			ID           string         `json:"id"`
			CFIRange     string         `json:"cfi_range"`
			Kind         string         `json:"kind"`
			ReadestType  string         `json:"readest_type"`
			Color        string         `json:"color"`
			SelectedText string         `json:"selected_text"`
			NoteText     string         `json:"note_text"`
			XPointer0    string         `json:"xpointer0"`
			XPointer1    string         `json:"xpointer1"`
			Page         int            `json:"page"`
			Style        string         `json:"style"`
			MetadataJSON map[string]any `json:"metadata_json"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed.Items) != 1 {
		t.Fatalf("items=%d body=%s", len(listed.Items), rec.Body.String())
	}
	got := listed.Items[0]
	if got.Kind != "annotation" || got.ReadestType != "annotation" || got.Style != "squiggly" || got.Page != 42 {
		t.Fatalf("readest fields not preserved: %+v", got)
	}
	if got.XPointer0 != "/html/body/p[1]" || got.XPointer1 != "/html/body/p[1]/text()[1]" {
		t.Fatalf("xpointer fields not preserved: %+v", got)
	}
	if got.MetadataJSON["source"] != "readest" {
		t.Fatalf("metadata_json not preserved: %+v", got.MetadataJSON)
	}

	update := []byte(`{
		"cfi_range":"epubcfi(/6/10!/4/2,/1:0,/1:8)",
		"color":"blue",
		"selected_text":"new selected text",
		"note_text":"updated reader note",
		"style":"underline"
	}`)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/me/annotations/"+listed.Items[0].ID, bytes.NewReader(update))
	req.Header.Set("X-Silo-User-Id", "user-1")
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/me/books/book-1/annotations", nil)
	req.Header.Set("X-Silo-User-Id", "user-1")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list after update status=%d body=%s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode updated list: %v", err)
	}
	got = listed.Items[0]
	if got.CFIRange != "epubcfi(/6/10!/4/2,/1:0,/1:8)" || got.SelectedText != "new selected text" {
		t.Fatalf("updated range/text not preserved: %+v", got)
	}
	if got.Color != "blue" || got.Style != "underline" || got.NoteText != "updated reader note" {
		t.Fatalf("updated style fields not preserved: %+v", got)
	}
}
