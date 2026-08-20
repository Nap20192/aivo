// Package session issues the anonymous diner session cookie and rate-limits
// the endpoints that key off it. See internal/menu/CONTEXT.md "Diner
// session" — this is Menu-specific plumbing (diner sessions, per-Table
// service-request dedupe) living in pkg/ alongside crypto and qrcode only
// because they're all single-implementation technical utilities, not
// because it's meant for reuse by other future services.
package session

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"time"
)

const (
	cookieName = "aivo_diner_session"
	ttl        = 5 * time.Hour
	tokenBytes = 32
)

// IssueOrRefresh reads the existing session cookie off r if present and
// well-formed, else mints a new random one. Either way it re-sets the
// cookie on w with a refreshed ttl expiry (sliding TTL) and returns the
// session ID.
func IssueOrRefresh(w http.ResponseWriter, r *http.Request) (sessionID string) {
	if c, err := r.Cookie(cookieName); err == nil && isValidToken(c.Value) {
		sessionID = c.Value
	} else {
		sessionID = newToken()
	}

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    sessionID,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	return sessionID
}

func newToken() string {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read only fails if the OS RNG is broken, which
		// leaves the process unable to do anything security-sensitive.
		panic("session: crypto/rand unavailable: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// isValidToken checks the cookie is a well-formed token of ours, not that
// it was actually issued by us — sessions are unauthenticated and
// server-side state is keyed by whatever ID shows up, so this only guards
// against obviously malformed/foreign cookie values.
func isValidToken(v string) bool {
	b, err := base64.RawURLEncoding.DecodeString(v)
	return err == nil && len(b) == tokenBytes
}
