package regius

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"path"
	"path/filepath"
)

func (r *Regius) WriteJSON(w http.ResponseWriter, status int, data interface{}, headers ...http.Header) error {
	out, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		return err
	}

	if len(headers) > 0 {
		for key, value := range headers[0] {
			w.Header()[key] = value
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, err = w.Write(out)
	if err != nil {
		return err
	}

	return nil
}

func (r *Regius) WriteXML(w http.ResponseWriter, status int, data interface{}, headers ...http.Header) error {
	out, err := xml.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	if len(headers) > 0 {
		for key, value := range headers[0] {
			w.Header()[key] = value
		}
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	_, err = w.Write(out)
	if err != nil {
		return err
	}

	return nil
}

func (c *Regius) DownloadFile(w http.ResponseWriter, r *http.Request, pathToFile, fileName string) error {
	fp := path.Join(pathToFile, fileName)
	fileToServe := filepath.Clean(fp)
	w.Header().Set("Content-Type", fmt.Sprintf("attachment; file=\"%s\"", fileName))
	http.ServeFile(w, r, fileToServe)

	return nil
}

func (c *Regius) Error404(w http.ResponseWriter, r *http.Request) {
	c.ErrorStatus(w, http.StatusNotFound)
}

func (c *Regius) Error500(w http.ResponseWriter, r *http.Request) {
	c.ErrorStatus(w, http.StatusInternalServerError)
}

func (c *Regius) ErrorUnauthorized(w http.ResponseWriter, r *http.Request) {
	c.ErrorStatus(w, http.StatusUnauthorized)
}

func (c *Regius) ErrorForbidden(w http.ResponseWriter, r *http.Request) {
	c.ErrorStatus(w, http.StatusForbidden)
}

func (r *Regius) ErrorStatus(w http.ResponseWriter, status int) {
	http.Error(w, http.StatusText(status), status)
}
