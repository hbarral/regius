package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const markerFixture = `package main

func Register() {
	m.Keep()
	// additional entries here
}
`

func TestInsertAtMarker_MarkerPresent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "register.go")
	if err := os.WriteFile(path, []byte(markerFixture), 0644); err != nil {
		t.Fatal(err)
	}

	snippet := "\tm.Add(\"one\", One)\n"
	already := "m.Add(\"one\", One)"
	if err := insertAtMarker(path, "// additional entries here", snippet, already, nil); err != nil {
		t.Fatalf("insertAtMarker: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := `package main

func Register() {
	m.Keep()
	m.Add("one", One)

	// additional entries here
}
`
	if string(got) != want {
		t.Fatalf("insertAtMarker output mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestInsertAtMarker_Fallback(t *testing.T) {
	fallbackFixture := strings.Replace(markerFixture, "\t// additional entries here\n", "", 1)

	tests := []struct {
		name    string
		fixture string
	}{
		{"marker_missing_uses_fallback", fallbackFixture},
		{"marker_missing_fallback_before_last_brace", "package main\n\nfunc Register() {\n\tm.Keep()\n}\n"},
	}

	for _, e := range tests {
		tt := e
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "register.go")
			if err := os.WriteFile(path, []byte(tt.fixture), 0644); err != nil {
				t.Fatal(err)
			}

			snippet := "\tm.Add(\"one\", One)\n"
			already := "m.Add(\"one\", One)"
			err := insertAtMarker(path, "// additional entries here", snippet, already, func(content string) (string, bool) {
				idx := strings.LastIndex(content, "\n}")
				if idx < 0 {
					return "", false
				}
				return content[:idx] + "\n" + snippet + content[idx:], true
			})
			if err != nil {
				t.Fatalf("insertAtMarker: %v", err)
			}

			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(got), snippet[:len(snippet)-1]) != 1 {
				t.Errorf("expected exactly one inserted snippet, got:\n%s", got)
			}
		})
	}
}

func TestInsertAtMarker_FallbackFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.go")
	if err := os.WriteFile(path, []byte("not go"), 0644); err != nil {
		t.Fatal(err)
	}

	err := insertAtMarker(path, "// nope", "\tanything\n", "", func(content string) (string, bool) {
		return "", false
	})
	if err == nil || !strings.Contains(err.Error(), "no insertion point") {
		t.Fatalf("expected no-insertion-point error, got %v", err)
	}
}

func TestInsertAtMarker_Idempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "register.go")
	if err := os.WriteFile(path, []byte(markerFixture), 0644); err != nil {
		t.Fatal(err)
	}

	snippet := "\tm.Add(\"one\", One)\n"
	already := "m.Add(\"one\", One)"
	if err := insertAtMarker(path, "// additional entries here", snippet, already, nil); err != nil {
		t.Fatalf("insertAtMarker: %v", err)
	}
	if err := insertAtMarker(path, "// additional entries here", snippet, already, nil); err != nil {
		t.Fatalf("insertAtMarker (2nd): %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(got), "m.Add(\"one\", One)") != 1 {
		t.Errorf("expected exactly one entry after re-run, got:\n%s", got)
	}
}

func TestMarkerLine(t *testing.T) {
	cases := []struct {
		name    string
		content string
		marker  string
		want    string
		ok      bool
	}{
		{"indented", "a\n\t\t// mark\nb", "// mark", "\t\t// mark\n", true},
		{"first_line", "// mark\nb", "// mark", "// mark\n", true},
		{"last_line_no_newline", "a\n\t// mark", "// mark", "\t// mark", true},
		{"absent", "a\nb", "// mark", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := markerLine(c.content, c.marker)
			if ok != c.ok {
				t.Fatalf("markerLine ok = %v, want %v", ok, c.ok)
			}
			if got != c.want {
				t.Fatalf("markerLine = %q, want %q", got, c.want)
			}
		})
	}
}

func TestRegiusGoModVersion(t *testing.T) {
	cases := []struct {
		name string
		ver  string
		want string
	}{
		{"release_without_v", "1.9.3", "v1.9.3"},
		{"release_with_v", "v1.9.3", "v1.9.3"},
		{"dev", "dev", defaultRegiusVersion},
		{"empty", "", defaultRegiusVersion},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			old := Version
			Version = c.ver
			defer func() { Version = old }()
			if got := regiusGoModVersion(); got != c.want {
				t.Fatalf("regiusGoModVersion() = %q, want %q", got, c.want)
			}
		})
	}
}
