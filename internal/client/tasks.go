package client

// Mirrors app/models/task.py and app/models/task_category.py in
// gcp-note-taking-backend (served by app/routers/v1_tasks.py).

type TaskRecurrence struct {
	Freq      string  `json:"freq"`
	Interval  int     `json:"interval"`
	ByWeekday []int   `json:"by_weekday"`
	Until     *string `json:"until"`
	Count     *int    `json:"count"`
}

type TaskLocation struct {
	DisplayName string  `json:"display_name"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
}

type TaskShare struct {
	UID   string `json:"uid"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type Task struct {
	ID             string          `json:"id"`
	Title          string          `json:"title"`
	Description    *string         `json:"description"`
	UserID         string          `json:"user_id"`
	Status         string          `json:"status"`
	DueAt          *string         `json:"due_at"`
	AllDay         bool            `json:"all_day"`
	StartAt        *string         `json:"start_at"`
	Location       *TaskLocation   `json:"location"`
	Recurrence     *TaskRecurrence `json:"recurrence"`
	Completed      bool            `json:"completed"`
	CompletedAt    *string         `json:"completed_at"`
	ExcludedFromAI bool            `json:"excluded_from_ai"`
	SharedWith     []TaskShare     `json:"shared_with"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
	TrashedAt      *string         `json:"trashed_at"`
	OccurrenceDate *string         `json:"occurrence_date"`
	IsSharedWithMe bool            `json:"is_shared_with_me"`
	CardColor      *string         `json:"card_color"`
	CardPattern    *string         `json:"card_pattern"`
	CategoryID     *string         `json:"category_id"`
	Reminders      []int           `json:"reminders"`
	NoteIDs        []string        `json:"note_ids"`
	ZkUnreadable   bool            `json:"zk_unreadable,omitempty"`
}

type TaskListResponse struct {
	Tasks   []Task `json:"tasks"`
	Total   int    `json:"total"`
	HasMore bool   `json:"has_more"`
}

type TaskDueResponse struct {
	Count int    `json:"count"`
	Items []Task `json:"items"`
}

type TaskCreateRequest struct {
	Title       string  `json:"title"`
	Description *string `json:"description,omitempty"`
	DueAt       *string `json:"due_at,omitempty"`
	AllDay      bool    `json:"all_day,omitempty"`
	CategoryID  *string `json:"category_id,omitempty"`
	Reminders   []int   `json:"reminders,omitempty"`
}

type TaskCompleteRequest struct {
	OccurrenceDate *string `json:"occurrence_date,omitempty"`
	Completed      bool    `json:"completed"`
}

type TaskCategory struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type TaskCategoryListResponse struct {
	Categories []TaskCategory `json:"categories"`
}
