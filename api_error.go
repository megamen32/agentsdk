package agentsdk

import "fmt"

// APIError is an authoritative HTTP error returned by the Airlock platform.
// Transport failures do not have this type. Error preserves the legacy text.
type APIError struct {
	Method     string
	Path       string
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s %s: status %d: %s", e.Method, e.Path, e.StatusCode, e.Body)
}
