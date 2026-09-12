package api

import (
	"math"
	"net/http"
	"strconv"
)

// PaginationMeta holds metadata for offset-based pagination.
type PaginationMeta struct {
	Page       int   `json:"page,omitempty"`
	PerPage    int   `json:"per_page,omitempty"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages,omitempty"`
	HasNext    bool  `json:"has_next"`
	HasPrev    bool  `json:"has_prev"`
}

// CursorMeta holds metadata for cursor-based pagination.
type CursorMeta struct {
	PerPage    int    `json:"per_page"`
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// OffsetPagination represents parsed offset-based pagination parameters.
type OffsetPagination struct {
	Page    int
	PerPage int
}

// Offset returns the SQL/database offset for this pagination.
func (p OffsetPagination) Offset() int {
	if p.Page < 1 {
		return 0
	}
	return (p.Page - 1) * p.PerPage
}

// Limit returns the SQL/database limit for this pagination.
func (p OffsetPagination) Limit() int {
	return p.PerPage
}

// Meta builds a PaginationMeta from the total record count.
func (p OffsetPagination) Meta(total int64) *PaginationMeta {
	totalPages := 0
	if p.PerPage > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(p.PerPage)))
	}
	return &PaginationMeta{
		Page:       p.Page,
		PerPage:    p.PerPage,
		Total:      total,
		TotalPages: totalPages,
		HasNext:    p.Page < totalPages,
		HasPrev:    p.Page > 1,
	}
}

// CursorPagination represents parsed cursor-based pagination parameters.
type CursorPagination struct {
	Cursor  string
	PerPage int
}

// Limit returns the SQL/database limit for this pagination.
// One extra row is fetched to determine hasMore.
func (c CursorPagination) Limit() int {
	return c.PerPage + 1
}

// Meta builds a CursorMeta from the next cursor and whether more rows exist.
func (c CursorPagination) Meta(nextCursor string, hasMore bool) *CursorMeta {
	return &CursorMeta{
		PerPage:    c.PerPage,
		HasMore:    hasMore,
		NextCursor: nextCursor,
	}
}

// ParseOffsetPagination extracts page and per_page query parameters from
// the request. If page is absent or invalid it defaults to 1. If per_page
// is absent or invalid it defaults to defaultPerPage. PerPage is clamped
// to maxPerPage.
func ParseOffsetPagination(r *http.Request, defaultPerPage, maxPerPage int) OffsetPagination {
	page := 1
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}

	perPage := defaultPerPage
	if v := r.URL.Query().Get("per_page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			perPage = n
		}
	}
	if maxPerPage > 0 && perPage > maxPerPage {
		perPage = maxPerPage
	}

	return OffsetPagination{Page: page, PerPage: perPage}
}

// ParseCursorPagination extracts cursor and per_page query parameters from
// the request. If cursor is absent it defaults to an empty string (first
// page). If per_page is absent or invalid it defaults to defaultPerPage.
// PerPage is clamped to maxPerPage.
func ParseCursorPagination(r *http.Request, defaultPerPage, maxPerPage int) CursorPagination {
	perPage := defaultPerPage
	if v := r.URL.Query().Get("per_page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			perPage = n
		}
	}
	if maxPerPage > 0 && perPage > maxPerPage {
		perPage = maxPerPage
	}

	return CursorPagination{
		Cursor:  r.URL.Query().Get("cursor"),
		PerPage: perPage,
	}
}
