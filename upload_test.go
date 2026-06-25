package regius

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInSlice(t *testing.T) {
	tests := []struct {
		name  string
		slice []string
		s     string
		want  bool
	}{
		{"found", []string{"image/png", "image/jpeg"}, "image/png", true},
		{"not found", []string{"image/png", "image/jpeg"}, "image/gif", false},
		{"empty slice", []string{}, "image/png", false},
		{"nil slice", nil, "image/png", false},
		{"empty string present", []string{""}, "", true},
		{"empty string absent", []string{"a"}, "", false},
		{"case sensitive", []string{"Image/PNG"}, "image/png", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, inSlice(tt.slice, tt.s))
		})
	}
}
