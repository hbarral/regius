//go:build !gorm

package regius

import (
	"fmt"
)

// GORM returns an error when the "gorm" build tag is not enabled.
// Build with -tags gorm to get the real GORM implementation.
func (r *Regius) GORM() (interface{}, error) {
	return nil, fmt.Errorf("GORM support not compiled in; build with -tags gorm")
}

// GORMWithConfig returns an error when the "gorm" build tag is not enabled.
func (r *Regius) GORMWithConfig(cfg interface{}) (interface{}, error) {
	return nil, fmt.Errorf("GORM support not compiled in; build with -tags gorm")
}

// AutoMigrate returns an error when the "gorm" build tag is not enabled.
func (r *Regius) AutoMigrate(dst ...interface{}) error {
	return fmt.Errorf("GORM support not compiled in; build with -tags gorm")
}
