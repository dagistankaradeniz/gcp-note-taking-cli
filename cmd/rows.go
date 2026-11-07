package cmd

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/charmbracelet/x/ansi"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/session"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/shell"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/ui"
	"github.com/spf13/cobra"
)

// The session remembers what its tables listed, in memory only, so an
// item can be picked by row number (`note get 3`) and ids complete with
// Tab, without asking the server for anything.

// maxRecent bounds the ids kept per kind for completion.
const maxRecent = 200

type listedItem struct{ id, title string }

type listing struct {
	rows   []listedItem // the last table, in order
	recent []listedItem // everything seen lately, newest first
}

var (
	listingsMu sync.Mutex
	listings   = map[string]*listing{}
	// listingKind is what the running command's table lists.
	listingKind string
)

// rememberRows is output.Listed. Titles read with an unlocked key aren't
// kept: they must not outlive the key in the completion list.
func rememberRows(ids, titles []string) {
	if anyUnlocked() {
		titles = make([]string, len(ids))
	}
	listingsMu.Lock()
	defer listingsMu.Unlock()
	l := listings[listingKind]
	if l == nil {
		l = &listing{}
		listings[listingKind] = l
	}
	l.rows = make([]listedItem, len(ids))
	seen := make(map[string]bool, len(ids))
	for i, id := range ids {
		l.rows[i] = listedItem{id, titles[i]}
		seen[id] = true
	}
	recent := append([]listedItem(nil), l.rows...)
	for _, it := range l.recent {
		if !seen[it.id] && len(recent) < maxRecent {
			recent = append(recent, it)
		}
	}
	l.recent = recent
}

// forgetRows drops everything remembered, as the sign-in changed.
func forgetRows() {
	listingsMu.Lock()
	defer listingsMu.Unlock()
	listings = map[string]*listing{}
}

// kindOf is what a command's ids refer to.
func kindOf(c *cobra.Command) string {
	words := strings.Fields(c.CommandPath())[1:]
	switch {
	case len(words) == 0:
		return ""
	case len(words) > 1 && words[0] == "note" && words[1] == "attachment":
		return "attachment"
	case len(words) > 1 && words[0] == "task" && words[1] == "category":
		return "category"
	case words[0] == "tag" || words[0] == "shared":
		return "note" // these list notes
	}
	return words[0]
}

// idSlots is the kind of each positional argument of c from its usage
// ("download <note-id> <attachment-id>"), "" for one that isn't an id.
func idSlots(c *cobra.Command) []string {
	words := strings.Fields(c.Use)[1:]
	slots := make([]string, len(words))
	for i, w := range words {
		name := strings.Trim(w, "<>[]")
		switch {
		case name == "id":
			slots[i] = kindOf(c)
		case strings.HasSuffix(name, "-id"):
			slots[i] = strings.TrimSuffix(name, "-id")
		}
	}
	return slots
}

// walkLine finds the command a line reaches and the indexes of its
// positional arguments, skipping flags and their values.
func walkLine(args []string) (*cobra.Command, []int) {
	c := rootCmd
	inPath := true
	var positional []int
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") && a != "-" {
			name, _, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
			if f := lookupFlag(c, name, !strings.HasPrefix(a, "--")); f != nil && !hasValue && f.Value.Type() != "bool" {
				i++
			}
			continue
		}
		if inPath {
			if next := findChild(c, a); next != nil {
				c = next
				continue
			}
			inPath = false
		}
		positional = append(positional, i)
	}
	return c, positional
}

func commandFor(args []string) *cobra.Command {
	c, _ := walkLine(args)
	return c
}

var rowNumber = regexp.MustCompile(`^[1-9][0-9]{0,3}$`)

// pickRows swaps row numbers in id arguments for the ids they stand for,
// and says which item each one picked.
func pickRows(args []string) ([]string, error) {
	c, positional := walkLine(args)
	slots := idSlots(c)
	var out []string
	for n, i := range positional {
		if n >= len(slots) || slots[n] == "" || !rowNumber.MatchString(args[i]) {
			continue
		}
		item, err := rowOf(slots[n], args[i])
		if err != nil {
			return nil, err
		}
		if out == nil {
			out = append([]string(nil), args...)
		}
		out[i] = item.id
		ui.Notice(os.Stderr, fmt.Sprintf("Row %s: %s", args[i], strings.TrimSpace(item.id+"  "+item.title)))
	}
	if out == nil {
		return args, nil
	}
	return out, nil
}

func rowOf(kind, number string) (listedItem, error) {
	listingsMu.Lock()
	defer listingsMu.Unlock()
	l := listings[kind]
	if l == nil {
		return listedItem{}, fmt.Errorf("no %s list to pick row %s from -- list them first, or use the id", kind, number)
	}
	n, _ := strconv.Atoi(number)
	if n > len(l.rows) {
		return listedItem{}, fmt.Errorf("the last %s list has %d rows, so there's no row %s", kind, len(l.rows), number)
	}
	return l.rows[n-1], nil
}

// suggestIDs completes an id argument from what's been listed: word
// matches the start of an id, part of a title, or a row number. nil means
// the word isn't an id argument, or nothing is known for it.
func suggestIDs(fields []string, word string) []session.Suggestion {
	kind := ""
	if n := len(fields); n > 0 && fields[n-1] == "--folder" {
		kind = "folder"
	} else {
		c, positional := walkLine(fields)
		if slots := idSlots(c); len(positional) < len(slots) {
			kind = slots[len(positional)]
		}
	}
	if kind == "" {
		return nil
	}
	listingsMu.Lock()
	defer listingsMu.Unlock()
	l := listings[kind]
	if l == nil {
		return nil
	}
	needle := strings.ToLower(word)
	number := map[string]string{}
	for i, it := range l.rows {
		number[it.id] = strconv.Itoa(i + 1)
	}
	var out []session.Suggestion
	for _, it := range l.recent {
		title := ansi.Strip(it.title)
		if strings.HasPrefix(strings.ToLower(it.id), needle) || number[it.id] == word ||
			needle != "" && strings.Contains(strings.ToLower(title), needle) {
			out = append(out, session.Suggestion{Text: it.id, Desc: title})
		}
	}
	return out
}

// cacheable are the read-only lists shown at once when run again.
var cacheable = map[string]bool{
	"note list": true, "task list": true, "task due": true,
	"tag list": true, "folder list": true, "canvas list": true,
}

// cacheableLine says whether line's output may be kept. Decrypted content
// never is: nothing unlocked may stay readable after the key is wiped.
func cacheableLine(line string) bool {
	args, err := shell.Split(line)
	if err != nil || len(args) < 2 || !cacheable[args[0]+" "+args[1]] || shell.HasSecretFlag(args) {
		return false
	}
	for _, a := range args {
		name, _, _ := strings.Cut(a, "=")
		if name == "--out" || name == "--output" || name == "--json" {
			return false
		}
	}
	return !anyUnlocked()
}

// anyUnlocked reports whether a Zero-Knowledge or Vault key is in memory.
func anyUnlocked() bool {
	owner := zkSessionOwner()
	return zkSession.get(owner) != nil || vaultSession.get(owner) != nil
}

// sessionTokens keeps stored credentials in memory for the session, as
// reading the OS keychain for every command is slow.
var sessionTokens tokenMemo

type tokenMemo struct {
	sync.Mutex
	byWorkspace map[string]string
}

func (t *tokenMemo) get(workspace string) (string, bool) {
	t.Lock()
	defer t.Unlock()
	token, ok := t.byWorkspace[workspace]
	return token, ok
}

func (t *tokenMemo) set(workspace, token string) {
	t.Lock()
	defer t.Unlock()
	if t.byWorkspace == nil {
		t.byWorkspace = map[string]string{}
	}
	t.byWorkspace[workspace] = token
}

// forget is for after a sign-in changes: what was listed goes too.
func (t *tokenMemo) forget() {
	t.Lock()
	t.byWorkspace = nil
	t.Unlock()
	forgetRows()
}
