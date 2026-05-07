package dirclient

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/agntcy/dir/client"
)

const deviceFlowTimeout = 5 * time.Minute

// TryGetCachedToken returns a valid access token from the local cache without
// triggering the device flow. It returns ("", nil) when no valid token is
// available — the caller should fall back to an interactive auth flow.
func TryGetCachedToken() (string, error) {
	cache := client.NewTokenCache()

	tok, err := cache.GetValidToken()
	if err != nil {
		return "", fmt.Errorf("reading token cache: %w", err)
	}
	if tok != nil {
		return tok.AccessToken, nil
	}
	return "", nil
}

// EnsureOIDCToken checks the token cache for a valid token and returns it.
// If no valid token exists, it runs the OIDC device flow to obtain one,
// writing the device code URL and instructions to output.
func EnsureOIDCToken(ctx context.Context, issuer, clientID string, output io.Writer) (string, error) {
	token, err := TryGetCachedToken()
	if err != nil {
		return "", err
	}
	if token != "" {
		return token, nil
	}

	result, err := client.OIDC.RunDeviceFlow(ctx, &client.DeviceFlowConfig{
		Issuer:   issuer,
		ClientID: clientID,
		Timeout:  deviceFlowTimeout,
		Output:   output,
	})
	if err != nil {
		return "", fmt.Errorf("device flow: %w", err)
	}

	cached := &client.CachedToken{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		TokenType:    result.TokenType,
		Provider:     "oidc",
		Issuer:       issuer,
		ExpiresAt:    result.ExpiresAt,
		User:         result.Name,
		UserID:       result.Subject,
		Email:        result.Email,
		CreatedAt:    time.Now().UTC(),
	}
	if err := client.NewTokenCache().Save(cached); err != nil {
		return "", fmt.Errorf("saving token: %w", err)
	}

	return result.AccessToken, nil
}
