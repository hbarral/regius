package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewDocument(t *testing.T) {
	doc := NewDocument("My API", "1.0.0")

	if doc.OpenAPI != "3.1.0" {
		t.Errorf("expected OpenAPI 3.1.0, got %s", doc.OpenAPI)
	}
	if doc.Info.Title != "My API" {
		t.Errorf("expected title 'My API', got %s", doc.Info.Title)
	}
	if doc.Info.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %s", doc.Info.Version)
	}
	if doc.Paths == nil {
		t.Error("expected non-nil Paths map")
	}
}

func TestDocumentBuilder(t *testing.T) {
	doc := NewDocument("Test API", "2.0.0").
		Description("A test API").
		Server("https://api.example.com", "Production").
		License("MIT", "https://opensource.org/licenses/MIT")

	if doc.Info.Description != "A test API" {
		t.Errorf("expected description 'A test API', got %s", doc.Info.Description)
	}
	if len(doc.Servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(doc.Servers))
	}
	if doc.Servers[0].URL != "https://api.example.com" {
		t.Errorf("expected server URL 'https://api.example.com', got %s", doc.Servers[0].URL)
	}
	if doc.Info.License == nil || doc.Info.License.Name != "MIT" {
		t.Error("expected license MIT")
	}
}

func TestDocumentPathAndOperations(t *testing.T) {
	doc := NewDocument("Test API", "1.0.0")

	userSchema := ObjectSchema()
	userSchema["properties"] = map[string]interface{}{
		"id":   map[string]interface{}{"type": "integer"},
		"name": map[string]interface{}{"type": "string"},
	}

	op := NewOperation("Users", "Get a user").
		WithOperationID("getUser").
		Param("id", "path", "User ID", true, IntSchema()).
		JSONResponse(200, "User found", userSchema).
		PlainResponse(404, "User not found")

	item := NewPathItem().WithGet(op)
	doc.Path("/users/{id}", item)

	if doc.Paths["/users/{id}"] == nil {
		t.Fatal("expected path /users/{id} to be registered")
	}
	if doc.Paths["/users/{id}"].Get == nil {
		t.Fatal("expected GET operation on /users/{id}")
	}
	if doc.Paths["/users/{id}"].Get.OperationID != "getUser" {
		t.Errorf("expected operationId 'getUser', got %s", doc.Paths["/users/{id}"].Get.OperationID)
	}
	if len(doc.Paths["/users/{id}"].Get.Parameters) != 1 {
		t.Fatalf("expected 1 parameter, got %d", len(doc.Paths["/users/{id}"].Get.Parameters))
	}
	if doc.Paths["/users/{id}"].Get.Parameters[0].Name != "id" {
		t.Errorf("expected parameter name 'id', got %s", doc.Paths["/users/{id}"].Get.Parameters[0].Name)
	}
	if doc.Paths["/users/{id}"].Get.Responses[200] == nil {
		t.Error("expected 200 response")
	}
	if doc.Paths["/users/{id}"].Get.Responses[404] == nil {
		t.Error("expected 404 response")
	}
}

func TestOperationJSONBody(t *testing.T) {
	doc := NewDocument("Test API", "1.0.0")

	createSchema := ObjectSchema()
	createSchema["properties"] = map[string]interface{}{
		"name": map[string]interface{}{"type": "string"},
	}

	op := NewOperation("Users", "Create user").
		JSONBody("User data", true, createSchema).
		JSONResponse(201, "User created", RefSchema("User"))

	item := NewPathItem().WithPost(op)
	doc.Path("/users", item)

	postOp := doc.Paths["/users"].Post
	if postOp.RequestBody == nil {
		t.Fatal("expected request body")
	}
	if !postOp.RequestBody.Required {
		t.Error("expected required request body")
	}
	if postOp.RequestBody.Content["application/json"].Schema == nil {
		t.Error("expected JSON schema in request body")
	}
}

func TestDocumentAddSchema(t *testing.T) {
	doc := NewDocument("Test API", "1.0.0")
	schema := ObjectSchema()
	doc.AddSchema("User", schema)

	if doc.Components == nil {
		t.Fatal("expected components")
	}
	if doc.Components.Schemas["User"] == nil {
		t.Error("expected User schema in components")
	}
}

func TestDocumentAddSecurityScheme(t *testing.T) {
	doc := NewDocument("Test API", "1.0.0")
	doc.AddSecurityScheme("bearerAuth", map[string]interface{}{
		"type":         "http",
		"scheme":       "bearer",
		"bearerFormat": "JWT",
	})

	if doc.Components == nil || doc.Components.SecuritySchemes["bearerAuth"] == nil {
		t.Error("expected bearerAuth security scheme")
	}
}

func TestDocumentMarshalJSON(t *testing.T) {
	doc := NewDocument("Test API", "1.0.0").
		Description("A test")

	op := NewOperation("Users", "List users").
		JSONResponse(200, "Success", ArraySchema(StringSchema()))
	doc.Path("/users", NewPathItem().WithGet(op))

	data, err := doc.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if parsed["openapi"] != "3.1.0" {
		t.Errorf("expected openapi 3.1.0, got %v", parsed["openapi"])
	}
	info := parsed["info"].(map[string]interface{})
	if info["title"] != "Test API" {
		t.Errorf("expected title 'Test API', got %v", info["title"])
	}
	paths := parsed["paths"].(map[string]interface{})
	if paths["/users"] == nil {
		t.Error("expected /users path in JSON")
	}
}

func TestDocumentMarshalYAML(t *testing.T) {
	doc := NewDocument("Test API", "1.0.0").
		Description("A test")

	op := NewOperation("Users", "List users").
		JSONResponse(200, "Success", ArraySchema(StringSchema()))
	doc.Path("/users", NewPathItem().WithGet(op))

	data, err := doc.MarshalYAML()
	if err != nil {
		t.Fatalf("MarshalYAML failed: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty YAML output")
	}
}

func TestLoadSpecFile(t *testing.T) {
	dir := t.TempDir()

	jsonPath := filepath.Join(dir, "openapi.json")
	jsonContent := `{"openapi":"3.1.0","info":{"title":"Test","version":"1.0.0"}}`
	if err := os.WriteFile(jsonPath, []byte(jsonContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	content, ct, err := LoadSpecFile(jsonPath)
	if err != nil {
		t.Fatalf("LoadSpecFile failed: %v", err)
	}
	if ct != "application/json" {
		t.Errorf("expected content type 'application/json', got %s", ct)
	}
	if string(content) != jsonContent {
		t.Error("content mismatch")
	}

	yamlPath := filepath.Join(dir, "openapi.yaml")
	yamlContent := "openapi: 3.1.0\n"
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	content, ct, err = LoadSpecFile(yamlPath)
	if err != nil {
		t.Fatalf("LoadSpecFile failed: %v", err)
	}
	if ct != "application/yaml" {
		t.Errorf("expected content type 'application/yaml', got %s", ct)
	}

	if _, _, err := LoadSpecFile(filepath.Join(dir, "openapi.txt")); err == nil {
		t.Error("expected error for unsupported extension")
	}

	if _, _, err := LoadSpecFile(filepath.Join(dir, "nonexistent.json")); err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestSchemaHelpers(t *testing.T) {
	tests := []struct {
		name     string
		schema   Schema
		wantType string
	}{
		{"string", StringSchema(), "string"},
		{"int", IntSchema(), "integer"},
		{"int32", Int32Schema(), "integer"},
		{"bool", BoolSchema(), "boolean"},
		{"float", FloatSchema(), "number"},
		{"object", ObjectSchema(), "object"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.schema["type"] != tt.wantType {
				t.Errorf("expected type %s, got %v", tt.wantType, tt.schema["type"])
			}
		})
	}

	arr := ArraySchema(StringSchema())
	if arr["type"] != "array" {
		t.Errorf("expected array type, got %v", arr["type"])
	}

	ref := RefSchema("User")
	if ref["$ref"] != "#/components/schemas/User" {
		t.Errorf("expected $ref '#/components/schemas/User', got %v", ref["$ref"])
	}
}

func TestSchemaFromStruct(t *testing.T) {
	type User struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
		Bio  string `json:"bio,omitempty"`
	}

	schema := SchemaFromStruct(User{})

	if schema["type"] != "object" {
		t.Errorf("expected type 'object', got %v", schema["type"])
	}

	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties map")
	}

	if props["id"] == nil {
		t.Error("expected 'id' property")
	}
	if props["name"] == nil {
		t.Error("expected 'name' property")
	}
	if props["bio"] == nil {
		t.Error("expected 'bio' property")
	}

	idSchema, ok := props["id"].(map[string]interface{})
	if !ok {
		t.Fatal("expected id schema to be a map")
	}
	if idSchema["type"] != "integer" {
		t.Errorf("expected id type 'integer', got %v", idSchema["type"])
	}

	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("expected required to be a string slice")
	}

	requiredMap := map[string]bool{}
	for _, r := range required {
		requiredMap[r] = true
	}
	if !requiredMap["id"] {
		t.Error("expected 'id' to be required")
	}
	if !requiredMap["name"] {
		t.Error("expected 'name' to be required")
	}
	if requiredMap["bio"] {
		t.Error("expected 'bio' to NOT be required (omitempty)")
	}
}

func TestSchemaFromStructWithSlice(t *testing.T) {
	type Tag struct {
		Name string `json:"name"`
	}
	type Article struct {
		Title string `json:"title"`
		Tags  []Tag  `json:"tags"`
	}

	schema := SchemaFromStruct(Article{})
	props := schema["properties"].(map[string]interface{})

	tagsSchema, ok := props["tags"].(map[string]interface{})
	if !ok {
		t.Fatal("expected tags schema to be a map")
	}
	if tagsSchema["type"] != "array" {
		t.Errorf("expected tags type 'array', got %v", tagsSchema["type"])
	}
	items, ok := tagsSchema["items"].(map[string]interface{})
	if !ok {
		t.Fatal("expected items to be a map")
	}
	if items["type"] != "object" {
		t.Errorf("expected items type 'object', got %v", items["type"])
	}
}
