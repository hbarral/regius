package cli

import "testing"

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
