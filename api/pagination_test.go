package api

import (
	"net/http/httptest"
	"testing"
)

func TestOffsetPaginationOffset(t *testing.T) {
	tests := []struct {
		name    string
		page    int
		perPage int
		wantOff int
		wantLim int
	}{
		{"page 1", 1, 20, 0, 20},
		{"page 2", 2, 20, 20, 20},
		{"page 3", 3, 10, 20, 10},
		{"page 0 (invalid)", 0, 20, 0, 20},
		{"page negative", -1, 20, 0, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := OffsetPagination{Page: tt.page, PerPage: tt.perPage}
			if p.Offset() != tt.wantOff {
				t.Errorf("expected offset %d, got %d", tt.wantOff, p.Offset())
			}
			if p.Limit() != tt.wantLim {
				t.Errorf("expected limit %d, got %d", tt.wantLim, p.Limit())
			}
		})
	}
}

func TestOffsetPaginationMeta(t *testing.T) {
	tests := []struct {
		name        string
		page        int
		perPage     int
		total       int64
		wantTotal   int
		wantHasNext bool
		wantHasPrev bool
	}{
		{"page 1 of 5", 1, 20, 100, 5, true, false},
		{"page 3 of 5", 3, 20, 100, 5, true, true},
		{"page 5 of 5 (last)", 5, 20, 100, 5, false, true},
		{"page 6 beyond total", 6, 20, 100, 5, false, true},
		{"zero results", 1, 20, 0, 0, false, false},
		{"partial last page", 3, 20, 50, 3, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := OffsetPagination{Page: tt.page, PerPage: tt.perPage}
			meta := p.Meta(tt.total)
			if meta.Total != tt.total {
				t.Errorf("expected total %d, got %d", tt.total, meta.Total)
			}
			if meta.TotalPages != tt.wantTotal {
				t.Errorf("expected total_pages %d, got %d", tt.wantTotal, meta.TotalPages)
			}
			if meta.HasNext != tt.wantHasNext {
				t.Errorf("expected has_next %v, got %v", tt.wantHasNext, meta.HasNext)
			}
			if meta.HasPrev != tt.wantHasPrev {
				t.Errorf("expected has_prev %v, got %v", tt.wantHasPrev, meta.HasPrev)
			}
		})
	}
}

func TestCursorPaginationLimit(t *testing.T) {
	c := CursorPagination{Cursor: "abc", PerPage: 20}
	if c.Limit() != 21 {
		t.Errorf("expected limit 21 (perPage + 1), got %d", c.Limit())
	}
}

func TestCursorPaginationMeta(t *testing.T) {
	c := CursorPagination{Cursor: "abc", PerPage: 20}
	meta := c.Meta("def", true)
	if meta.PerPage != 20 {
		t.Errorf("expected per_page 20, got %d", meta.PerPage)
	}
	if !meta.HasMore {
		t.Error("expected has_more true")
	}
	if meta.NextCursor != "def" {
		t.Errorf("expected next_cursor 'def', got %s", meta.NextCursor)
	}

	meta = c.Meta("", false)
	if meta.HasMore {
		t.Error("expected has_more false")
	}
	if meta.NextCursor != "" {
		t.Errorf("expected empty next_cursor, got %s", meta.NextCursor)
	}
}

func TestParseOffsetPagination(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		defaultPerPage int
		maxPerPage     int
		wantPage       int
		wantPerPage    int
	}{
		{"no params", "", 20, 100, 1, 20},
		{"valid page and per_page", "?page=3&per_page=10", 20, 100, 3, 10},
		{"invalid page", "?page=abc", 20, 100, 1, 20},
		{"zero page", "?page=0", 20, 100, 1, 20},
		{"negative page", "?page=-1", 20, 100, 1, 20},
		{"invalid per_page", "?per_page=abc", 20, 100, 1, 20},
		{"zero per_page", "?per_page=0", 20, 100, 1, 20},
		{"per_page exceeds max", "?per_page=200", 20, 100, 1, 100},
		{"no max enforcement", "?per_page=50", 20, 0, 1, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/items"+tt.query, nil)
			p := ParseOffsetPagination(req, tt.defaultPerPage, tt.maxPerPage)
			if p.Page != tt.wantPage {
				t.Errorf("expected page %d, got %d", tt.wantPage, p.Page)
			}
			if p.PerPage != tt.wantPerPage {
				t.Errorf("expected per_page %d, got %d", tt.wantPerPage, p.PerPage)
			}
		})
	}
}

func TestParseCursorPagination(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		defaultPerPage int
		maxPerPage     int
		wantCursor     string
		wantPerPage    int
	}{
		{"no params", "", 20, 100, "", 20},
		{"with cursor", "?cursor=abc123", 20, 100, "abc123", 20},
		{"valid per_page", "?per_page=10", 20, 100, "", 10},
		{"invalid per_page", "?per_page=abc", 20, 100, "", 20},
		{"per_page exceeds max", "?per_page=200", 20, 100, "", 100},
		{"both cursor and per_page", "?cursor=xyz&per_page=5", 20, 100, "xyz", 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/items"+tt.query, nil)
			c := ParseCursorPagination(req, tt.defaultPerPage, tt.maxPerPage)
			if c.Cursor != tt.wantCursor {
				t.Errorf("expected cursor %q, got %q", tt.wantCursor, c.Cursor)
			}
			if c.PerPage != tt.wantPerPage {
				t.Errorf("expected per_page %d, got %d", tt.wantPerPage, c.PerPage)
			}
		})
	}
}
