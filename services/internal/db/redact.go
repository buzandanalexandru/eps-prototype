package db

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	// postgres://user:secret@host/db  ->  postgres://user:xxxxx@host/db
	urlPassword = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://[^:/?#@\s]+):([^@\s]*)@`)
	// password=secret or password='se cret'
	kvPassword = regexp.MustCompile(`(?i)(password|pass|pwd|sslpassword)\s*=\s*('[^']*'|"[^"]*"|[^\s&]+)`)
)

// Redact removes passwords from any text that may hold a connection string, in URL or key=value form.
func Redact(s string) string {
	s = urlPassword.ReplaceAllString(s, "${1}:xxxxx@")
	return kvPassword.ReplaceAllString(s, "${1}=xxxxx")
}

// HostOf returns the host (and port) of a connection string, for logs; "" when it cannot be parsed.
func HostOf(dsn string) string {
	if strings.Contains(dsn, "://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return ""
		}
		return u.Host
	}
	for _, f := range strings.Fields(dsn) {
		if k, v, ok := strings.Cut(f, "="); ok && strings.EqualFold(k, "host") {
			return v
		}
	}
	return ""
}

// SafeError wraps a connection error so that logging it never shows a password or the full
// connection string: only the host and the redacted cause.
type SafeError struct {
	Host string
	Err  error
}

func (e *SafeError) Error() string {
	return "postgres " + e.Host + ": " + Redact(e.Err.Error())
}

func (e *SafeError) Unwrap() error { return e.Err }
