package render

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"path/filepath"

	"github.com/CloudyKit/jet/v6"
	"github.com/alexedwards/scs/v2"
	"github.com/justinas/nosurf"

	ri18n "github.com/hbarral/regius/i18n"
)

type Render struct {
	RootPath   string
	Secure     bool
	Port       string
	ServerName string
	JetViews   *jet.Set
	Session    *scs.SessionManager
}

type TemplateData struct {
	IsAuthenticated bool
	IntMap          map[string]int
	StringMap       map[string]string
	FloatMap        map[string]float32
	Data            map[string]interface{}
	CSRFToken       string
	Port            string
	ServerName      string
	Secure          bool
	Locale          string
	Error           string
	Flash           string
}

type Template interface {
	Render(ctx context.Context, w io.Writer) error
}

func (s *Render) defaultData(td *TemplateData, r *http.Request) *TemplateData {
	td.Secure = s.Secure
	td.ServerName = s.ServerName
	td.CSRFToken = nosurf.Token(r)
	td.Port = s.Port
	td.Locale = ri18n.Locale(r.Context())
	if s.Session.Exists(r.Context(), "userID") {
		td.IsAuthenticated = true
	}

	td.Error = s.Session.PopString(r.Context(), "error")
	td.Flash = s.Session.PopString(r.Context(), "flash")

	return td
}

func (re *Render) Page(
	w http.ResponseWriter,
	r *http.Request,
	view Template,
	data interface{},
) error {
	td := &TemplateData{}

	if data != nil {
		td = data.(*TemplateData)
	}

	td = re.defaultData(td, r)

	ctx := context.WithValue(r.Context(), "templateData", td)

	if err := view.Render(ctx, w); err != nil {
		return fmt.Errorf("error rendering template: %w", err)
	}

	return nil
}

func (re *Render) Jet(name string, vars jet.VarMap) Template {
	return &jetView{set: re.JetViews, name: name + ".jet", vars: vars}
}

func (re *Render) Go(name string) Template {
	return &goView{rootPath: re.RootPath, name: name}
}

// GoLayout renders a Go template inside a shared layout.
// The page file is views/<name>.page.template and must define a "content" block.
// The layout file is views/layouts/<layout>.layout.template and executes the
// "content" block, e.g. {{template "content" .}}.
func (re *Render) GoLayout(name, layout string) Template {
	return &goView{rootPath: re.RootPath, name: name, layout: layout}
}

type jetView struct {
	set  *jet.Set
	name string
	vars jet.VarMap
}

func (j *jetView) Render(ctx context.Context, w io.Writer) error {
	t, err := j.set.GetTemplate(j.name)
	if err != nil {
		return fmt.Errorf("error loading jet template %s: %w", j.name, err)
	}

	if j.vars == nil {
		j.vars = make(jet.VarMap)
	}

	j.vars.Set("T", func(key string) string {
		return ri18n.T(ctx, key)
	})

	td, _ := ctx.Value("templateData").(*TemplateData)

	if err := t.Execute(w, j.vars, td); err != nil {
		return fmt.Errorf("error executing jet template %s: %w", j.name, err)
	}

	return nil
}

type goView struct {
	rootPath string
	name     string
	layout   string
}

func (g *goView) Render(ctx context.Context, w io.Writer) error {
	td, _ := ctx.Value("templateData").(*TemplateData)

	files := []string{
		fmt.Sprintf("%s/views/%s.page.template", g.rootPath, g.name),
	}
	if g.layout != "" {
		files = append([]string{fmt.Sprintf("%s/views/layouts/%s.layout.template", g.rootPath, g.layout)}, files...)
	}

	// Make component partials available to all Go templates.
	components, err := filepath.Glob(fmt.Sprintf("%s/views/components/*.page.template", g.rootPath))
	if err != nil {
		return fmt.Errorf("error listing go template components: %w", err)
	}
	files = append(files, components...)

	rootName := filepath.Base(files[0])
	t, err := template.New(rootName).Funcs(template.FuncMap{
		"T": func(key string) string {
			return ri18n.T(ctx, key)
		},
	}).ParseFiles(files...)
	if err != nil {
		if g.layout != "" {
			return fmt.Errorf("error loading go layout %q or page %q: %w", g.layout, g.name, err)
		}
		return fmt.Errorf("error loading go template %s: %w", g.name, err)
	}

	if g.layout != "" {
		layoutName := fmt.Sprintf("%s.layout.template", g.layout)
		if err := t.ExecuteTemplate(w, layoutName, td); err != nil {
			return fmt.Errorf("error executing go layout %q: %w", g.layout, err)
		}
		return nil
	}

	if err := t.Execute(w, td); err != nil {
		return fmt.Errorf("error executing go template %s: %w", g.name, err)
	}

	return nil
}
