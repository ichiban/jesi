package embed

import "fmt"

// Error represents an embedding error.
type Error struct {
	Status int                    `json:"status,omitempty"`
	Title  string                 `json:"title"`
	Detail string                 `json:"detail,omitempty"`
	Links  map[string]interface{} `json:"_links,omitempty"`
}

var _ error = (*Error)(nil)

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Title, e.Detail)
}
