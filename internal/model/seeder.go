package model

// Seeder defines initialization commands for an infrastructure container.
type Seeder struct {
	Name        string          `json:"name"`
	Target      string          `json:"target"`
	Description string          `json:"description,omitempty"`
	Order       int             `json:"order,omitempty"`
	Commands    []SeederCommand `json:"commands"`
}

// SeederCommand is a single initialization step.
type SeederCommand struct {
	Name        string `json:"name"`
	Command     string `json:"command,omitempty"`
	Script      string `json:"script,omitempty"`
	Interpreter string `json:"interpreter,omitempty"`
}
