package cmd

import (
	"os"
	"regexp"
	"strings"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/style"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Help and usage text is colourised on a terminal so commands, arguments
// and flags are easy to spot: headings bold, command names yellow,
// arguments cyan, flags green, flag value types dim. Piped or redirected
// help, and NO_COLOR, get exactly the plain text.

// usageTemplate is Cobra's default usage template with colour hooks (no
// command groups or aliases are used in this CLI, so those parts are left
// out).
const usageTemplate = `{{heading "Usage:"}}{{if .Runnable}}
  {{useLine .}}{{end}}{{if .HasAvailableSubCommands}}
  {{cmdName .CommandPath}} {{arg "[command]"}}{{end}}{{if .HasExample}}

{{heading "Examples:"}}
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

{{heading "Available Commands:"}}{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{cmdName (rpad .Name .NamePadding)}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

{{heading "Flags:"}}
{{flagList .LocalFlags}}{{end}}{{if .HasAvailableInheritedFlags}}

{{heading "Global Flags:"}}
{{flagList .InheritedFlags}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{cmdName .CommandPath}} {{arg "[command]"}} {{flag "--help"}}" for more information about a command.{{end}}
`

func init() {
	pal := func() style.Palette { return style.For(os.Stdout) }
	cobra.AddTemplateFunc("heading", func(s string) string { return pal().Bold(s) })
	cobra.AddTemplateFunc("cmdName", func(s string) string { return pal().Brand(s) })
	cobra.AddTemplateFunc("arg", func(s string) string { return pal().Arg(s) })
	cobra.AddTemplateFunc("flag", func(s string) string { return pal().Flag(s) })
	cobra.AddTemplateFunc("useLine", func(c *cobra.Command) string { return colorUseLine(pal(), c) })
	cobra.AddTemplateFunc("flagList", func(fs *pflag.FlagSet) string { return colorFlagUsages(pal(), fs.FlagUsages()) })
	rootCmd.SetUsageTemplate(usageTemplate)
}

// colorUseLine colours "quillink note update <id> [flags]": the command
// path yellow, <args> and [optional args] cyan, [flags] dim.
func colorUseLine(p style.Palette, c *cobra.Command) string {
	line := c.UseLine()
	path := c.CommandPath()
	rest := strings.TrimPrefix(line, path)
	if rest == line {
		return line
	}
	words := strings.Split(rest, " ")
	for i, w := range words {
		switch {
		case w == "[flags]":
			words[i] = p.Dim(w)
		case strings.HasPrefix(w, "<") || strings.HasPrefix(w, "["):
			words[i] = p.Arg(w)
		}
	}
	return p.Brand(path) + strings.Join(words, " ")
}

// flagLine matches one pflag usage line: indent, "-x, --name" or
// "--name", an optional value type, then the description (pflag always
// separates it by at least two spaces).
var flagLine = regexp.MustCompile(`^(\s+)((?:-\S, )?--[^\s=]+)( \S+)?(\s{2,}.*)?$`)

func colorFlagUsages(p style.Palette, usages string) string {
	lines := strings.Split(strings.TrimRight(usages, " \n"), "\n")
	for i, l := range lines {
		l = strings.TrimRight(l, " ")
		m := flagLine.FindStringSubmatch(l)
		if m == nil {
			lines[i] = l
			continue
		}
		typ := ""
		if m[3] != "" {
			typ = " " + p.Dim(m[3][1:])
		}
		lines[i] = m[1] + p.Flag(m[2]) + typ + m[4]
	}
	return strings.Join(lines, "\n")
}
