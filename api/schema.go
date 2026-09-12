package api

import (
	"reflect"
	"strings"
)

// Schema is a flexible map representing an OpenAPI 3.1 JSON Schema object.
// It supports any combination of keys (type, properties, items, $ref, etc.)
// and can be used directly in request bodies, responses, and parameters.
type Schema = map[string]interface{}

// StringSchema returns a schema for a JSON string.
func StringSchema() Schema {
	return Schema{"type": "string"}
}

// IntSchema returns a schema for a JSON integer.
func IntSchema() Schema {
	return Schema{"type": "integer", "format": "int64"}
}

// Int32Schema returns a schema for a 32-bit JSON integer.
func Int32Schema() Schema {
	return Schema{"type": "integer", "format": "int32"}
}

// BoolSchema returns a schema for a JSON boolean.
func BoolSchema() Schema {
	return Schema{"type": "boolean"}
}

// FloatSchema returns a schema for a JSON number (float).
func FloatSchema() Schema {
	return Schema{"type": "number", "format": "double"}
}

// ArraySchema returns a schema for a JSON array whose items conform to the
// provided item schema.
func ArraySchema(items Schema) Schema {
	return Schema{"type": "array", "items": items}
}

// ObjectSchema returns a base schema for a JSON object. Callers should add
// a "properties" key and a "required" key as needed.
func ObjectSchema() Schema {
	return Schema{"type": "object", "properties": map[string]interface{}{}}
}

// RefSchema returns a schema that references a component schema by name.
// The ref string should be a bare name (e.g. "User"); the full $ref path
// is constructed automatically.
func RefSchema(ref string) Schema {
	return Schema{"$ref": "#/components/schemas/" + ref}
}

// SchemaFromStruct reflects a Go struct into a JSON Schema object schema.
// It reads "json" struct tags for property names and required fields
// (fields without omitempty are required). Nested structs are
// recursively converted. Pointers are dereferenced for type detection.
// Unexported fields are skipped.
func SchemaFromStruct(v interface{}) Schema {
	if v == nil {
		return Schema{"type": "null"}
	}
	return structToSchema(reflect.TypeOf(v))
}

func structToSchema(t reflect.Type) Schema {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return Schema{"type": goKindToJSONType(t.Kind())}
	}

	properties := map[string]interface{}{}
	var required []string

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		name := field.Name
		omitEmpty := false

		if tag := field.Tag.Get("json"); tag != "" {
			parts := strings.Split(tag, ",")
			if parts[0] == "-" {
				continue
			}
			if parts[0] != "" {
				name = parts[0]
			}
			for _, p := range parts[1:] {
				if p == "omitempty" {
					omitEmpty = true
				}
			}
		}

		ft := field.Type
		for ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}

		var propSchema Schema
		if ft.Kind() == reflect.Struct && ft != reflect.TypeOf(Schema{}) {
			propSchema = structToSchema(ft)
		} else if ft.Kind() == reflect.Slice || ft.Kind() == reflect.Array {
			elemType := ft.Elem()
			for elemType.Kind() == reflect.Ptr {
				elemType = elemType.Elem()
			}
			if elemType.Kind() == reflect.Struct {
				propSchema = ArraySchema(structToSchema(elemType))
			} else {
				propSchema = ArraySchema(Schema{"type": goKindToJSONType(elemType.Kind())})
			}
		} else {
			propSchema = Schema{"type": goKindToJSONType(ft.Kind())}
		}

		properties[name] = propSchema
		if !omitEmpty {
			required = append(required, name)
		}
	}

	s := ObjectSchema()
	s["properties"] = properties
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func goKindToJSONType(k reflect.Kind) string {
	switch k {
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Bool:
		return "boolean"
	case reflect.Slice, reflect.Array:
		return "array"
	case reflect.Map, reflect.Struct:
		return "object"
	default:
		return "string"
	}
}
