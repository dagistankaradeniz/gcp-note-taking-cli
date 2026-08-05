package output

import "testing"

func TestCodeForStatus(t *testing.T) {
	cases := map[int]int{
		401: ExitAuthFailure,
		403: ExitAuthFailure,
		404: ExitNotFound,
		429: ExitRateLimited,
		400: ExitInvalidInput,
		422: ExitInvalidInput,
		500: ExitGeneral,
		200: ExitGeneral,
	}
	for status, want := range cases {
		if got := CodeForStatus(status); got != want {
			t.Errorf("CodeForStatus(%d) = %d, want %d", status, got, want)
		}
	}
}
