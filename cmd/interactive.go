package cmd

import (
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"os/signal"
	"strings"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/banner"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
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
	if shellActive || jsonOutput || !isTerminalStdin() || !term.IsTerminal(int(os.Stdout.Fd())) {
		return cmd.Help()
	}
	return runShell()
}

// versionText is the banner without the animation on a terminal, and
// Cobra's usual "quillink version X" line everywhere else.
func versionText(c *cobra.Command) string {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
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
// stored or set?), never a network call.
func shellSuggestions() []string {
	if resolveToken() == "" {
		return []string{"login", "help", "exit"}
	}
	return []string{"note list", "note search", "note create", "help", "logout", "exit"}
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
	fmt.Fprint(out, banner.Header(p, bannerInfo(), cols))
	fmt.Fprintln(out)
	fmt.Fprint(out, banner.Suggested(p, shellSuggestions(), cols))
	fmt.Fprintln(out)

	fd := int(os.Stdin.Fd())
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
			fmt.Fprint(out, "\033[H\033[2J")
			continue
		case "lock":
			if zkSessionWipe() {
				fmt.Fprintln(out, p.Dim("Locked: the Zero-Knowledge key is cleared from memory."))
			} else {
				fmt.Fprintln(out, p.Dim("Nothing is unlocked."))
			}
			continue
		}
		runShellCommand(args)
	}
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

func runShellCommand(args []string) {
	resetFlags(rootCmd)
	rootCmd.SetArgs(args)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
	}
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
