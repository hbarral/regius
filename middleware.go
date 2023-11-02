package regius

import "net/http"

func (r *Regius) SessionLoad(next http.Handler) http.Handler {
	r.InfoLog.Println("SessionLoad called")
	return r.Session.LoadAndSave(next)
}
