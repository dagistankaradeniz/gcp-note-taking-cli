package client

// Mirrors app/models/canvas.py's CanvasResponse / CanvasListResponse in
// gcp-note-taking-backend (served read-only by app/routers/v1_canvases.py).
// Elements stay untyped: they're the shared Canvas v1 format, normalised
// by internal/canvasrender before anything reads them.

type Canvas struct {
	ID                string   `json:"id"`
	Title             string   `json:"title"`
	FolderID          *string  `json:"folder_id"`
	Elements          []any    `json:"elements"`
	UserID            string   `json:"user_id"`
	Status            string   `json:"status"`
	Pinned            bool     `json:"pinned"`
	ExcludedFromAI    bool     `json:"excluded_from_ai"`
	Tags              []string `json:"tags"`
	VaultEncrypted    bool     `json:"vault_encrypted"`
	Locked            bool     `json:"locked"`
	NoteEncrypted     bool     `json:"note_encrypted"`
	EncryptedElements *string  `json:"encrypted_elements"`
	SizeBytes         int      `json:"size_bytes"`
	CreatedAt         string   `json:"created_at"`
	UpdatedAt         string   `json:"updated_at"`
	TrashedAt         *string  `json:"trashed_at"`
	SharedCount       int      `json:"shared_count"`
	CollabEnabled     bool     `json:"collab_enabled"`
	CardColor         *string  `json:"card_color"`
	CardPattern       *string  `json:"card_pattern"`
	ZkUnreadable      bool     `json:"zk_unreadable,omitempty"`
}

type CanvasListResponse struct {
	Canvases []Canvas `json:"canvases"`
	Total    int      `json:"total"`
	HasMore  bool     `json:"has_more"`
}
