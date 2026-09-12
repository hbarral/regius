package api

import (
	"encoding/json"
	"testing"
)

func TestNewResponse(t *testing.T) {
	resp := NewResponse("hello")
	if resp.Data != "hello" {
		t.Errorf("expected data 'hello', got %v", resp.Data)
	}
	if resp.Error != nil {
		t.Error("expected nil error")
	}
}

func TestNewErrorResponse(t *testing.T) {
	resp := NewErrorResponse("not_found", "resource not found")
	if resp.Error == nil {
		t.Fatal("expected non-nil error")
	}
	if resp.Error.Code != "not_found" {
		t.Errorf("expected code 'not_found', got %s", resp.Error.Code)
	}
	if resp.Error.Message != "resource not found" {
		t.Errorf("expected message 'resource not found', got %s", resp.Error.Message)
	}
	if resp.Data != nil {
		t.Error("expected nil data for error response")
	}
}

func TestNewErrorResponseWithDetails(t *testing.T) {
	details := map[string]interface{}{"field": "email", "issue": "invalid format"}
	resp := NewErrorResponseWithDetails("validation", "validation failed", details)
	if resp.Error.Details == nil {
		t.Fatal("expected non-nil details")
	}
}

func TestResponseWithMeta(t *testing.T) {
	meta := &Meta{Total: 100}
	resp := NewResponse("data").WithMeta(meta)
	if resp.Meta == nil || resp.Meta.Total != 100 {
		t.Error("expected meta with total 100")
	}
}

func TestResponseWithPagination(t *testing.T) {
	pm := &PaginationMeta{Page: 2, PerPage: 20, Total: 100, TotalPages: 5, HasNext: true, HasPrev: true}
	resp := NewResponse("data").WithPagination(pm)
	if resp.Meta == nil || resp.Meta.Pagination == nil {
		t.Fatal("expected pagination meta")
	}
	if resp.Meta.Pagination.Page != 2 {
		t.Errorf("expected page 2, got %d", resp.Meta.Pagination.Page)
	}
}

func TestResponseWithCursor(t *testing.T) {
	cm := &CursorMeta{PerPage: 20, HasMore: true, NextCursor: "abc123"}
	resp := NewResponse("data").WithCursor(cm)
	if resp.Meta == nil || resp.Meta.Cursor == nil {
		t.Fatal("expected cursor meta")
	}
	if resp.Meta.Cursor.NextCursor != "abc123" {
		t.Errorf("expected next_cursor 'abc123', got %s", resp.Meta.Cursor.NextCursor)
	}
}

func TestResponseWithTotal(t *testing.T) {
	resp := NewResponse("data").WithTotal(42)
	if resp.Meta == nil || resp.Meta.Total != 42 {
		t.Error("expected meta with total 42")
	}
}

func TestResponseJSONSerialization(t *testing.T) {
	t.Run("success response", func(t *testing.T) {
		resp := NewResponse("hello").WithPagination(&PaginationMeta{
			Page: 1, PerPage: 10, Total: 50, TotalPages: 5, HasNext: true, HasPrev: false,
		})
		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if parsed["data"] != "hello" {
			t.Errorf("expected data 'hello', got %v", parsed["data"])
		}
		if parsed["error"] != nil {
			t.Error("expected no error field")
		}
		meta, ok := parsed["meta"].(map[string]interface{})
		if !ok {
			t.Fatal("expected meta object")
		}
		pagination, ok := meta["pagination"].(map[string]interface{})
		if !ok {
			t.Fatal("expected pagination object")
		}
		if pagination["total"] != float64(50) {
			t.Errorf("expected total 50, got %v", pagination["total"])
		}
	})

	t.Run("error response", func(t *testing.T) {
		resp := NewErrorResponse("not_found", "resource not found")
		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if parsed["data"] != nil {
			t.Error("expected no data field")
		}
		errObj, ok := parsed["error"].(map[string]interface{})
		if !ok {
			t.Fatal("expected error object")
		}
		if errObj["code"] != "not_found" {
			t.Errorf("expected code 'not_found', got %v", errObj["code"])
		}
	})
}
