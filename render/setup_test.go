package render

import (
	"os"
	"testing"

	"github.com/CloudyKit/jet/v6"
	"github.com/alexedwards/scs/v2"
)

var views = jet.NewSet(
	jet.NewOSFileSystemLoader("./testdata/views"),
	jet.InDevelopmentMode(),
)

var testRenderer = Render{
	Renderer: "",
	RootPath: "",
	JetViews: views,
	Session:  scs.New(),
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
