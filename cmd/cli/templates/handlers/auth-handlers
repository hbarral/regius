package handlers

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"regius-app/data"
	"time"

	"github.com/CloudyKit/jet/v6"
	"gitlab.com/hbarral/regius/mailer"
	"gitlab.com/hbarral/regius/urlsigner"
)

func (h *Handlers) UserSignIn(w http.ResponseWriter, r *http.Request) {
	err := h.App.Render.Page(w, r, "login", nil, nil)
	if err != nil {
		h.App.ErrorLog.Println(err)
	}
}

func (h *Handlers) PostUserSignIn(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		w.Write([]byte(err.Error()))
		return
	}

	email := r.Form.Get("email")
	password := r.Form.Get("password")

	user, err := h.Models.Users.GetByEmail(email)
	if err != nil {
		w.Write([]byte(err.Error()))
		return
	}

	matches, err := user.PasswordMatches(password)
	if err != nil {
		w.Write([]byte("Error validating password"))
		return
	}

	if !matches {
		w.Write([]byte("Invalid password!"))
		return
	}

	if r.Form.Get("remember") == "remember" {
		randomString := h.randomString(12)
		hasher := sha256.New()

		_, err := hasher.Write([]byte(randomString))
		if err != nil {
			h.App.ErrorStatus(w, http.StatusBadRequest)
			return
		}

		sha := base64.URLEncoding.EncodeToString(hasher.Sum(nil))
		rm := data.RememberToken{}
		err = rm.InsertToken(user.ID, sha)
		if err != nil {
			h.App.ErrorStatus(w, http.StatusBadRequest)
			return
		}

		expire := time.Now().Add(365 * 24 * 60 * 60 * time.Second)
		cookie := http.Cookie{
			Name:     fmt.Sprintf("_%s_remember", h.App.AppName),
			Value:    fmt.Sprintf("%d|%s", user.ID, sha),
			Path:     "/",
			Expires:  expire,
			HttpOnly: true,
			Domain:   h.App.Session.Cookie.Domain,
			MaxAge:   315350000,
			Secure:   h.App.Session.Cookie.Secure,
			SameSite: http.SameSiteStrictMode,
		}

		http.SetCookie(w, &cookie)
		h.App.Session.Put(r.Context(), "rememberToken", sha)
	}

	h.App.Session.Put(r.Context(), "userID", user.ID)

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handlers) SignOut(w http.ResponseWriter, r *http.Request) {
	if h.App.Session.Exists(r.Context(), "remember_token") {
		rt := data.RememberToken{}
		_ = rt.Delete(h.App.Session.GetString(r.Context(), "remember_token"))
	}

	newCookie := http.Cookie{
		Name:     fmt.Sprintf("_%s_remember", h.App.AppName),
		Value:    "",
		Path:     "/",
		Expires:  time.Now().Add(-100 * time.Hour),
		HttpOnly: true,
		Domain:   h.App.Session.Cookie.Domain,
		MaxAge:   -1,
		Secure:   h.App.Session.Cookie.Secure,
		SameSite: http.SameSiteStrictMode,
	}

	http.SetCookie(w, &newCookie)

	h.App.Session.RenewToken(r.Context())
	h.App.Session.Remove(r.Context(), "userID")
	h.App.Session.Remove(r.Context(), "remember_token")
	h.App.Session.Destroy(r.Context())
	h.App.Session.RenewToken(r.Context())
	http.Redirect(w, r, "/users/signin", http.StatusSeeOther)
}

func (h *Handlers) Forgot(w http.ResponseWriter, r *http.Request) {
	err := h.render(w, r, "forgot", nil, nil)
	if err != nil {
		h.App.ErrorLog.Println("Error Rendering: ", err)
		h.App.Error500(w, r)
	}
}

func (h *Handlers) PostForgot(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		h.App.ErrorStatus(w, http.StatusBadRequest)
		return
	}

	var u *data.User
	email := r.Form.Get("email")
	u, err = u.GetByEmail(email)
	if err != nil {
		h.App.ErrorStatus(w, http.StatusBadRequest)
		return
	}

	link := fmt.Sprintf("%s/users/reset-password?email=%s", h.App.Server.URL, email)

	sign := urlsigner.Signer{
		Secret: []byte(h.App.EncryptionKey),
	}
	signedLink := sign.GenerateTokenFromString(link)
	h.App.InfoLog.Println("Signed link is: ", signedLink)

	var data struct {
		Link string
	}
	data.Link = signedLink
	msg := mailer.Message{
		To:       u.Email,
		Subject:  "Password Reset",
		Template: "password-reset",
		Data:     data,
		From:     "admin@some-example.com",
	}
	h.App.Mail.Jobs <- msg
	res := <-h.App.Mail.Results
	if res.Error != nil {
		h.App.ErrorStatus(w, http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/users/signin", http.StatusSeeOther)
}

func (h *Handlers) ResetPasswordForm(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	theURL := r.RequestURI
	testURL := fmt.Sprintf("%s%s", h.App.Server.URL, theURL)

	signer := urlsigner.Signer{
		Secret: []byte(h.App.EncryptionKey),
	}
	valid := signer.VerifyToken(testURL)
	if !valid {
		h.App.ErrorLog.Println("Invalid url")
		h.App.ErrorUnauthorized(w, r)
		return
	}

	expired := signer.Expired(testURL, 60)
	if expired {
		h.App.ErrorLog.Println("Link expired")
		h.App.ErrorUnauthorized(w, r)
		return
	}

	encryptedEmail, _ := h.encrypt(email)
	vars := make(jet.VarMap)
	vars.Set("email", encryptedEmail)

	err := h.render(w, r, "reset-password", vars, nil)
	if err != nil {
		return
	}
}

func (h *Handlers) PostResetPassword(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		h.App.Error500(w, r)
		return
	}

	email, err := h.decrypt(r.Form.Get("email"))
	if err != nil {
		h.App.Error500(w, r)
		return
	}

	var u data.User
	user, err := u.GetByEmail(email)
	if err != nil {
		h.App.Error500(w, r)
		return
	}

	err = user.ResetPassword(user.ID, r.Form.Get("password"))
	if err != nil {
		h.App.Error500(w, r)
		return
	}

	h.App.Session.Put(r.Context(), "flash", "Password has been reset. You can now sign in.")
	http.Redirect(w, r, "/users/signin", http.StatusSeeOther)
}
