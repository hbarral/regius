package regius

import (
	"fmt"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/rod/lib/utils"
)

func (r *Regius) TakeScreenShot(pageURL, testName string, w, h float64) {
	page := rod.New().MustConnect().MustIgnoreCertErrors(true).MustPage(pageURL).MustWaitLoad()

	image, _ := page.Screenshot(true, &proto.PageCaptureScreenshot{
		Format: proto.PageCaptureScreenshotFormatPng,
		Clip: &proto.PageViewport{
			X:      0,
			Y:      0,
			Width:  w,
			Height: h,
			Scale:  1,
		},
		FromSurface: true,
	})

	fileName := time.Now().Format("2006-01-02_15-04-05.000") + "_" + testName
	_ = utils.OutputFile(fmt.Sprintf("%s/screenshots/%s-%s.png", r.RootPath, testName, fileName), image)
}

func (r *Regius) FetchPage(pageURL string) *rod.Page {
	return rod.New().MustConnect().MustIgnoreCertErrors(true).MustPage(pageURL).MustWaitLoad()
}

func (r *Regius) SelectElementByID(page *rod.Page, elementID string) *rod.Element {
	js := fmt.Sprintf(`function() { return document.getElementById('%s'); }`, elementID)
	return page.MustElementByJS(js)
}
