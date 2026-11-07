package session

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func session(t *testing.T, h Host, height int) tea.Model {
	t.Helper()
	var m tea.Model = newModel(h)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 60, Height: height})
	return m
}

func start(m tea.Model, line string) tea.Model {
	mm := m.(model)
	mm.input.SetValue(line)
	m, _ = mm.submit()
	m, _ = m.Update(redrawMsg{}) // the session lays out after every message
	return m
}

func pane(m tea.Model) string { return ansi.Strip(m.(model).paneView()) }

func TestFailedCommandGetsARedDivider(t *testing.T) {
	m := start(session(t, Host{}, 20), "note get x")
	m, _ = m.Update(doneMsg{outcome: Failed(3)})
	if !strings.Contains(pane(m), "✗ exit 3 · ") {
		t.Fatalf("want the exit code on the divider, got %q", pane(m))
	}
	m = start(m, "whoami")
	m, _ = m.Update(doneMsg{outcome: Ran})
	if strings.Count(pane(m), "✗") != 1 {
		t.Fatal("a command that worked gets a plain divider")
	}
}

func TestCommandStaysPinnedWhileReadingItsOutput(t *testing.T) {
	m := start(session(t, Host{}, 12), "note list")
	for i := range 40 {
		m, _ = m.Update(outputMsg(fmt.Sprintf("line %d\n", i)))
	}
	m, _ = m.Update(doneMsg{outcome: Ran})
	first := func() string { r, _, _ := strings.Cut(pane(m), "\n"); return r }
	if !strings.HasPrefix(first(), "› note list ┄") {
		t.Fatalf("want the command pinned, got %q", first())
	}
	for range 20 {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	}
	if strings.Contains(first(), "┄") || !strings.HasPrefix(first(), "› note list") {
		t.Fatalf("at the command itself nothing is pinned, got %q", first())
	}
}

func TestPinnedRowIgnoresClicks(t *testing.T) {
	m := start(session(t, Host{}, 12), "note list")
	for i := range 40 {
		m, _ = m.Update(outputMsg(fmt.Sprintf("line %d\n", i)))
	}
	if _, ok := m.(model).cell(0, 1, true); ok {
		t.Fatal("the pinned row isn't the content under it")
	}
}

func typeKeys(m tea.Model, s string) tea.Model {
	for _, r := range s {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

func TestFindGoesThroughMatches(t *testing.T) {
	m := session(t, Host{}, 12)
	for i := range 40 {
		m, _ = m.Update(outputMsg(fmt.Sprintf("Line %d\n", i)))
	}
	m, _ = m.Update(outputMsg("needle one\n"))
	for range 30 {
		m, _ = m.Update(outputMsg("filler\n"))
	}
	m, _ = m.Update(outputMsg("NEEDLE two\n"))

	m = typeKeys(m, "/needle")
	if !m.(model).finding || len(m.(model).matches) != 2 {
		t.Fatalf("want 2 matches, got %v", m.(model).matches)
	}
	if !strings.Contains(pane(m), "NEEDLE two") || !strings.Contains(pane(m), "2 of 2") {
		t.Fatalf("searching starts at the newest match, got %q", pane(m))
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(pane(m), "needle one") || !strings.Contains(pane(m), "1 of 2") {
		t.Fatalf("Enter goes to the older match, got %q", pane(m))
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.(model).finding || m.(model).matches != nil || m.(model).follow {
		t.Fatal("Esc closes the search and stays where it is")
	}
}

func TestSlashOnlyOpensFindOnAnEmptyInput(t *testing.T) {
	m := typeKeys(session(t, Host{}, 12), "note /x")
	if m.(model).finding || m.(model).input.Value() != "note /x" {
		t.Fatalf("got %q", m.(model).input.Value())
	}
}

func TestFindRows(t *testing.T) {
	got := findRows([]string{"\x1b[31mçAbc\x1b[0m abc", "none"}, "ABC")
	if len(got) != 2 || got[0] != (match{0, 1, 4}) || got[1] != (match{0, 5, 8}) {
		t.Fatalf("got %v", got)
	}
	if findRows([]string{"abc"}, "") != nil {
		t.Fatal("an empty search finds nothing")
	}
}

func TestLinks(t *testing.T) {
	cases := map[string]string{
		"see https://a.example/x.":     "https://a.example/x",
		"(http://a.example/y)":         "http://a.example/y",
		"cut https://a.example/long…":  "",
		"file:///etc/passwd":           "",
		"javascript:alert(1)":          "",
		"https://a.example/z │ column": "https://a.example/z",
	}
	for text, want := range cases {
		got := ""
		if l := links(text); len(l) > 0 {
			got = text[l[0][0]:l[0][1]]
		}
		if got != want {
			t.Errorf("%q: got %q, want %q", text, got, want)
		}
	}
}

func TestClickOpensALinkButDoubleClickCopiesIt(t *testing.T) {
	var opened []string
	openLink = func(u string) error { opened = append(opened, u); return nil }
	t.Cleanup(func() { openLink = defaultOpenLink })

	m := session(t, Host{}, 12)
	m, _ = m.Update(outputMsg("go to https://a.example/x now\n"))
	m, _ = m.Update(tea.MouseClickMsg{X: 10, Y: 1, Button: tea.MouseLeft})
	m, cmd := m.Update(tea.MouseReleaseMsg{X: 10, Y: 1, Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatal("a click on a link should open it")
	}
	first := openMsg{"https://a.example/x", m.(model).clickTime}

	m = click(m, 10, 1) // the second click of a double-click
	if got := m.(model).selectedText(); got != "https://a.example/x" {
		t.Fatalf("selected %q", got)
	}
	m, cmd = m.Update(first)
	if cmd != nil {
		t.Fatal("a double-click must not also open the link")
	}
}

func TestLinkAtFollowsAWrappedLink(t *testing.T) {
	link := "https://a.example/" + strings.Repeat("x", 80)
	m := session(t, Host{}, 12)
	m, _ = m.Update(outputMsg(link + "\n"))
	if got := m.(model).linkAt(point{1, 5}); got != link {
		t.Fatalf("got %q", got)
	}
}

func TestListShowsAtOnceThenRefreshes(t *testing.T) {
	h := Host{Cacheable: func(string) bool { return true }}
	m := start(session(t, h, 20), "note list")
	m, _ = m.Update(outputMsg("first note\nsecond note\n"))
	m, _ = m.Update(doneMsg{outcome: Ran})

	m = start(m, "note list")
	if strings.Count(pane(m), "first note") != 2 || !strings.Contains(pane(m), "Refreshing note list") {
		t.Fatalf("want the last result right away, got %q", pane(m))
	}
	m, _ = m.Update(outputMsg("third note\n"))
	m, _ = m.Update(doneMsg{outcome: Ran})
	if strings.Count(pane(m), "first note") != 1 || !strings.Contains(pane(m), "third note") {
		t.Fatalf("the refreshed result replaces it, got %q", pane(m))
	}
}

func TestCachedListGoesOnFailureAndSignIn(t *testing.T) {
	h := Host{
		Cacheable:    func(l string) bool { return l == "note list" },
		RefreshAfter: func(l string) bool { return l == "logout" },
	}
	run := func(m tea.Model, line string, o Outcome) tea.Model {
		m = start(m, line)
		m, _ = m.Update(outputMsg("output\n"))
		m, _ = m.Update(doneMsg{outcome: o})
		return m
	}
	m := run(session(t, h, 20), "note list", Ran)
	if len(m.(model).cache) != 1 {
		t.Fatal("kept")
	}
	m = run(m, "note list", Failed(1))
	if len(m.(model).cache) != 0 {
		t.Fatal("a failure drops what was kept")
	}
	m = run(m, "note list", Ran)
	m = run(m, "note get 1", Ran)
	if len(m.(model).cache) != 1 {
		t.Fatal("only cacheable lines are kept")
	}
	m = run(m, "logout", Ran)
	if len(m.(model).cache) != 0 {
		t.Fatal("signing out empties it")
	}
}

func TestExpiredOutputIsNotShown(t *testing.T) {
	m := session(t, Host{}, 20).(model)
	m.cache["note list"] = cachedOutput{text: "old", at: time.Now().Add(-cacheFor - time.Second)}
	m.showCached("note list")
	if m.stale != 0 || len(m.cache) != 0 {
		t.Fatal("too old to show")
	}
}

func TestClearDuringARefreshKeepsThePaneIntact(t *testing.T) {
	m := session(t, Host{}, 20).(model)
	m.cache["note list"] = cachedOutput{text: "a\nb", at: time.Now()}
	var tm tea.Model = start(m, "note list")
	tm, _ = tm.Update(tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
	tm, _ = tm.Update(outputMsg("fresh\n"))
	if !strings.Contains(pane(tm), "fresh") {
		t.Fatalf("got %q", pane(tm))
	}
}
