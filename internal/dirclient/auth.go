// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package dirclient

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/agntcy/dir/client"
)

const deviceFlowTimeout = 5 * time.Minute

// TryGetCachedToken returns a valid access token from the issuer-scoped cache
// without triggering the device flow. If the cached token is expired but
// carries a refresh token, it is refreshed transparently. It returns
// ("", nil) when no usable token is available — the caller should fall back
// to an interactive auth flow.
func TryGetCachedToken(ctx context.Context, issuer, clientID string) (string, error) {
	cache, err := client.ResolveTokenCacheForIssuer(issuer)
	if err != nil {
		return "", nil //nolint:nilerr // no cache yet is not an error
	}

	tok, err := cache.GetValidToken()
	if err != nil {
		return "", fmt.Errorf("reading token cache: %w", err)
	}
	if tok != nil {
		return tok.AccessToken, nil
	}

	cached, err := cache.Load()
	if err != nil || cached == nil {
		return "", nil
	}

	if cached.AccessToken != "" && !cache.IsValid(cached) {
		refreshed, refreshErr := client.RefreshExpiredCachedOIDCToken(ctx, cache, cached, clientID)
		if refreshErr != nil {
			return "", nil
		}
		return refreshed.AccessToken, nil
	}

	return "", nil
}

// EnsureOIDCToken checks the token cache for a valid token and returns it.
// If no valid token exists, it runs the OIDC device flow to obtain one,
// writing the device code URL and instructions to output.
func EnsureOIDCToken(ctx context.Context, issuer, clientID string, output io.Writer) (string, error) {
	token, err := TryGetCachedToken(ctx, issuer, clientID)
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

	issuerCache, err := client.NewTokenCacheForIssuer(issuer)
	if err != nil {
		return "", fmt.Errorf("creating token cache for issuer: %w", err)
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
	if err := issuerCache.SaveAtomic(cached); err != nil {
		return "", fmt.Errorf("saving token: %w", err)
	}

	return result.AccessToken, nil
}
