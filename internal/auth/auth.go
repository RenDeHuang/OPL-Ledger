package auth

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"os"
	"strings"
)

type Role string

const (
	RoleService Role = "service"
	RoleAdmin   Role = "admin"
)

var (
	ErrMissingToken = errors.New("auth_token_missing")
	ErrInvalidToken = errors.New("auth_token_invalid")
)

type Config struct {
	ServiceToken string
	AdminToken   string
}

func ConfigFromEnvironment() Config {
	return Config{
		ServiceToken: os.Getenv("OPL_LEDGER_SERVICE_TOKEN"),
		AdminToken:   os.Getenv("OPL_LEDGER_ADMIN_TOKEN"),
	}
}

func (c Config) Enabled(role Role) bool {
	return c.tokenFor(role) != ""
}

func Authorize(r *http.Request, config Config, role Role) error {
	expected := config.tokenFor(role)
	if expected == "" {
		return nil
	}
	actual := bearerToken(r.Header.Get("Authorization"))
	if actual == "" {
		return ErrMissingToken
	}
	if subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) != 1 {
		return ErrInvalidToken
	}
	return nil
}

func (c Config) tokenFor(role Role) string {
	switch role {
	case RoleService:
		return c.ServiceToken
	case RoleAdmin:
		return c.AdminToken
	default:
		return ""
	}
}

func bearerToken(header string) string {
	prefix := "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}
