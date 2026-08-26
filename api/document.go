package api

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Document represents a top-level OpenAPI 3.1 document.
type Document struct {
	OpenAPI    string      `json:"openapi" yaml:"openapi"`
	Info       Info        `json:"info" yaml:"info"`
	Servers    []Server    `json:"servers,omitempty" yaml:"servers,omitempty"`
	Paths      Paths       `json:"paths" yaml:"paths"`
	Components *Components `json:"components,omitempty" yaml:"components,omitempty"`
}

// Info describes the API metadata.
type Info struct {
	Title          string   `json:"title" yaml:"title"`
	Version        string   `json:"version" yaml:"version"`
	Description    string   `json:"description,omitempty" yaml:"description,omitempty"`
	TermsOfService string   `json:"termsOfService,omitempty" yaml:"termsOfService,omitempty"`
	License        *License `json:"license,omitempty" yaml:"license,omitempty"`
}

// License describes the API license.
type License struct {
	Name string `json:"name" yaml:"name"`
	URL  string `json:"url,omitempty" yaml:"url,omitempty"`
}

// Server describes a server endpoint.
type Server struct {
	URL         string `json:"url" yaml:"url"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// Paths maps path patterns to their PathItem.
type Paths map[string]*PathItem

// PathItem describes the operations available on a single path.
type PathItem struct {
	Summary     string      `json:"summary,omitempty" yaml:"summary,omitempty"`
	Description string      `json:"description,omitempty" yaml:"description,omitempty"`
	Get         *Operation  `json:"get,omitempty" yaml:"get,omitempty"`
	Post        *Operation  `json:"post,omitempty" yaml:"post,omitempty"`
	Put         *Operation  `json:"put,omitempty" yaml:"put,omitempty"`
	Delete      *Operation  `json:"delete,omitempty" yaml:"delete,omitempty"`
	Patch       *Operation  `json:"patch,omitempty" yaml:"patch,omitempty"`
	Parameters  []Parameter `json:"parameters,omitempty" yaml:"parameters,omitempty"`
}

// Operation describes a single API operation.
type Operation struct {
	Tags        []string              `json:"tags,omitempty" yaml:"tags,omitempty"`
	Summary     string                `json:"summary,omitempty" yaml:"summary,omitempty"`
	Description string                `json:"description,omitempty" yaml:"description,omitempty"`
	OperationID string                `json:"operationId,omitempty" yaml:"operationId,omitempty"`
	Parameters  []Parameter           `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	RequestBody *RequestBody          `json:"requestBody,omitempty" yaml:"requestBody,omitempty"`
	Responses   map[int]*SpecResponse `json:"responses" yaml:"responses"`
}

// Parameter describes a single operation parameter.
type Parameter struct {
	Name        string `json:"name" yaml:"name"`
	In          string `json:"in" yaml:"in"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Required    bool   `json:"required,omitempty" yaml:"required,omitempty"`
	Schema      Schema `json:"schema,omitempty" yaml:"schema,omitempty"`
}

// RequestBody describes the body of a request.
type RequestBody struct {
	Description string               `json:"description,omitempty" yaml:"description,omitempty"`
	Required    bool                 `json:"required,omitempty" yaml:"required,omitempty"`
	Content     map[string]MediaType `json:"content" yaml:"content"`
}

// SpecResponse describes a single response code in an OpenAPI operation.
type SpecResponse struct {
	Description string               `json:"description" yaml:"description"`
	Content     map[string]MediaType `json:"content,omitempty" yaml:"content,omitempty"`
}

// MediaType describes a media type (e.g. application/json) and its schema.
type MediaType struct {
	Schema Schema `json:"schema,omitempty" yaml:"schema,omitempty"`
}

// Components holds reusable schemas and security schemes.
type Components struct {
	Schemas         map[string]Schema      `json:"schemas,omitempty" yaml:"schemas,omitempty"`
	SecuritySchemes map[string]interface{} `json:"securitySchemes,omitempty" yaml:"securitySchemes,omitempty"`
}

// NewDocument creates a new OpenAPI 3.1 document with sensible defaults.
func NewDocument(title, version string) *Document {
	return &Document{
		OpenAPI: "3.1.0",
		Info: Info{
			Title:   title,
			Version: version,
		},
		Paths: Paths{},
	}
}

// Description sets the API description.
func (d *Document) Description(s string) *Document {
	d.Info.Description = s
	return d
}

// TermsOfService sets the API terms of service URL.
func (d *Document) TermsOfService(url string) *Document {
	d.Info.TermsOfService = url
	return d
}

// License sets the API license.
func (d *Document) License(name, url string) *Document {
	d.Info.License = &License{Name: name, URL: url}
	return d
}

// Server adds a server endpoint to the document.
func (d *Document) Server(url, description string) *Document {
	d.Servers = append(d.Servers, Server{URL: url, Description: description})
	return d
}

// Path registers a path item under the given path pattern.
func (d *Document) Path(pattern string, item *PathItem) *Document {
	d.Paths[pattern] = item
	return d
}

// NewPathItem returns a fresh PathItem for building.
func NewPathItem() *PathItem {
	return &PathItem{}
}

// WithGet adds a GET operation to the path item.
func (p *PathItem) WithGet(op *Operation) *PathItem {
	p.Get = op
	return p
}

// WithPost adds a POST operation to the path item.
func (p *PathItem) WithPost(op *Operation) *PathItem {
	p.Post = op
	return p
}

// WithPut adds a PUT operation to the path item.
func (p *PathItem) WithPut(op *Operation) *PathItem {
	p.Put = op
	return p
}

// WithDelete adds a DELETE operation to the path item.
func (p *PathItem) WithDelete(op *Operation) *PathItem {
	p.Delete = op
	return p
}

// WithPatch adds a PATCH operation to the path item.
func (p *PathItem) WithPatch(op *Operation) *PathItem {
	p.Patch = op
	return p
}

// NewOperation creates a new operation with the given tag and summary.
func NewOperation(tag, summary string) *Operation {
	return &Operation{
		Tags:      []string{tag},
		Summary:   summary,
		Responses: map[int]*SpecResponse{},
	}
}

// WithDescription sets the operation description.
func (o *Operation) WithDescription(s string) *Operation {
	o.Description = s
	return o
}

// WithOperationID sets the operation ID.
func (o *Operation) WithOperationID(id string) *Operation {
	o.OperationID = id
	return o
}

// Param adds a parameter to the operation.
func (o *Operation) Param(name, in, description string, required bool, schema Schema) *Operation {
	o.Parameters = append(o.Parameters, Parameter{
		Name:        name,
		In:          in,
		Description: description,
		Required:    required,
		Schema:      schema,
	})
	return o
}

// JSONBody sets a JSON request body on the operation.
func (o *Operation) JSONBody(description string, required bool, schema Schema) *Operation {
	o.RequestBody = &RequestBody{
		Description: description,
		Required:    required,
		Content: map[string]MediaType{
			"application/json": {Schema: schema},
		},
	}
	return o
}

// JSONResponse adds a JSON response with the given status code.
func (o *Operation) JSONResponse(status int, description string, schema Schema) *Operation {
	o.Responses[status] = &SpecResponse{
		Description: description,
		Content: map[string]MediaType{
			"application/json": {Schema: schema},
		},
	}
	return o
}

// PlainResponse adds a response with the given status code and description
// but no content schema.
func (o *Operation) PlainResponse(status int, description string) *Operation {
	o.Responses[status] = &SpecResponse{
		Description: description,
	}
	return o
}

// AddSchema registers a reusable component schema by name.
func (d *Document) AddSchema(name string, schema Schema) *Document {
	if d.Components == nil {
		d.Components = &Components{}
	}
	if d.Components.Schemas == nil {
		d.Components.Schemas = map[string]Schema{}
	}
	d.Components.Schemas[name] = schema
	return d
}

// AddSecurityScheme registers a reusable security scheme by name.
func (d *Document) AddSecurityScheme(name string, scheme map[string]interface{}) *Document {
	if d.Components == nil {
		d.Components = &Components{}
	}
	if d.Components.SecuritySchemes == nil {
		d.Components.SecuritySchemes = map[string]interface{}{}
	}
	d.Components.SecuritySchemes[name] = scheme
	return d
}

// MergePaths merges all paths and component schemas from src into d.
// This is useful when building a single OpenAPI document from multiple
// handler-level documents.
func (d *Document) MergePaths(src *Document) *Document {
	if src == nil {
		return d
	}
	for pattern, item := range src.Paths {
		d.Paths[pattern] = item
	}
	if src.Components != nil {
		if d.Components == nil {
			d.Components = &Components{}
		}
		if d.Components.Schemas == nil {
			d.Components.Schemas = map[string]Schema{}
		}
		for name, schema := range src.Components.Schemas {
			d.Components.Schemas[name] = schema
		}
		if src.Components.SecuritySchemes != nil {
			if d.Components.SecuritySchemes == nil {
				d.Components.SecuritySchemes = map[string]interface{}{}
			}
			for name, scheme := range src.Components.SecuritySchemes {
				d.Components.SecuritySchemes[name] = scheme
			}
		}
	}
	return d
}

// SchemaFrom reflects a Go struct into a JSON Schema and returns it.
// It is a convenience wrapper around SchemaFromStruct.
func (d *Document) SchemaFrom(v interface{}) Schema {
	return SchemaFromStruct(v)
}

// MarshalJSON returns the document as JSON.
func (d *Document) MarshalJSON() ([]byte, error) {
	type alias Document
	return json.MarshalIndent((*alias)(d), "", "  ")
}

// MarshalYAML returns the document as YAML.
func (d *Document) MarshalYAML() ([]byte, error) {
	out, err := json.Marshal(d)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal document for YAML conversion: %w", err)
	}
	return jsonToYAML(out)
}

// jsonToYAML converts a JSON byte slice to YAML by first unmarshaling into
// an interface{} and then marshaling back as YAML. Since our structs use
// json struct tags, we go through JSON first to ensure field names are
// respected in the YAML output.
func jsonToYAML(jsonBytes []byte) ([]byte, error) {
	var v interface{}
	dec := json.NewDecoder(strings.NewReader(string(jsonBytes)))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("failed to decode JSON for YAML conversion: %w", err)
	}

	yamlBytes, err := yaml.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal YAML: %w", err)
	}
	return yamlBytes, nil
}

// LoadSpecFile reads a static OpenAPI spec file from disk and returns its
// raw content along with the appropriate content type based on the file
// extension (.json -> application/json, .yaml/.yml -> application/yaml).
func LoadSpecFile(path string) ([]byte, string, error) {
	content, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, "", fmt.Errorf("failed to read spec file: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		return content, "application/json", nil
	case ".yaml", ".yml":
		return content, "application/yaml", nil
	default:
		return nil, "", fmt.Errorf("unsupported spec file extension: %s (use .json, .yaml, or .yml)", ext)
	}
}
