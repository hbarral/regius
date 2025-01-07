package render

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/CloudyKit/jet/v6"
	"github.com/alexedwards/scs/v2"
	"github.com/justinas/nosurf"
)

type Render struct {
	Renderer        string
	RootPath        string
	Secure          bool
	Port            string
	ServerName      string
	JetViews        *jet.Set
	Session         *scs.SessionManager
	TemplComponents map[string]Template
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
	Error           string
	Flash           string
}

func (s *Render) defaultData(td *TemplateData, r *http.Request) *TemplateData {
	td.Secure = s.Secure
	td.ServerName = s.ServerName
	td.CSRFToken = nosurf.Token(r)
	td.Port = s.Port
	if s.Session.Exists(r.Context(), "userID") {
		td.IsAuthenticated = true
	}

	td.Error = s.Session.PopString(r.Context(), "error")
	td.Flash = s.Session.PopString(r.Context(), "flash")

	return td
}

func (c *Render) Page(
	w http.ResponseWriter,
	r *http.Request,
	view string,
	variables, data interface{},
) error {
	switch strings.ToLower(c.Renderer) {
	case "go":
		return c.GoPage(w, r, view, data)
	case "jet":
		return c.JetPage(w, r, view, variables, data)
	case "templ":
		return c.TemplPage(w, r, view, data)
	default:
	}

	return errors.New("No rendering engine specified")
}

func (c *Render) GoPage(
	w http.ResponseWriter,
	r *http.Request,
	view string,
	data interface{},
) error {
	htmlTemplate, err := template.ParseFiles(fmt.Sprintf("%s/views/%s.page.template", c.RootPath, view))
	if err != nil {
		return err
	}

	td := &TemplateData{}

	if data != nil {
		td = data.(*TemplateData)
	}

	err = htmlTemplate.Execute(w, &td)
	if err != nil {
		return err
	}

	return nil
}

func (c *Render) JetPage(
	w http.ResponseWriter,
	r *http.Request,
	templateName string,
	variables, data interface{},
) error {
	var vars jet.VarMap

	if variables == nil {
		vars = make(jet.VarMap)
	} else {
		vars = variables.(jet.VarMap)
	}

	td := &TemplateData{}
	if data != nil {
		td = data.(*TemplateData)
	}

	td = c.defaultData(td, r)

	t, err := c.JetViews.GetTemplate(fmt.Sprintf("%s.jet", templateName))
	if err != nil {
		log.Println(err)

		return err
	}

	if err = t.Execute(w, vars, td); err != nil {
		log.Println(err)

		return err
	}

	return nil
}

type Template interface {
	Render(ctx context.Context, w io.Writer) error
}

func (re *Render) TemplPage(w http.ResponseWriter, r *http.Request, view string, data interface{}) error {
	td := &TemplateData{}
	if data != nil {
		td = data.(*TemplateData)
	}

	td = re.defaultData(td, r)

	tmpl, err := re.loadTemplComponent(view)
	if err != nil {
		return fmt.Errorf("error loading template %s: %w", view, err)
	}

	ctx := context.WithValue(r.Context(), "templateData", td)

	if err := tmpl.Render(ctx, w); err != nil {
		return fmt.Errorf("error rendering template %s: %w", view, err)
	}

	return nil
}

func (re *Render) loadTemplComponent(view string) (Template, error) {
	tmpl, exists := re.TemplComponents[view]
	if !exists {
		return nil, fmt.Errorf("template %s not found", view)
	}

	return tmpl, nil
}
