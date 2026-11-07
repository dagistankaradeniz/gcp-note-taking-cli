package cmd

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/banner"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
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

	p := style.For(os.Stdout)
	cols := terminalColumns()
	intro := introFrames(p, cols)

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
	client.KeepConnections()
	output.Listed = rememberRows
	defer func() { output.Listed = nil }()

	err := session.Run(session.Host{
		Run:          runSessionLine,
		Suggest:      suggestShellLine,
		Status:       sessionStatus,
		RefreshAfter: changesStatus,
		Cacheable:    cacheableLine,
		History:      shell.NewHistory(shellHistoryLimit),
		Classify:     classifyShellWords,
		IntroFrames:  intro,
		IntroEvery:   banner.PlayDuration / time.Duration(len(intro)),
		Corner:       banner.VersionLine(bannerInfo()),
	})
	if errors.Is(err, session.ErrInterrupted) {
		zkSessionWipe()
		os.Exit(130)
	}
	return err
}

// introFrames is the banner the session opens with, one entry per frame
// of the network animation (just the last when it's off or the window is
// too narrow for the network). The session plays them full-window, so
// nothing is drawn into the terminal's own scrollback. The version line is
// left out: the session pins it top right.
func introFrames(p style.Palette, cols int) []string {
	body := banner.Compose(p, banner.Info{}, cols, shellSuggestions())
	if cols < banner.Width {
		return []string{body}
	}
	net := banner.NewNetwork(rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())))
	frames := []banner.Frame{net.Final()}
	if os.Getenv("QUILLINK_NO_ANIMATION") == "" {
		frames = append(net.Frames(), net.Final())
	}
	out := make([]string, len(frames))
	for i, f := range frames {
		out[i] = "\n" + strings.Join(net.Render(p, f), "\n") + "\n" + body
	}
	return out
}

// runSessionLine runs one line typed in the session: a built-in, or a
// command through the same Cobra tree as one-shot use.
func runSessionLine(line string) session.Outcome {
	args, err := shell.Split(line)
	if err != nil {
		printError(err)
		return session.Failed(output.ExitInvalidInput)
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
			session.Wiped("Locked: unlocked keys are cleared from memory, and the output above with them.")
		} else {
			ui.Notice(os.Stdout, "Nothing is unlocked.")
		}
		return session.Ran
	}
	args, err = pickRows(args)
	if err != nil {
		printError(err)
		return session.Failed(output.ExitInvalidInput)
	}
	listingKind = kindOf(commandFor(args))
	resetFlags(rootCmd)
	rootCmd.SetArgs(args)
	err = finishOutput(rootCmd.Execute())
	progress.Reset()
	switch args[0] {
	case "login", "logout", "workspace":
		sessionTokens.forget()
	}
	if err != nil {
		printError(err)
		return session.Failed(exitCode(err))
	}
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
	st := session.Status{Workspace: workspaceLabel(), Production: resolveAPIBase() == client.DefaultAPIBase}
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
	status, err := accountStatus()
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

// accountStatus is /v1/status, asked once per sign-in and then again every
// statusFor: unlocking something changes the status line, not the account.
func accountStatus() (client.StatusResponse, error) {
	key := zkSessionOwner()
	statusMemo.Lock()
	defer statusMemo.Unlock()
	if statusMemo.key == key && time.Since(statusMemo.at) < statusFor {
		return statusMemo.resp, nil
	}
	saved := client.Busy
	client.Busy = nil // the status line has no spinner of its own
	var status client.StatusResponse
	err := newClient().Do("GET", "/v1/status", nil, nil, &status)
	client.Busy = saved
	if err == nil {
		statusMemo.key, statusMemo.at, statusMemo.resp = key, time.Now(), status
	}
	return status, err
}

const statusFor = 5 * time.Minute

var statusMemo struct {
	sync.Mutex
	key  string // the API base and credential it's for
	at   time.Time
	resp client.StatusResponse
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
// classifyShellWords tells the session what each typed word is: command
// names down the tree, flags, and arguments (including flag values). A
// word where a subcommand is required but none matches is Unknown.
func classifyShellWords(words []string) []session.Kind {
	kinds := make([]session.Kind, len(words))
	if len(words) == 0 {
		return kinds
	}
	i := 0
	if words[0] == rootCmd.Name() {
		kinds[0] = session.Command
		i = 1
	}
	if i < len(words) {
		if _, ok := builtinDesc[words[i]]; ok && i == len(words)-1 {
			kinds[i] = session.Command
			return kinds
		}
	}
	c := rootCmd
	inPath := true
	for ; i < len(words); i++ {
		w := words[i]
		if strings.HasPrefix(w, "-") && w != "-" {
			kinds[i] = session.Flag
			name, _, hasValue := strings.Cut(strings.TrimLeft(w, "-"), "=")
			if f := lookupFlag(c, name, !strings.HasPrefix(w, "--")); f != nil && !hasValue && f.Value.Type() != "bool" && i+1 < len(words) {
				i++
				kinds[i] = session.Arg
			}
			continue
		}
		if inPath {
			if next := findChild(c, w); next != nil {
				kinds[i] = session.Command
				c = next
				continue
			}
			inPath = false
			if c.HasAvailableSubCommands() && !c.Runnable() || c == rootCmd {
				kinds[i] = session.Unknown
				continue
			}
		}
		kinds[i] = session.Arg
	}
	return kinds
}

func lookupFlag(c *cobra.Command, name string, short bool) *pflag.Flag {
	if short {
		if f := c.Flags().ShorthandLookup(name); f != nil {
			return f
		}
		return c.InheritedFlags().ShorthandLookup(name)
	}
	if f := c.Flags().Lookup(name); f != nil {
		return f
	}
	return c.InheritedFlags().Lookup(name)
}

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
	if !strings.HasPrefix(word, "-") {
		if ids := suggestIDs(fields, word); ids != nil {
			return ids, from
		}
	}
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
