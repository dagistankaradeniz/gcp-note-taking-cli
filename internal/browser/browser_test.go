package browser

import "testing"

func TestCheck(t *testing.T) {
	ok := []string{
		"https://note-taking-app-prod.web.app/device?user_code=ABCD-1234",
		"http://localhost:5173/device",
	}
	bad := []string{"", "file:///etc/passwd", "javascript:alert(1)", "quillink://x", "https://", "/device"}
	for _, u := range ok {
		if err := check(u); err != nil {
			t.Errorf("check(%q) = %v, want nil", u, err)
		}
	}
	for _, u := range bad {
		if check(u) == nil {
			t.Errorf("check(%q) = nil, want an error", u)
		}
	}
}
