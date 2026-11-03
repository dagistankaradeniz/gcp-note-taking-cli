package cmd

import (
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/banner"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/pager"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/shell"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/style"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"
)

// Bare `quillink` on an interactive terminal opens a session: the
// animated banner, then a `quillink ›` prompt that runs subcommands
// without the `quillink` prefix. Anything non-interactive (a pipe, a
// script, --json) keeps the old behaviour of printing help, so scripts
// never see the banner. One-shot `quillink note list` is unchanged.

// shellActive is set while the session runs: it stops a nested session
// and turns on the session-scoped Zero-Knowledge unlock (zk.go).
var shellActive bool

// shellBuiltins are handled by the session itself, not the command tree.
var shellBuiltins = []string{"clear", "exit", "lock", "quit"}

const shellHistoryLimit = 500

func init() {
	rootCmd.RunE = runRoot
	cobra.AddTemplateFunc("versionText", versionText)
	rootCmd.SetVersionTemplate(`{{versionText .}}`)
}

func runRoot(cmd *cobra.Command, _ []string) error {
	if shellActive || jsonOutput || !isTerminalStdin() || !style.IsTerminal(os.Stdout) {
		return cmd.Help()
	}
	return runShell()
}

// versionText is the banner without the animation on a terminal, and
// Cobra's usual "quillink version X" line everywhere else.
func versionText(c *cobra.Command) string {
	if !style.IsTerminal(os.Stdout) {
		return fmt.Sprintf("%s version %s\n", c.Name(), c.Version)
	}
	return banner.Header(style.For(os.Stdout), bannerInfo(), terminalColumns()) + "\n"
}

func bannerInfo() banner.Info {
	commit, built := client.BuildStamp()
	return banner.Info{Version: client.Version, Commit: commit, Built: built}
}

func terminalColumns() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 80
	}
	return w
}

// shellSuggestions is decided from local state only (is a credential
// stored or set?), never a network call. `login` is always offered.
func shellSuggestions() []banner.Suggestion {
	if resolveToken() == "" {
		return []banner.Suggestion{
			{Command: "login", Desc: "sign in to Quillink"},
			{Command: "workspace list", Desc: "your workspaces"},
			{Command: "workspace add", Args: "<name>", Desc: "add a workspace"},
			{Command: "help", Args: "[command]", Desc: "all commands"},
			{Command: "exit", Desc: "leave the session"},
		}
	}
	return []banner.Suggestion{
		{Command: "note list", Desc: "your recent notes"},
		{Command: "note search", Args: "<text>", Desc: "find notes"},
		{Command: "note get", Args: "<id>", Desc: "open a note"},
		{Command: "note create", Args: "<title>", Desc: "start a note"},
		{Command: "task list", Args: "--open", Desc: "your open tasks"},
		{Command: "task create", Args: "<title>", Desc: "add a task"},
		{Command: "tag list", Desc: "tags and counts"},
		{Command: "folder list", Desc: "your folders"},
		{Command: "whoami", Desc: "who's signed in"},
		{Command: "help", Args: "[command]", Desc: "all commands"},
		{Command: "login", Desc: "sign in or switch account"},
		{Command: "logout", Desc: "sign out"},
		{Command: "exit", Desc: "leave the session"},
	}
}

func runShell() error {
	shellActive = true
	defer func() {
		zkSessionWipe()
		shellActive = false
	}()

	out := os.Stdout
	p := style.For(out)
	cols := terminalColumns()
	fd := int(os.Stdin.Fd())

	// Full screen: the alternate screen hides the user's earlier terminal
	// lines and gives them back untouched when the session ends, however
	// it ends.
	fmt.Fprint(out, altScreenOn)
	restore := func() { fmt.Fprint(out, "\033[?25h"+altScreenOff) }
	defer restore()
	defer func() {
		if r := recover(); r != nil {
			restore()
			panic(r)
		}
	}()

	if cols >= banner.Width {
		net := banner.NewNetwork(rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())))
		fmt.Fprintln(out)
		if os.Getenv("QUILLINK_NO_ANIMATION") != "" {
			for _, line := range net.Render(p, net.Final()) {
				fmt.Fprintln(out, line)
			}
		} else {
			playSkippable(net, out, p)
		}
	}
	fmt.Fprint(out, banner.Compose(p, bannerInfo(), cols, shellSuggestions()))
	fmt.Fprintln(out)

	// Ctrl-C while a command runs (or a terminal hang-up) ends the
	// session; the screen and terminal mode are put back first.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sig)
	go func() {
		if _, ok := <-sig; ok {
			zkSessionWipe()
			restore()
			os.Exit(130)
		}
	}()

	t := term.NewTerminal(struct {
		io.Reader
		io.Writer
	}{os.Stdin, out}, p.BrandBold("quillink ›")+" ")
	t.History = shell.NewHistory(shellHistoryLimit)
	t.AutoCompleteCallback = completeShellLine

	for {
		if w, h, err := term.GetSize(int(out.Fd())); err == nil {
			_ = t.SetSize(w, h)
		}
		line, err := readShellLine(fd, t)
		if errors.Is(err, io.EOF) {
			fmt.Fprintln(out)
			return nil
		}
		if err != nil {
			return err
		}
		args, err := shell.Split(line)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			continue
		}
		if len(args) > 0 && args[0] == rootCmd.Name() {
			args = args[1:]
		}
		if len(args) == 0 {
			continue
		}
		switch args[0] {
		case "exit", "quit":
			return nil
		case "clear":
			fmt.Fprint(out, clearScreen)
			continue
		case "lock":
			if zkSessionWipe() {
				fmt.Fprintln(out, p.Dim("Locked: unlocked keys are cleared from memory."))
			} else {
				fmt.Fprintln(out, p.Dim("Nothing is unlocked."))
			}
			continue
		}
		runShellCommand(fd, args)
	}
}

const (
	altScreenOn  = "\033[?1049h\033[H"
	altScreenOff = "\033[?1049l"
	clearScreen  = "\033[H\033[2J"
)

// playSkippable runs the animation; Ctrl-C skips to the final frame
// instead of quitting.
func playSkippable(net banner.Network, out *os.File, p style.Palette) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)
	stop := make(chan struct{})
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-sig:
			close(stop)
		case <-done:
		}
	}()
	net.Play(out, p, stop)
}

// readShellLine reads one line in raw mode (needed for history and
// completion) and puts the terminal back before the command runs, so
// command output and password prompts behave normally.
func readShellLine(fd int, t *term.Terminal) (string, error) {
	state, err := term.MakeRaw(fd)
	if err != nil {
		return "", err
	}
	defer func() { _ = term.Restore(fd, state) }()
	return t.ReadLine()
}

// streamingCommands talk to the user while they run (the device-login
// code and "waiting for approval"), so their output isn't held back for
// the pager.
var streamingCommands = map[string]bool{
	"quillink login":           true,
	"quillink workspace login": true,
}

// runShellCommand runs one command. Its output is collected first: when
// it's taller than the screen it opens in the pager (the full-screen
// session has no scrollback), otherwise it's printed as usual.
func runShellCommand(fd int, args []string) {
	resetFlags(rootCmd)
	rootCmd.SetArgs(args)
	if c, _, err := rootCmd.Find(args); err == nil && streamingCommands[c.CommandPath()] {
		if err := finishOutput(rootCmd.Execute()); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
		}
		return
	}

	text, err := captureStdout(func() error { return finishOutput(rootCmd.Execute()) })
	if err != nil {
		defer fmt.Fprintln(os.Stderr, "Error:", err)
	}
	cols, height, sizeErr := term.GetSize(int(os.Stdout.Fd()))
	rows := pager.Wrap(text, cols)
	if sizeErr != nil || len(rows) < height-1 {
		fmt.Fprint(os.Stdout, text)
		return
	}
	state, rawErr := term.MakeRaw(fd)
	if rawErr != nil {
		fmt.Fprint(os.Stdout, text)
		return
	}
	_ = pager.Show(os.Stdin, os.Stdout, style.For(os.Stdout), rows, height)
	_ = term.Restore(fd, state)
	// Back at the prompt, the end of the output stays in view.
	tail := rows[max(len(rows)-(height-2), 0):]
	fmt.Fprint(os.Stdout, clearScreen+strings.Join(tail, "\033[0m\n")+"\033[0m\n")
}

// captureStdout runs fn with stdout redirected into a pipe that still
// counts as the terminal for colour, and returns what was written.
func captureStdout(fn func() error) (string, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return "", fn()
	}
	prev := os.Stdout
	os.Stdout = w
	style.TreatAsTerminal(w)
	var buf strings.Builder
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()
	runErr := fn()
	os.Stdout = prev
	style.TreatAsTerminal(nil)
	_ = w.Close()
	<-done
	_ = r.Close()
	return buf.String(), runErr
}

// resetFlags puts every flag back to its default, so a flag given to one
// command in the session doesn't carry over to the next.
func resetFlags(c *cobra.Command) {
	reset := func(f *pflag.Flag) {
		if !f.Changed {
			return
		}
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			var def []string
			if d := strings.Trim(f.DefValue, "[]"); d != "" {
				def = strings.Split(d, ",")
			}
			_ = sv.Replace(def)
		} else {
			_ = f.Value.Set(f.DefValue)
		}
		f.Changed = false
	}
	c.Flags().VisitAll(reset)
	c.PersistentFlags().VisitAll(reset)
	for _, sub := range c.Commands() {
		resetFlags(sub)
	}
}

// completeShellLine completes the word before the cursor on Tab: a
// command or subcommand name, or a flag of the command typed so far.
func completeShellLine(line string, pos int, key rune) (string, int, bool) {
	if key != '\t' {
		return "", 0, false
	}
	before := line[:pos]
	fields := strings.Fields(before)
	word := ""
	if len(fields) > 0 && !strings.HasSuffix(before, " ") {
		word = fields[len(fields)-1]
		fields = fields[:len(fields)-1]
	}
	if len(fields) > 0 && fields[0] == rootCmd.Name() {
		fields = fields[1:]
	}
	c := rootCmd
	for _, f := range fields {
		if strings.HasPrefix(f, "-") {
			continue
		}
		next := findChild(c, f)
		if next == nil {
			break
		}
		c = next
	}
	var cands []string
	if strings.HasPrefix(word, "-") {
		add := func(f *pflag.Flag) {
			if !f.Hidden {
				cands = append(cands, "--"+f.Name)
			}
		}
		c.Flags().VisitAll(add)
		c.InheritedFlags().VisitAll(add)
	} else {
		for _, sub := range c.Commands() {
			if sub.IsAvailableCommand() || sub.Name() == "help" {
				cands = append(cands, sub.Name())
			}
		}
		if c == rootCmd && len(fields) == 0 {
			cands = append(cands, shellBuiltins...)
		}
	}
	completed, ok := shell.Complete(word, cands)
	if !ok {
		return "", 0, false
	}
	head := before[:len(before)-len(word)] + completed
	return head + line[pos:], len(head), true
}

func findChild(c *cobra.Command, name string) *cobra.Command {
	for _, sub := range c.Commands() {
		if sub.Name() == name || sub.HasAlias(name) {
			return sub
		}
	}
	return nil
}
