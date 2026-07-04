package ownership

import (
	"context"
	"errors"
	"testing"
)

type staticResolver map[string]string

func (r staticResolver) WorkspaceOwnerAccountID(_ context.Context, workspaceID string) (string, error) {
	accountID, ok := r[workspaceID]
	if !ok {
		return "", ErrWorkspaceNotFound
	}
	return accountID, nil
}

func TestValidateWorkspaceAccountAllowsMatchingOwner(t *testing.T) {
	err := ValidateWorkspaceAccount(context.Background(), staticResolver{"ws_1": "acct_1"}, "acct_1", "ws_1")
	if err != nil {
		t.Fatalf("ValidateWorkspaceAccount returned error: %v", err)
	}
}

func TestValidateWorkspaceAccountRejectsDifferentOwnerAsNotFound(t *testing.T) {
	err := ValidateWorkspaceAccount(context.Background(), staticResolver{"ws_1": "acct_2"}, "acct_1", "ws_1")
	if !errors.Is(err, ErrWorkspaceNotFound) {
		t.Fatalf("expected workspace not found, got %v", err)
	}
}

func TestValidateWorkspaceAccountAllowsMissingWorkspaceWhenNoResolverConfigured(t *testing.T) {
	err := ValidateWorkspaceAccount(context.Background(), nil, "acct_1", "ws_1")
	if err != nil {
		t.Fatalf("nil resolver should skip validation: %v", err)
	}
}
