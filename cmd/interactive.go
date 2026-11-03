package cmd

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/banner"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/progress"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/session"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/shell"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/style"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/ui"
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

	// Asked before the session owns the keyboard: the answer comes back
	// on stdin.
	ui.SetDark(ui.Dark())
	ui.SetWidth(cols)

	// A terminal hang-up or kill still clears unlocked keys.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sig)
	go func() {
		if _, ok := <-sig; ok {
			zkSessionWipe()
			os.Exit(130)
		}
	}()

	// No separate network spinner: the session already shows the running
	// command with its own.
	client.Busy = nil

	err := session.Run(session.Host{
		Run:          runSessionLine,
		Suggest:      suggestShellLine,
		Status:       sessionStatus,
		RefreshAfter: changesStatus,
		History:      shell.NewHistory(shellHistoryLimit),
	})
	if errors.Is(err, session.ErrInterrupted) {
		zkSessionWipe()
		os.Exit(130)
	}
	return err
}

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

// runSessionLine runs one line typed in the session: a built-in, or a
// command through the same Cobra tree as one-shot use.
func runSessionLine(line string) session.Outcome {
	args, err := shell.Split(line)
	if err != nil {
		printError(err)
		return session.Ran
	}
	if len(args) > 0 && args[0] == rootCmd.Name() {
		args = args[1:]
	}
	if len(args) == 0 {
		return session.Ran
	}
	switch args[0] {
	case "exit", "quit":
		return session.Quit
	case "clear":
		return session.Clear
	case "lock":
		if zkSessionWipe() {
			success("Locked: unlocked keys are cleared from memory.")
		} else {
			ui.Notice(os.Stdout, "Nothing is unlocked.")
		}
		return session.Ran
	}
	resetFlags(rootCmd)
	rootCmd.SetArgs(args)
	if err := finishOutput(rootCmd.Execute()); err != nil {
		printError(err)
	}
	progress.Reset()
	return session.Ran
}

// changesStatus: lines after which the status line is read again.
func changesStatus(line string) bool {
	args, _ := shell.Split(line)
	for _, a := range args {
		switch a {
		case "login", "logout", "use", "remove", "unlock", "lock", "vault", "zk":
			return true
		}
	}
	return false
}

// sessionStatus is the line under the input: where commands go, who is
// signed in, and what's unlocked. It's read from local state plus one
// /v1/status call, never more.
func sessionStatus() session.Status {
	st := session.Status{Workspace: workspaceLabel()}
	owner := zkSessionOwner()
	if zkSession.get(owner) != nil {
		st.Unlocked = append(st.Unlocked, "Zero-Knowledge")
	}
	if vaultSession.get(owner) != nil {
		st.Unlocked = append(st.Unlocked, "Vault")
	}
	if resolveToken() == "" {
		return st
	}
	saved := client.Busy
	client.Busy = nil // the status line has no spinner of its own
	var status client.StatusResponse
	err := newClient().Do("GET", "/v1/status", nil, nil, &status)
	client.Busy = saved
	if err != nil {
		st.Account = "signed in"
		return st
	}
	st.Account = status.Email
	if st.Account == "" {
		st.Account = credentialName(status.CredentialType)
	}
	st.Plan = planName(status.Plan)
	return st
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

// suggestShellLine lists what can come at the word before the cursor --
// a command or subcommand with its summary, or a flag of the command
// typed so far with its usage -- for the session's dropdown and Tab.
func suggestShellLine(line string, pos int) ([]session.Suggestion, int) {
	before := line[:pos]
	fields := strings.Fields(before)
	word := ""
	if len(fields) > 0 && !strings.HasSuffix(before, " ") {
		word = fields[len(fields)-1]
		fields = fields[:len(fields)-1]
	}
	from := len(before) - len(word)
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
	var out []session.Suggestion
	if strings.HasPrefix(word, "-") {
		add := func(f *pflag.Flag) {
			if !f.Hidden && strings.HasPrefix("--"+f.Name, word) {
				out = append(out, session.Suggestion{Text: "--" + f.Name, Desc: f.Usage})
			}
		}
		c.Flags().VisitAll(add)
		c.InheritedFlags().VisitAll(add)
		return out, from
	}
	if c.HasAvailableSubCommands() || c == rootCmd {
		for _, sub := range c.Commands() {
			if (sub.IsAvailableCommand() || sub.Name() == "help") && strings.HasPrefix(sub.Name(), word) {
				out = append(out, session.Suggestion{Text: sub.Name(), Desc: sub.Short})
			}
		}
		if c == rootCmd && len(fields) == 0 {
			for _, b := range shellBuiltins {
				if strings.HasPrefix(b, word) {
					out = append(out, session.Suggestion{Text: b, Desc: builtinDesc[b]})
				}
			}
		}
	}
	return out, from
}

var builtinDesc = map[string]string{
	"clear": "clear the screen",
	"exit":  "leave the session",
	"lock":  "clear unlocked keys from memory",
	"quit":  "leave the session",
}

func findChild(c *cobra.Command, name string) *cobra.Command {
	for _, sub := range c.Commands() {
		if sub.Name() == name || sub.HasAlias(name) {
			return sub
		}
	}
	return nil
}
