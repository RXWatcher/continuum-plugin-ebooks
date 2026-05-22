package server

import (
	"context"
	"errors"

	sdkruntime "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginsdk/runtime"
)

// CredentialValidator resolves a third-party "user#profile" / "password#pin"
// login to (userID, profileID) by delegating to the continuum host. profileID
// is "" for the primary profile. Defined as an interface so reader-route
// handlers can be tested with a fake.
type CredentialValidator interface {
	ValidateProfileCredential(ctx context.Context, username, password string) (userID, profileID string, err error)
}

// hostCredentialValidator is the production CredentialValidator — it calls the
// host RuntimeHost.ValidateProfileCredential RPC through the SDK runtime host.
type hostCredentialValidator struct{}

// NewHostCredentialValidator returns a CredentialValidator backed by the host.
func NewHostCredentialValidator() CredentialValidator { return hostCredentialValidator{} }

func (hostCredentialValidator) ValidateProfileCredential(
	ctx context.Context, username, password string,
) (string, string, error) {
	host := sdkruntime.Host()
	if host == nil {
		return "", "", errors.New("runtime host unavailable")
	}
	cred, err := host.ValidateProfileCredential(ctx, username, password)
	if err != nil {
		return "", "", err
	}
	return cred.UserID, cred.ProfileID, nil
}
