//go:build !dusk

package regius

import (
	"fmt"
	"time"
)

// TakeScreenShot is a no-op stub when the "dusk" build tag is not enabled.
// Build with -tags dusk to get the real go-rod-based implementation.
func (r *Regius) TakeScreenShot(pageURL, testName string, w, h float64) {
	fileName := time.Now().Format("2006-01-02_15-04-05.000") + "_" + testName
	fmt.Printf("TakeScreenShot: build with -tags dusk to capture screenshots (%s)\n", fileName)
}

// FetchPage is a no-op stub when the "dusk" build tag is not enabled.
func (r *Regius) FetchPage(pageURL string) interface{} {
	return nil
}

// SelectElementByID is a no-op stub when the "dusk" build tag is not enabled.
func (r *Regius) SelectElementByID(page interface{}, elementID string) interface{} {
	return nil
}
