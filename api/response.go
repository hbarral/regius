package api

// Response is the standard API response envelope. Successful responses
// populate Data (and optionally Meta); error responses populate Error.
// At most one of Data or Error should be non-nil.
type Response struct {
	Data  interface{} `json:"data,omitempty"`
	Error *Error      `json:"error,omitempty"`
	Meta  *Meta       `json:"meta,omitempty"`
}

// Error describes an API error in the response envelope.
type Error struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// Meta carries response metadata such as pagination info or a total count.
type Meta struct {
	Pagination *PaginationMeta `json:"pagination,omitempty"`
	Cursor     *CursorMeta     `json:"cursor,omitempty"`
	Total      int64           `json:"total,omitempty"`
}

// NewResponse creates a successful response envelope with the given data.
func NewResponse(data interface{}) *Response {
	return &Response{Data: data}
}

// NewErrorResponse creates an error response envelope with the given code
// and message.
func NewErrorResponse(code, message string) *Response {
	return &Response{Error: &Error{Code: code, Message: message}}
}

// NewErrorResponseWithDetails creates an error response envelope with the
// given code, message, and details.
func NewErrorResponseWithDetails(code, message string, details interface{}) *Response {
	return &Response{Error: &Error{Code: code, Message: message, Details: details}}
}

// WithMeta attaches metadata to the response and returns the response for
// chaining.
func (r *Response) WithMeta(meta *Meta) *Response {
	r.Meta = meta
	return r
}

// WithPagination attaches offset-based pagination metadata to the response.
func (r *Response) WithPagination(pm *PaginationMeta) *Response {
	if r.Meta == nil {
		r.Meta = &Meta{}
	}
	r.Meta.Pagination = pm
	return r
}

// WithCursor attaches cursor-based pagination metadata to the response.
func (r *Response) WithCursor(cm *CursorMeta) *Response {
	if r.Meta == nil {
		r.Meta = &Meta{}
	}
	r.Meta.Cursor = cm
	return r
}

// WithTotal attaches a total count to the response metadata.
func (r *Response) WithTotal(total int64) *Response {
	if r.Meta == nil {
		r.Meta = &Meta{}
	}
	r.Meta.Total = total
	return r
}
