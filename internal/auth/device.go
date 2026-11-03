package auth

import (
	"fmt"
	"time"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/browser"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
)

// LoginView is how the sign-in steps are shown: plain lines for scripts
// and pipes, a boxed code with a spinner on a terminal.
type LoginView interface {
	// ShowCode presents the code and where to enter it; opened says the
	// browser was opened on the page already.
	ShowCode(uri, completeURI, code string, opened bool)
	// Waiting starts the wait for approval; the returned func ends it.
	Waiting() (done func())
}

// PlainLogin prints the steps as plain lines through print.
type PlainLogin func(format string, a ...any)

func (p PlainLogin) ShowCode(uri, completeURI, code string, opened bool) {
	p("Go to %s and enter code: %s\n", uri, code)
	if opened {
		p("Opened your browser. Check the code there matches this one.\n")
	} else {
		p("Or open %s directly.\n", completeURI)
	}
}

func (p PlainLogin) Waiting() func() {
	p("Waiting for confirmation...\n")
	return func() {}
}

// Login runs the OAuth 2.0 Device Authorization Grant (RFC 8628): request
// a device code, print it for the user, then poll until the browser step
// completes. Mirrors `gh auth login` / `gcloud auth login`. clientID is
// resolved by the caller (see cmd.resolveClientID) -- this package doesn't
// know about flags/env vars. With openBrowser the sign-in page opens on
// its own, code filled in; the link is shown either way.
func Login(c *client.Client, clientID string, openBrowser bool, view LoginView) (string, error) {
	var codeResp client.DeviceCodeResponse
	err := c.PostPublic("/api/oauth/device/code", client.DeviceCodeRequest{
		ClientID: clientID,
		Scope:    client.CLIScopes,
	}, &codeResp)
	if err != nil {
		return "", fmt.Errorf("start device login: %w", err)
	}

	opened := openBrowser && browser.Open(codeResp.VerificationURIComplete) == nil
	view.ShowCode(codeResp.VerificationURI, codeResp.VerificationURIComplete, codeResp.UserCode, opened)
	stopWaiting := view.Waiting()
	defer stopWaiting()

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
