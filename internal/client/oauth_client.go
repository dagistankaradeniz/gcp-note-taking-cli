package client

// CLIClientID identifies the Quillink CLI as an OAuth client to the
// device-grant endpoints (see app/services/oauth_device.py). It's a public
// client (no secret) -- device grant doesn't require one, same as `gh`/
// `gcloud`. Registered once per environment via the admin-only
// POST /api/admin/oauth-clients endpoint and baked into release builds via
// -ldflags "-X .../internal/client.CLIClientID=...". The value below is a
// local-dev placeholder; it will not resolve against staging/prod.
var CLIClientID = "REPLACE_WITH_REGISTERED_CLIENT_ID"

// CLIScopes is the fixed scope set requested by `quillink login` -- v1
// covers all note/folder/tag scopes; narrower opt-in scopes can follow
// once a real use case needs them.
var CLIScopes = []string{
	"notes:read", "notes:write",
	"folders:read", "folders:write",
	"tags:read", "tags:write",
}
