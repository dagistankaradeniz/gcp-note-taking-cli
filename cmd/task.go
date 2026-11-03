package cmd

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/zkcrypto"
	"github.com/spf13/cobra"
)

// Tasks over /v1/tasks (backend app/routers/v1_tasks.py). A Zero-Knowledge
// account's task title/description arrive encrypted and are decrypted here
// with the same account key as notes; ZK credentials are read-only, so the
// write commands fail on the server with a missing-scope error.

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "List, create, update and complete tasks",
}

var (
	taskListStatus     string
	taskListShared     bool
	taskListFrom       string
	taskListTo         string
	taskListCategories []string
	taskListSort       string
	taskListLimit      int
	taskListStartAfter string
	taskListOpen       bool
	taskListDone       bool

	taskDescription   string
	taskDue           string
	taskCategory      string
	taskTitle         string
	taskClearDue      bool
	taskOccurrence    string
	taskClearCategory bool
)

var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks",
	Example: `  quillink task list
  quillink task list --open
  quillink task list --from 2026-11-01 --to 2026-11-30
  quillink task list --category <category-id> --output csv --out tasks.csv`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if taskListOpen && taskListDone {
			return fmt.Errorf("--open and --done can't be used together")
		}
		q := url.Values{}
		q.Set("status", taskListStatus)
		if taskListShared {
			q.Set("shared_with_me", "true")
		}
		if (taskListFrom == "") != (taskListTo == "") {
			return fmt.Errorf("--from and --to go together")
		}
		if taskListFrom != "" {
			from, err := parseDay(taskListFrom)
			if err != nil {
				return fmt.Errorf("--from: %w", err)
			}
			to, err := parseDay(taskListTo)
			if err != nil {
				return fmt.Errorf("--to: %w", err)
			}
			q.Set("range_start", from.Format(time.RFC3339))
			q.Set("range_end", to.Add(24*time.Hour-time.Second).Format(time.RFC3339))
		}
		for _, id := range taskListCategories {
			q.Add("category_ids", id)
		}
		switch taskListSort {
		case "due":
			q.Set("sort", "due_at")
		case "updated":
			q.Set("sort", "updated_at")
		default:
			return fmt.Errorf("--sort must be due or updated")
		}
		q.Set("limit", strconv.Itoa(taskListLimit))
		if taskListStartAfter != "" {
			q.Set("start_after", taskListStartAfter)
		}

		c := newClient()
		dek, err := resolveZkDek(c)
		if err != nil {
			return err
		}
		var resp client.TaskListResponse
		if err := c.Do("GET", "/v1/tasks", q, nil, &resp); err != nil {
			return err
		}
		// --open/--done filter the fetched page; paging still follows the
		// server's own cursor (the last task it returned).
		cursor := lastTaskID(resp.Tasks, taskListStartAfter)
		filtered := resp.Tasks[:0]
		for _, t := range resp.Tasks {
			if (taskListOpen && t.Completed) || (taskListDone && !t.Completed) {
				continue
			}
			decryptTaskZK(dek, &t)
			filtered = append(filtered, t)
		}
		resp.Tasks = filtered
		resp.Total = len(filtered)

		if jsonOutput {
			return output.JSON(resp)
		}
		printTaskTable(c, resp.Tasks)
		if resp.HasMore && taskListFrom == "" {
			fmt.Fprintf(os.Stderr, "\nMore tasks available. Continue with:\n  quillink task list --start-after %s\n", cursor)
		}
		return nil
	},
}

var taskDueCmd = &cobra.Command{
	Use:   "due",
	Short: "Tasks due today or overdue",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		dek, err := resolveZkDek(c)
		if err != nil {
			return err
		}
		var resp client.TaskDueResponse
		if err := c.Do("GET", "/v1/tasks/due", nil, nil, &resp); err != nil {
			return err
		}
		for i := range resp.Items {
			decryptTaskZK(dek, &resp.Items[i])
		}
		if jsonOutput {
			return output.JSON(resp)
		}
		if len(resp.Items) == 0 {
			fmt.Println("Nothing due.")
			return nil
		}
		printTaskTable(c, resp.Items)
		return nil
	},
}

var taskGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Show one task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		dek, err := resolveZkDek(c)
		if err != nil {
			return err
		}
		var t client.Task
		if err := c.Do("GET", "/v1/tasks/"+url.PathEscape(args[0]), nil, nil, &t); err != nil {
			return err
		}
		decryptTaskZK(dek, &t)
		if jsonOutput {
			return output.JSON(t)
		}
		printTaskDetail(c, t)
		return nil
	},
}

var taskCreateCmd = &cobra.Command{
	Use:   "create <title>",
	Short: "Create a task",
	Example: `  quillink task create "Send the weekly report" --due 2026-11-07
  quillink task create "Call the dentist" --due "2026-11-05 14:30" --description "Ask about Friday"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		req := client.TaskCreateRequest{Title: args[0]}
		if cmd.Flags().Changed("description") {
			req.Description = &taskDescription
		}
		if taskDue != "" {
			due, allDay, err := parseDue(taskDue)
			if err != nil {
				return fmt.Errorf("--due: %w", err)
			}
			req.DueAt, req.AllDay = &due, allDay
		}
		if taskCategory != "" {
			req.CategoryID = &taskCategory
		}
		c := newClient()
		dek, err := resolveZkDek(c)
		if err != nil {
			return err
		}
		if dek != nil {
			if err := sealTaskCreate(dek, &req); err != nil {
				return err
			}
		}
		var t client.Task
		if err := c.Do("POST", "/v1/tasks", nil, req, &t); err != nil {
			return err
		}
		decryptTaskZK(dek, &t)
		if jsonOutput {
			return output.JSON(t)
		}
		fmt.Printf("Created task %s\n", t.ID)
		return nil
	},
}

var taskUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Change a task's title, description, due date or category",
	Example: `  quillink task update <id> --due 2026-11-10
  quillink task update <id> --title "Send the monthly report" --clear-due`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		body := map[string]any{}
		if cmd.Flags().Changed("title") {
			body["title"] = taskTitle
		}
		if cmd.Flags().Changed("description") {
			body["description"] = taskDescription
		}
		switch {
		case taskClearDue && taskDue != "":
			return fmt.Errorf("--due and --clear-due can't be used together")
		case taskClearDue:
			body["due_at"], body["all_day"], body["start_at"] = nil, false, nil
		case taskDue != "":
			due, allDay, err := parseDue(taskDue)
			if err != nil {
				return fmt.Errorf("--due: %w", err)
			}
			body["due_at"], body["all_day"] = due, allDay
		}
		switch {
		case taskClearCategory && taskCategory != "":
			return fmt.Errorf("--category and --clear-category can't be used together")
		case taskClearCategory:
			body["category_id"] = nil
		case taskCategory != "":
			body["category_id"] = taskCategory
		}
		if len(body) == 0 {
			return fmt.Errorf("nothing to update -- pass at least one flag (see --help)")
		}
		c := newClient()
		dek, err := resolveZkDek(c)
		if err != nil {
			return err
		}
		if dek != nil {
			if err := refuseZkTaskScheduling(body["due_at"] != nil, body["category_id"] != nil); err != nil {
				return err
			}
			if err := sealTaskWrite(c, dek, args[0], body); err != nil {
				return err
			}
		}
		var t client.Task
		if err := c.Do("PUT", "/v1/tasks/"+url.PathEscape(args[0]), nil, body, &t); err != nil {
			return err
		}
		decryptTaskZK(dek, &t)
		if jsonOutput {
			return output.JSON(t)
		}
		fmt.Printf("Updated task %s\n", t.ID)
		return nil
	},
}

func completeCmd(use, short string, completed bool) *cobra.Command {
	c := &cobra.Command{
		Use:   use + " <id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req := client.TaskCompleteRequest{Completed: completed}
			if taskOccurrence != "" {
				if _, err := parseDay(taskOccurrence); err != nil {
					return fmt.Errorf("--date: %w", err)
				}
				req.OccurrenceDate = &taskOccurrence
			}
			var t client.Task
			if err := newClient().Do("POST", "/v1/tasks/"+url.PathEscape(args[0])+"/complete", nil, req, &t); err != nil {
				return err
			}
			if jsonOutput {
				return output.JSON(t)
			}
			if completed {
				fmt.Printf("Marked task %s done\n", t.ID)
			} else {
				fmt.Printf("Reopened task %s\n", t.ID)
			}
			return nil
		},
	}
	c.Flags().StringVar(&taskOccurrence, "date", "", "for a repeating task: the occurrence to mark (YYYY-MM-DD)")
	return c
}

var taskDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Move a task to the trash",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := newClient().Do("DELETE", "/v1/tasks/"+url.PathEscape(args[0]), nil, nil, nil); err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(map[string]any{"id": args[0], "status": "trashed"})
		}
		fmt.Printf("Moved task %s to the trash (restore with `quillink task restore %s`)\n", args[0], args[0])
		return nil
	},
}

var taskRestoreCmd = &cobra.Command{
	Use:   "restore <id>",
	Short: "Restore a task from the trash",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var t client.Task
		if err := newClient().Do("POST", "/v1/tasks/"+url.PathEscape(args[0])+"/restore", nil, nil, &t); err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(t)
		}
		fmt.Printf("Restored task %s\n", t.ID)
		return nil
	},
}

var taskCategoryCmd = &cobra.Command{
	Use:   "category",
	Short: "Task categories",
}

var taskCategoryListCmd = &cobra.Command{
	Use:   "list",
	Short: "List task categories",
	RunE: func(cmd *cobra.Command, args []string) error {
		var resp client.TaskCategoryListResponse
		if err := newClient().Do("GET", "/v1/task-categories", nil, nil, &resp); err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(resp)
		}
		rows := make([][]string, 0, len(resp.Categories))
		for _, cat := range resp.Categories {
			rows = append(rows, []string{cat.ID, cat.Name, cat.Color})
		}
		output.Table([]string{"ID", "NAME", "COLOR"}, rows)
		return nil
	},
}

func init() {
	f := taskListCmd.Flags()
	f.StringVar(&taskListStatus, "status", "active", "active or trashed")
	f.BoolVar(&taskListShared, "shared", false, "tasks other people shared with you")
	f.StringVar(&taskListFrom, "from", "", "with --to: tasks due in this range, repeating tasks once per occurrence (YYYY-MM-DD, max 92 days)")
	f.StringVar(&taskListTo, "to", "", "end of the --from range (YYYY-MM-DD, inclusive)")
	f.StringSliceVar(&taskListCategories, "category", nil, "only tasks in these category IDs (comma-separated)")
	f.StringVar(&taskListSort, "sort", "due", "due or updated")
	f.IntVar(&taskListLimit, "limit", 50, "max tasks to return")
	f.StringVar(&taskListStartAfter, "start-after", "", "resume listing after this task ID (see the next page hint)")
	f.BoolVar(&taskListOpen, "open", false, "only tasks that aren't done")
	f.BoolVar(&taskListDone, "done", false, "only completed tasks")

	taskCreateCmd.Flags().StringVar(&taskDescription, "description", "", "task description")
	taskCreateCmd.Flags().StringVar(&taskDue, "due", "", `due date "YYYY-MM-DD" (all day) or "YYYY-MM-DD HH:MM" (local time)`)
	taskCreateCmd.Flags().StringVar(&taskCategory, "category", "", "category ID (see `task category list`)")

	taskUpdateCmd.Flags().StringVar(&taskTitle, "title", "", "new title")
	taskUpdateCmd.Flags().StringVar(&taskDescription, "description", "", "new description")
	taskUpdateCmd.Flags().StringVar(&taskDue, "due", "", `new due date "YYYY-MM-DD" (all day) or "YYYY-MM-DD HH:MM" (local time)`)
	taskUpdateCmd.Flags().BoolVar(&taskClearDue, "clear-due", false, "remove the due date")
	taskUpdateCmd.Flags().StringVar(&taskCategory, "category", "", "move to this category ID")
	taskUpdateCmd.Flags().BoolVar(&taskClearCategory, "clear-category", false, "remove the category")

	taskCategoryCmd.AddCommand(taskCategoryListCmd)
	taskCmd.AddCommand(taskListCmd, taskDueCmd, taskGetCmd, taskCreateCmd, taskUpdateCmd,
		completeCmd("done", "Mark a task done", true),
		completeCmd("reopen", "Mark a task not done", false),
		taskDeleteCmd, taskRestoreCmd, taskCategoryCmd)
	rootCmd.AddCommand(taskCmd)
}

// parseDay reads a YYYY-MM-DD date at local midnight.
func parseDay(s string) (time.Time, error) {
	d, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("%q isn't a YYYY-MM-DD date", s)
	}
	return d, nil
}

// parseDue turns --due into the API's due_at. A bare date is an all-day
// task at 09:00 local time, the same instant the web app stores.
func parseDue(s string) (string, bool, error) {
	s = strings.TrimSpace(s)
	if d, err := parseDay(s); err == nil {
		return d.Add(9 * time.Hour).UTC().Format(time.RFC3339), true, nil
	}
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02T15:04"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t.UTC().Format(time.RFC3339), false, nil
		}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format(time.RFC3339), false, nil
	}
	return "", false, fmt.Errorf(`%q isn't "YYYY-MM-DD" or "YYYY-MM-DD HH:MM"`, s)
}

// formatDue shows a due date in local time: the day alone for all-day
// tasks, the occurrence day for an expanded repeating task.
func formatDue(t client.Task) string {
	if t.DueAt == nil {
		return "-"
	}
	due, err := time.Parse(time.RFC3339Nano, *t.DueAt)
	if err != nil {
		return *t.DueAt
	}
	due = due.Local()
	day := due.Format("2006-01-02")
	if t.OccurrenceDate != nil {
		day = *t.OccurrenceDate
	}
	if t.AllDay {
		return day
	}
	return day + " " + due.Format("15:04")
}

func printTaskTable(c *client.Client, tasks []client.Task) {
	names := categoryNames(c, tasks)
	rows := make([][]string, 0, len(tasks))
	for _, t := range tasks {
		done := ""
		if t.Completed {
			done = "✓"
		}
		title := t.Title
		if t.Recurrence != nil {
			title += " ↻"
		}
		rows = append(rows, []string{t.ID, done, title, formatDue(t), names[ptr(t.CategoryID)]})
	}
	output.Table([]string{"ID", "DONE", "TITLE", "DUE", "CATEGORY"}, rows)
}

func printTaskDetail(c *client.Client, t client.Task) {
	names := categoryNames(c, []client.Task{t})
	status := "open"
	if t.Completed {
		status = "done"
	}
	if t.Status == "trashed" {
		status += " (in trash)"
	}
	fmt.Println(t.Title)
	fmt.Println()
	fmt.Printf("Status:    %s\n", status)
	fmt.Printf("Due:       %s\n", formatDue(t))
	if t.Recurrence != nil {
		fmt.Printf("Repeats:   every %d %s\n", max(t.Recurrence.Interval, 1), t.Recurrence.Freq)
	}
	if name := names[ptr(t.CategoryID)]; name != "" {
		fmt.Printf("Category:  %s\n", name)
	}
	if t.Location != nil {
		fmt.Printf("Location:  %s\n", t.Location.DisplayName)
	}
	if len(t.NoteIDs) > 0 {
		fmt.Printf("Notes:     %s\n", strings.Join(t.NoteIDs, ", "))
	}
	if t.Description != nil && *t.Description != "" {
		fmt.Printf("\n%s\n", *t.Description)
	}
}

// categoryNames maps category IDs to names for the table/detail view, one
// extra request and only when a task has a category. Failure just leaves
// the column blank.
func categoryNames(c *client.Client, tasks []client.Task) map[string]string {
	names := map[string]string{}
	need := false
	for _, t := range tasks {
		if t.CategoryID != nil {
			need = true
			break
		}
	}
	if !need {
		return names
	}
	var resp client.TaskCategoryListResponse
	if err := c.Do("GET", "/v1/task-categories", nil, nil, &resp); err != nil {
		return names
	}
	for _, cat := range resp.Categories {
		names[cat.ID] = cat.Name
	}
	return names
}

func ptr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func lastTaskID(tasks []client.Task, fallback string) string {
	if len(tasks) == 0 {
		return fallback
	}
	return tasks[len(tasks)-1].ID
}

// decryptTaskZK decrypts a Zero-Knowledge task's title and description in
// place (a no-op when dek is nil). Fails closed like decryptNoteZK: a task
// that doesn't decrypt shows zkUnreadableText, never the server's text.
func decryptTaskZK(dek []byte, t *client.Task) {
	if dek == nil {
		return
	}
	open := zkcrypto.NewOpener(dek, "task")
	title := t.Title
	if title != "" {
		decrypted, err := open.String("title", title)
		if err != nil {
			t.Title, t.Description, t.ZkUnreadable = zkUnreadableText, nil, true
			return
		}
		title = decrypted
	}
	if t.Description != nil && *t.Description != "" {
		decrypted, err := open.String("description", *t.Description)
		if err != nil {
			t.Title, t.Description, t.ZkUnreadable = zkUnreadableText, nil, true
			return
		}
		t.Description = &decrypted
	}
	t.Title = title
}
