package output

// Distinct exit codes for common failure cases (see CLI Access Confluence
// page, "Output design") so scripts can branch without parsing error text.
const (
	ExitOK           = 0
	ExitGeneral      = 1
	ExitAuthFailure  = 2
	ExitNotFound     = 3
	ExitRateLimited  = 4
	ExitInvalidInput = 5
)

// CodeForStatus maps an HTTP status from a /v1 ProblemDetail response to
// one of the exit codes above.
func CodeForStatus(status int) int {
	switch status {
	case 401, 403:
		return ExitAuthFailure
	case 404:
		return ExitNotFound
	case 429:
		return ExitRateLimited
	case 400, 422:
		return ExitInvalidInput
	default:
		return ExitGeneral
	}
}
