package clipboard

import (
	"errors"
	"testing"
)

func TestFind(t *testing.T) {
	has := func(names ...string) func(string) (string, error) {
		return func(n string) (string, error) {
			for _, h := range names {
				if h == n {
					return n, nil
				}
			}
			return "", errors.New("not found")
		}
	}
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}
	cases := []struct {
		name string
		goos string
		env  map[string]string
		look func(string) (string, error)
		want string
	}{
		{"mac", "darwin", nil, has("pbcopy"), "pbcopy"},
		{"over ssh", "darwin", map[string]string{"SSH_TTY": "/dev/ttys001"}, has("pbcopy"), ""},
		{"wayland", "linux", map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, has("wl-copy"), "wl-copy"},
		{"x11 falls through to xsel", "linux", map[string]string{"DISPLAY": ":0"}, has("xsel"), "xsel"},
		{"linux without a desktop", "linux", nil, has("xclip"), ""},
		{"windows uses the terminal", "windows", nil, has("clip"), ""},
	}
	for _, c := range cases {
		got := ""
		if tool := find(c.goos, env(c.env), c.look); tool != nil {
			got = tool[0]
		}
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
