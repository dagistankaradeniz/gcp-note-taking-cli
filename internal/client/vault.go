package client

// Mirrors app/models/vault.py in gcp-note-taking-backend (served read-only
// by app/routers/v1_vault.py). Titles, bodies and canvas elements are
// ciphertext; the CLI decrypts them with the Vault key.

type VaultStatusResponse struct {
	IsSetup         bool    `json:"is_setup"`
	VaultFolderID   *string `json:"vault_folder_id"`
	VaultSalt       *string `json:"vault_salt"`
	VaultIterations *int    `json:"vault_iterations"`
}

type VaultUnlockRequest struct {
	PasswordHash string `json:"password_hash"`
}

type VaultUnlockResponse struct {
	VaultFolderID   string `json:"vault_folder_id"`
	VaultSalt       string `json:"vault_salt"`
	VaultIterations int    `json:"vault_iterations"`
	WrappedDek      string `json:"wrapped_dek"`
	WrappedDekIV    string `json:"wrapped_dek_iv"`
}

type VaultNote struct {
	ID        string         `json:"id"`
	Title     string         `json:"title"`
	Body      map[string]any `json:"body"`
	Tags      []string       `json:"tags"`
	FolderID  *string        `json:"folder_id"`
	Pinned    bool           `json:"pinned"`
	SizeBytes int            `json:"size_bytes"`
	CreatedAt string         `json:"created_at"`
	UpdatedAt string         `json:"updated_at"`
	// Set by the CLI when the note doesn't decrypt.
	Unreadable bool `json:"unreadable,omitempty"`
}

type VaultNoteListResponse struct {
	Notes []VaultNote `json:"notes"`
	Total int         `json:"total"`
}

type VaultCanvas struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	Elements  *string `json:"elements"`
	FolderID  *string `json:"folder_id"`
	Pinned    bool    `json:"pinned"`
	SizeBytes int     `json:"size_bytes"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
	// Set by the CLI when the canvas doesn't decrypt.
	Unreadable bool `json:"unreadable,omitempty"`
}

type VaultCanvasListResponse struct {
	Canvases []VaultCanvas `json:"canvases"`
	Total    int           `json:"total"`
}
