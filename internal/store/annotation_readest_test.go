package store_test

import (
	"context"
	"testing"

	"github.com/RXWatcher/silo-plugin-ebooks/internal/store"
)

func TestAnnotationReadestFieldsRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	page := 42

	if err := s.InsertAnnotation(ctx, store.Annotation{
		ID:           "readest-note-1",
		UserID:       "u-readest",
		BookID:       "b-readest",
		CFIRange:     "epubcfi(/6/8!/4/2,/1:0,/1:12)",
		Kind:         "annotation",
		Color:        "yellow",
		SelectedText: "selected",
		NoteText:     "reader note",
		ReadestType:  "annotation",
		XPointer0:    "/html/body/p[1]",
		XPointer1:    "/html/body/p[1]/text()[1]",
		Page:         &page,
		Style:        "underline",
		MetadataJSON: []byte(`{"source":"readest"}`),
	}); err != nil {
		t.Fatalf("InsertAnnotation: %v", err)
	}

	got, err := s.ListAnnotationsByBook(ctx, "u-readest", "b-readest")
	if err != nil {
		t.Fatalf("ListAnnotationsByBook: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("annotation count = %d", len(got))
	}
	ann := got[0]
	if ann.ReadestType != "annotation" || ann.XPointer0 != "/html/body/p[1]" || ann.XPointer1 != "/html/body/p[1]/text()[1]" {
		t.Fatalf("xpointer/readest fields not preserved: %+v", ann)
	}
	if ann.Page == nil || *ann.Page != page {
		t.Fatalf("page not preserved: %+v", ann.Page)
	}
	if ann.Style != "underline" {
		t.Fatalf("style = %q", ann.Style)
	}
	assertJSONEqual(t, []byte(`{"source":"readest"}`), ann.MetadataJSON)
}

func TestUpdateAnnotationCanMoveReadestRange(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.InsertAnnotation(ctx, store.Annotation{
		ID:           "range-edit-1",
		UserID:       "u-readest",
		BookID:       "b-readest",
		CFIRange:     "epubcfi(/6/8!/4/2,/1:0,/1:12)",
		Kind:         "annotation",
		Color:        "yellow",
		SelectedText: "old selection",
		NoteText:     "reader note",
		ReadestType:  "annotation",
		Style:        "highlight",
		MetadataJSON: []byte(`{}`),
	}); err != nil {
		t.Fatalf("InsertAnnotation: %v", err)
	}

	if err := s.UpdateAnnotation(ctx, "range-edit-1", "u-readest", store.Annotation{
		CFIRange:     "epubcfi(/6/10!/4/2,/1:0,/1:8)",
		Color:        "blue",
		SelectedText: "new text",
		NoteText:     "updated note",
		Style:        "squiggly",
	}); err != nil {
		t.Fatalf("UpdateAnnotation: %v", err)
	}

	got, err := s.ListAnnotationsByBook(ctx, "u-readest", "b-readest")
	if err != nil {
		t.Fatalf("ListAnnotationsByBook: %v", err)
	}
	ann := got[0]
	if ann.CFIRange != "epubcfi(/6/10!/4/2,/1:0,/1:8)" || ann.SelectedText != "new text" {
		t.Fatalf("range/text not updated: %+v", ann)
	}
	if ann.Color != "blue" || ann.Style != "squiggly" || ann.NoteText != "updated note" {
		t.Fatalf("style fields not updated: %+v", ann)
	}
}
