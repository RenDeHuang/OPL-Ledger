package ownership

import (
	"context"
	"errors"
)

var ErrWorkspaceNotFound = errors.New("workspace_not_found")

type WorkspaceResolver interface {
	WorkspaceOwnerAccountID(ctx context.Context, workspaceID string) (string, error)
}

func ValidateWorkspaceAccount(ctx context.Context, resolver WorkspaceResolver, accountID string, workspaceID string) error {
	if resolver == nil || workspaceID == "" {
		return nil
	}
	ownerAccountID, err := resolver.WorkspaceOwnerAccountID(ctx, workspaceID)
	if err != nil {
		return err
	}
	if ownerAccountID == "" || ownerAccountID != accountID {
		return ErrWorkspaceNotFound
	}
	return nil
}
