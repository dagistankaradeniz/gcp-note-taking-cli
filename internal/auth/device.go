package auth

import (
	"fmt"
	"time"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
)

// Login runs the OAuth 2.0 Device Authorization Grant (RFC 8628): request
// a device code, print it for the user, then poll until the browser step
// completes. Mirrors `gh auth login` / `gcloud auth login`. clientID is
// resolved by the caller (see cmd.resolveClientID) -- this package doesn't
// know about flags/env vars.
func Login(c *client.Client, clientID string, print func(format string, a ...any)) (string, error) {
	var codeResp client.DeviceCodeResponse
	err := c.PostPublic("/api/oauth/device/code", client.DeviceCodeRequest{
		ClientID: clientID,
		Scope:    client.CLIScopes,
	}, &codeResp)
	if err != nil {
		return "", fmt.Errorf("start device login: %w", err)
	}

	print("Go to %s and enter code: %s\n", codeResp.VerificationURI, codeResp.UserCode)
	print("Or open %s directly.\n", codeResp.VerificationURIComplete)
	print("Waiting for confirmation...\n")

	interval := time.Duration(codeResp.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(codeResp.ExpiresIn) * time.Second)

	for time.Now().Before(deadline) {
		time.Sleep(interval)

		var tokenResp client.OAuthTokenResponse
		err := c.PostPublic("/api/oauth/device/token", client.DeviceTokenRequest{
			GrantType:  "urn:ietf:params:oauth:grant-type:device_code",
			DeviceCode: codeResp.DeviceCode,
			ClientID:   clientID,
		}, &tokenResp)

		if err == nil {
			return tokenResp.AccessToken, nil
		}

		apiErr, ok := err.(*client.APIError)
		if !ok {
			// A network-level error (timeout, connection reset) rather
			// than an HTTP response -- treat as transient and keep
			// polling until the device code's own expiry, same as a
			// real gh/gcloud device-grant login would.
			continue
		}
		switch apiErr.Detail {
		case "authorization_pending":
			continue
		case "slow_down":
			interval += 5 * time.Second
			continue
		case "access_denied":
			return "", fmt.Errorf("login denied in the browser")
		default:
			return "", fmt.Errorf("device login failed: %s", apiErr.Detail)
		}
	}

	return "", fmt.Errorf("device code expired before login was confirmed")
}
