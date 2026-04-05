package model

// Seeder defines initialization commands for an infrastructure container.
type Seeder struct {
	Name        string          `json:"name"`
	Target      string          `json:"target"`
	Description string          `json:"description,omitempty"`
	Order       int             `json:"order,omitempty"`
	Commands    []SeederCommand `json:"commands"`
}

// SeederHTTP defines an HTTP request to execute as a seeder step.
type SeederHTTP struct {
	URL          string            `json:"url"`
	Method       string            `json:"method,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	Body         string            `json:"body,omitempty"`
	ExpectStatus int               `json:"expect_status,omitempty"`
}

// SeederCommand is a single initialization step.
type SeederCommand struct {
	Name        string      `json:"name"`
	Command     string      `json:"command,omitempty"`
	Script      string      `json:"script,omitempty"`
	Interpreter string      `json:"interpreter,omitempty"`
	HTTP        *SeederHTTP `json:"http,omitempty"`
}
