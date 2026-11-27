package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// LoginRole is an application login role (control migration 00004): it can log in, cannot bypass
// row-level security and owns nothing. Only the constants below exist.
type LoginRole string

const (
	LoginWeb  LoginRole = "eps_web_login"
	LoginCore LoginRole = "eps_core_login"
)

// LoginRoleFor maps "web" or "core" to its login role.
func LoginRoleFor(side string) (LoginRole, error) {
	switch side {
	case "web":
		return LoginWeb, nil
	case "core":
		return LoginCore, nil
	}
	return "", fmt.Errorf("db: unknown side %q (web or core)", side)
}

// NewPassword returns a random password (32 bytes, base64url: only [A-Za-z0-9_-]).
func NewPassword() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func validPassword(p string) bool {
	if len(p) < 32 {
		return false
	}
	for _, r := range p {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

// SetLoginPassword sets the role's password (run by the pipeline as the branch owner). The
// password must be at least 32 characters of [A-Za-z0-9_-], so it can be placed in the statement
// without escaping (ALTER ROLE takes no parameters).
func SetLoginPassword(ctx context.Context, owner *sql.DB, role LoginRole, password string) error {
	if role != LoginWeb && role != LoginCore {
		return fmt.Errorf("db: unknown login role %q", role)
	}
	if !validPassword(password) {
		return errors.New("db: login password must be at least 32 characters of [A-Za-z0-9_-]")
	}
	if _, err := owner.ExecContext(ctx, "ALTER ROLE "+string(role)+" WITH LOGIN PASSWORD '"+password+"'"); err != nil {
		return fmt.Errorf("db: set password for %s: %s", role, Redact(err.Error()))
	}
	return nil
}

// LoginURL returns ownerURL with the user and password replaced by the login role's.
func LoginURL(ownerURL string, role LoginRole, password string) (string, error) {
	u, err := url.Parse(ownerURL)
	if err != nil || u.Host == "" || !strings.HasPrefix(u.Scheme, "postgres") {
		return "", errors.New("db: invalid connection URL")
	}
	u.User = url.UserPassword(string(role), password)
	return u.String(), nil
}
