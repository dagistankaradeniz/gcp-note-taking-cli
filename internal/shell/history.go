package shell

// History is the session's in-memory command history (up/down arrows). It
// is never written to disk, and lines that pass a credential as a literal
// flag (HasSecretFlag) are dropped instead of recorded. It implements
// golang.org/x/term's History interface.
type History struct {
	entries []string // oldest first
	limit   int
}

// NewHistory keeps at most limit entries.
func NewHistory(limit int) *History { return &History{limit: limit} }

// Add records entry unless it is blank, repeats the previous one, or
// carries a secret.
func (h *History) Add(entry string) {
	args, err := Split(entry)
	if err != nil || len(args) == 0 || HasSecretFlag(args) {
		return
	}
	if n := len(h.entries); n > 0 && h.entries[n-1] == entry {
		return
	}
	h.entries = append(h.entries, entry)
	if len(h.entries) > h.limit {
		h.entries = h.entries[1:]
	}
}

// Len is the number of entries.
func (h *History) Len() int { return len(h.entries) }

// At returns entry idx, 0 being the most recent.
func (h *History) At(idx int) string { return h.entries[len(h.entries)-1-idx] }
