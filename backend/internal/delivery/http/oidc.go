package http

import "net/http"

const oidcTransactionCookie = "orchestrator_oidc_transaction"

func (s *Server) handleAuthConfig(w http.ResponseWriter, r *http.Request) {
	mode := "local"
	if s.auth.OIDCEnabled() {
		mode = "oidc"
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"mode": mode, "localLoginAllowed": s.auth.LocalTestLoginAllowed()})
}
func (s *Server) handleOIDCStart(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	url, binding, err := s.auth.BeginOIDC()
	if err != nil {
		http.Redirect(w, r, "/ui/?sso_error=1", http.StatusSeeOther)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: oidcTransactionCookie, Value: binding, Path: "/api/v1/auth/oidc", HttpOnly: true, Secure: s.auth.SecureCookie(), SameSite: http.SameSiteLaxMode, MaxAge: 300})
	http.Redirect(w, r, url, http.StatusSeeOther)
}
func (s *Server) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	cookie, err := r.Cookie(oidcTransactionCookie)
	http.SetCookie(w, &http.Cookie{Name: oidcTransactionCookie, Path: "/api/v1/auth/oidc", HttpOnly: true, Secure: s.auth.SecureCookie(), SameSite: http.SameSiteLaxMode, MaxAge: -1})
	if err != nil || r.URL.Query().Get("error") != "" {
		http.Redirect(w, r, "/ui/?sso_error=1", http.StatusSeeOther)
		return
	}
	token, _, err := s.auth.CompleteOIDC(r.Context(), r.URL.Query().Get("state"), cookie.Value, r.URL.Query().Get("code"))
	if err != nil {
		http.Redirect(w, r, "/ui/?sso_error=1", http.StatusSeeOther)
		return
	}
	// A successful new login replaces any previous browser session.
	if old, err := r.Cookie(sessionCookie); err == nil {
		_ = s.auth.EndReplacedSession(r.Context(), old.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: s.auth.SecureCookie(), SameSite: http.SameSiteLaxMode, MaxAge: 8 * 60 * 60})
	http.Redirect(w, r, "/ui/", http.StatusSeeOther)
}
