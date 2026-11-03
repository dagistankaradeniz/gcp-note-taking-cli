package cmd

import (
	"fmt"
	"net/url"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/zkcrypto"
)

// Zero-Knowledge writes: content is encrypted here, exactly as the web's
// zkFieldCrypto.ts does (envelope v2, one binding id per item, Padmé
// padding), and the server refuses anything else for a ZK account. There
// is never a plaintext fallback: without the unlocked key, a write fails.
//
// Binding ids: a new item gets a fresh one; a write that replaces every
// encrypted field may too; any other edit must reuse the item's own, which
// is read by opening its current ciphertext -- so an item that doesn't
// decrypt is never written over.

// sealNoteCreate encrypts a new note's title, body and tags, and adds the
// body's link ids when the account has the Knowledge Graph turned on.
func sealNoteCreate(c *client.Client, dek []byte, req *client.NoteCreateRequest) error {
	bind, err := zkcrypto.NewBindingID()
	if err != nil {
		return err
	}
	if req.Title, err = zkcrypto.SealString(dek, "note", "title", bind, req.Title); err != nil {
		return err
	}
	if req.Body != nil {
		if graphOn, err := zkGraphOptIn(c); err != nil {
			return err
		} else if graphOn {
			req.LinksTo, req.CanvasLinksTo = extractGraphLinks(req.Body)
		}
		if req.Body, err = zkcrypto.SealBody(dek, bind, req.Body); err != nil {
			return err
		}
	}
	req.Tags, err = sealTags(dek, bind, req.Tags)
	return err
}

// sealNoteUpdate encrypts the fields an update changes, under the note's
// own binding id unless it replaces title, body and tags together.
func sealNoteUpdate(c *client.Client, dek []byte, id string, req *client.NoteUpdateRequest) error {
	if req.Title == nil && req.Body == nil && req.Tags == nil {
		return nil
	}
	var bind string
	var err error
	if req.Title != nil && req.Body != nil && req.Tags != nil {
		bind, err = zkcrypto.NewBindingID()
	} else {
		bind, err = noteBind(c, dek, id)
	}
	if err != nil {
		return err
	}
	if req.Title != nil {
		sealed, err := zkcrypto.SealString(dek, "note", "title", bind, *req.Title)
		if err != nil {
			return err
		}
		req.Title = &sealed
	}
	if req.Body != nil {
		if graphOn, err := zkGraphOptIn(c); err != nil {
			return err
		} else if graphOn {
			notes, canvases := extractGraphLinks(req.Body)
			req.LinksTo, req.CanvasLinksTo = &notes, &canvases
		}
		if req.Body, err = zkcrypto.SealBody(dek, bind, req.Body); err != nil {
			return err
		}
	}
	if req.Tags != nil {
		sealed, err := sealTags(dek, bind, *req.Tags)
		if err != nil {
			return err
		}
		req.Tags = &sealed
	}
	return nil
}

// noteBind opens the note's current fields and returns their binding id.
func noteBind(c *client.Client, dek []byte, id string) (string, error) {
	var n client.Note
	if err := c.Do("GET", "/v1/notes/"+url.PathEscape(id), nil, nil, &n); err != nil {
		return "", err
	}
	open := zkcrypto.NewOpener(dek, "note")
	var err error
	switch {
	case n.Title != "":
		_, err = open.String("title", n.Title)
	case n.Body != nil:
		_, err = open.Body("body", n.Body)
	case len(n.Tags) > 0:
		_, err = open.String("tags", n.Tags[0])
	}
	if err != nil || open.Bind() == "" {
		return "", fmt.Errorf("note %s can't be decrypted on this device, so it isn't changed -- set its title, content and tags together to replace it", id)
	}
	return open.Bind(), nil
}

func sealTags(dek []byte, bind string, tags []string) ([]string, error) {
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		sealed, err := zkcrypto.SealString(dek, "note", "tags", bind, t)
		if err != nil {
			return nil, err
		}
		out = append(out, sealed)
	}
	return out, nil
}

// sealTaskWrite encrypts a task's title/description in a create or update
// body (encryptTaskWrite): fresh binding id for a new task or when both
// fields are written, otherwise the task's own.
func sealTaskWrite(c *client.Client, dek []byte, id string, body map[string]any) error {
	title, sealsTitle := body["title"].(string)
	desc, _ := body["description"].(string)
	sealsDescription := desc != ""
	if d, ok := body["description"]; ok && d == "" {
		// Cleared: nothing to encrypt, and an empty string isn't ciphertext.
		body["description"] = nil
	}
	if !sealsTitle && !sealsDescription {
		return nil
	}
	_, descriptionSet := body["description"]
	var bind string
	var err error
	if id == "" || (sealsTitle && descriptionSet) {
		bind, err = zkcrypto.NewBindingID()
	} else {
		bind, err = taskBind(c, dek, id)
	}
	if err != nil {
		return err
	}
	if sealsTitle {
		if body["title"], err = zkcrypto.SealString(dek, "task", "title", bind, title); err != nil {
			return err
		}
	}
	if sealsDescription {
		if body["description"], err = zkcrypto.SealString(dek, "task", "description", bind, desc); err != nil {
			return err
		}
	}
	return nil
}

func taskBind(c *client.Client, dek []byte, id string) (string, error) {
	var t client.Task
	if err := c.Do("GET", "/v1/tasks/"+url.PathEscape(id), nil, nil, &t); err != nil {
		return "", err
	}
	open := zkcrypto.NewOpener(dek, "task")
	var err error
	switch {
	case t.Title != "":
		_, err = open.String("title", t.Title)
	case t.Description != nil && *t.Description != "":
		_, err = open.String("description", *t.Description)
	}
	if err != nil || open.Bind() == "" {
		return "", fmt.Errorf("task %s can't be decrypted on this device, so it isn't changed -- set its title and description together to replace it", id)
	}
	return open.Bind(), nil
}

// sealFolderName: a folder's name is its only encrypted field, so every
// write gets a fresh binding id (encryptFolderWrite).
func sealFolderName(dek []byte, name string) (string, error) {
	bind, err := zkcrypto.NewBindingID()
	if err != nil {
		return "", err
	}
	return zkcrypto.SealString(dek, "folder", "name", bind, name)
}

// zkGraphOptIn: whether a ZK account shares its link ids with the server
// (the Knowledge Graph setting).
func zkGraphOptIn(c *client.Client) (bool, error) {
	var status client.ZkStatusResponse
	if err := c.Do("GET", "/v1/zk/status", nil, nil, &status); err != nil {
		return false, err
	}
	return status.ZkGraphOptIn, nil
}

// extractGraphLinks is extractGraphLinks.ts: the noteLink ids in a body,
// split into notes and canvases, first-seen order, no duplicates.
func extractGraphLinks(body map[string]any) (notes, canvases []string) {
	notes, canvases = []string{}, []string{}
	seen := map[string]bool{}
	var walk func(n map[string]any)
	walk = func(n map[string]any) {
		if n["type"] == "noteLink" {
			attrs, _ := n["attrs"].(map[string]any)
			if id, _ := attrs["id"].(string); id != "" {
				if attrs["refType"] == "canvas" {
					if !seen["c:"+id] {
						seen["c:"+id] = true
						canvases = append(canvases, id)
					}
				} else if !seen["n:"+id] {
					seen["n:"+id] = true
					notes = append(notes, id)
				}
			}
		}
		children, _ := n["content"].([]any)
		for _, ch := range children {
			if m, ok := ch.(map[string]any); ok {
				walk(m)
			}
		}
	}
	if body != nil {
		walk(body)
	}
	return notes, canvases
}

// sealTaskCreate encrypts a new task's title and description under a fresh
// binding id. A ZK task has no due date or category (the server refuses
// them too), so those are refused here first, with a clearer message.
func sealTaskCreate(dek []byte, req *client.TaskCreateRequest) error {
	if err := refuseZkTaskScheduling(req.DueAt != nil, req.CategoryID != nil); err != nil {
		return err
	}
	bind, err := zkcrypto.NewBindingID()
	if err != nil {
		return err
	}
	if req.Title, err = zkcrypto.SealString(dek, "task", "title", bind, req.Title); err != nil {
		return err
	}
	if req.Description != nil && *req.Description != "" {
		sealed, err := zkcrypto.SealString(dek, "task", "description", bind, *req.Description)
		if err != nil {
			return err
		}
		req.Description = &sealed
	}
	return nil
}

func refuseZkTaskScheduling(due, category bool) error {
	switch {
	case due:
		return fmt.Errorf("due dates aren't available for Zero-Knowledge accounts")
	case category:
		return fmt.Errorf("task categories aren't available for Zero-Knowledge accounts")
	}
	return nil
}
