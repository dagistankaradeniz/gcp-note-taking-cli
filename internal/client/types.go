package client

// Mirrors app/models/note.py / app/models/folder.py / app/models/api_access.py
// in gcp-note-taking-backend. Field names are snake_case to match the wire
// contract exactly -- see that repo's AGENTS.md domain conventions.

type Note struct {
	ID              string         `json:"id"`
	Title           string         `json:"title"`
	Body            map[string]any `json:"body"`
	FolderID        *string        `json:"folder_id"`
	Tags            []string       `json:"tags"`
	LinksTo         []string       `json:"links_to"`
	UserID          string         `json:"user_id"`
	Status          string         `json:"status"`
	Pinned          bool           `json:"pinned"`
	Locked          bool           `json:"locked"`
	EditorMode      string         `json:"editor_mode"`
	SizeBytes       int            `json:"size_bytes"`
	CreatedAt       string         `json:"created_at"`
	UpdatedAt       string         `json:"updated_at"`
	TrashedAt       *string        `json:"trashed_at"`
	SharedCount     int            `json:"shared_count"`
	ExcludedFromAI  bool           `json:"excluded_from_ai"`
	VaultEncrypted  bool           `json:"vault_encrypted"`
	SensitiveLocked bool           `json:"sensitive_locked"`
	AttachmentCount int            `json:"attachment_count"`
}

type NoteListResponse struct {
	Notes   []Note `json:"notes"`
	Total   int    `json:"total"`
	HasMore bool   `json:"has_more"`
}

type NoteCreateRequest struct {
	Title      string         `json:"title"`
	Body       map[string]any `json:"body,omitempty"`
	FolderID   *string        `json:"folder_id,omitempty"`
	Tags       []string       `json:"tags,omitempty"`
	Pinned     bool           `json:"pinned,omitempty"`
	EditorMode string         `json:"editor_mode,omitempty"`
}

type NoteUpdateRequest struct {
	Title    *string        `json:"title,omitempty"`
	Body     map[string]any `json:"body,omitempty"`
	FolderID *string        `json:"folder_id,omitempty"`
	Tags     *[]string      `json:"tags,omitempty"`
	Pinned   *bool          `json:"pinned,omitempty"`
}

type NoteUnlockRequest struct {
	Password string `json:"password"`
}

type NoteUnlockResponse struct {
	Body map[string]any `json:"body"`
}

type Folder struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Color       *string `json:"color"`
	ParentID    *string `json:"parent_id"`
	UserID      string  `json:"user_id"`
	Pinned      bool    `json:"pinned"`
	Status      string  `json:"status"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
	SharedCount int     `json:"shared_count"`
}

type FolderListResponse struct {
	Folders []Folder `json:"folders"`
	Total   int      `json:"total"`
}

type StatusResponse struct {
	Authenticated  bool     `json:"authenticated"`
	Scopes         []string `json:"scopes"`
	CredentialType string   `json:"credential_type"`
}

// Device Authorization Grant (RFC 8628) -- app/models/api_access.py.

type DeviceCodeRequest struct {
	ClientID string   `json:"client_id"`
	Scope    []string `json:"scope"`
}

type DeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type DeviceTokenRequest struct {
	GrantType  string `json:"grant_type"`
	DeviceCode string `json:"device_code"`
	ClientID   string `json:"client_id"`
}

type OAuthTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
}

// Organizations (read-only in v1 -- see app/routers/v1_organizations.py's
// module docstring for why there's no write endpoint yet). Mirrors
// app/models/organization.py's OrganizationSummary/OrganizationDetail/
// OrgMemberSummary/SeatAllocation/OrgAlert.

type SeatAllocation struct {
	Free int `json:"free"`
	Plus int `json:"plus"`
	Pro  int `json:"pro"`
}

type OrgAlert struct {
	Type          string `json:"type"`
	Severity      string `json:"severity"`
	DaysRemaining int    `json:"days_remaining"`
}

type OrgMember struct {
	UID              string  `json:"uid"`
	Email            string  `json:"email"`
	DisplayName      *string `json:"display_name"`
	OrgRole          string  `json:"org_role"`
	OrgJoinedAt      *string `json:"org_joined_at"`
	SubscriptionPlan string  `json:"subscription_plan"`
}

type Organization struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	PlanTier        string         `json:"plan_tier"`
	SeatCount       int            `json:"seat_count"`
	SeatAllocations SeatAllocation `json:"seat_allocations"`
	MemberCount     int            `json:"member_count"`
	Frozen          bool           `json:"frozen"`
	FrozenAt        *string        `json:"frozen_at"`
	CreatedAt       *string        `json:"created_at"`
	Members         []OrgMember    `json:"members"`
	SsoStatus       string         `json:"sso_status"`
	Alerts          []OrgAlert     `json:"alerts"`
}
