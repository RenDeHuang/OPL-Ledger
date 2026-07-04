package auth

import (
	"net/http/httptest"
	"testing"
)

func TestAuthorizeServiceRejectsMissingTokenWhenConfigured(t *testing.T) {
	err := Authorize(httptest.NewRequest("POST", "/api/v1/billing/topups", nil), Config{ServiceToken: "svc_1"}, RoleService)
	if err != ErrMissingToken {
		t.Fatalf("error = %v, want %v", err, ErrMissingToken)
	}
}

func TestAuthorizeServiceAcceptsBearerToken(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/v1/billing/topups", nil)
	req.Header.Set("Authorization", "Bearer svc_1")
	err := Authorize(req, Config{ServiceToken: "svc_1"}, RoleService)
	if err != nil {
		t.Fatalf("Authorize returned error: %v", err)
	}
}

func TestAuthorizeAdminAcceptsAdminToken(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/audit/events", nil)
	req.Header.Set("Authorization", "Bearer admin_1")
	err := Authorize(req, Config{AdminToken: "admin_1"}, RoleAdmin)
	if err != nil {
		t.Fatalf("Authorize returned error: %v", err)
	}
}

func TestAuthorizeSkipsWhenRequiredTokenIsNotConfigured(t *testing.T) {
	err := Authorize(httptest.NewRequest("POST", "/api/v1/billing/topups", nil), Config{}, RoleService)
	if err != nil {
		t.Fatalf("Authorize should skip unconfigured auth: %v", err)
	}
}

func TestConfigFromEnvironmentReadsLedgerTokens(t *testing.T) {
	t.Setenv("OPL_LEDGER_SERVICE_TOKEN", "svc_env")
	t.Setenv("OPL_LEDGER_ADMIN_TOKEN", "admin_env")

	config := ConfigFromEnvironment()
	if config.ServiceToken != "svc_env" || config.AdminToken != "admin_env" {
		t.Fatalf("config = %+v", config)
	}
}
