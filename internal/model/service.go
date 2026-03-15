package model

// GlobalConfig holds environment-wide settings shared across all containers.
type GlobalConfig struct {
	Network       string `json:"network"`
	NetworkSubnet string `json:"network_subnet,omitempty"`
}

// Service represents a single container definition stored as an individual JSON file.
type Service struct {
	Name          string            `json:"name"`
	Group         string            `json:"group,omitempty"`
	Hostname      string            `json:"hostname"`
	Image         string            `json:"image"`
	Registry      string            `json:"registry,omitempty"`
	Network       string            `json:"network"`
	Ports         PortConfig        `json:"ports"`
	Environment   map[string]string `json:"environment,omitempty"`
	Volumes       []string          `json:"volumes,omitempty"`
	Command       string            `json:"command,omitempty"`
	Files         []string          `json:"files,omitempty"`
	HealthCheck   *HealthCheck      `json:"health_check,omitempty"`
	Logs          []LogSource       `json:"logs,omitempty"`
	RAMEstimateMB int               `json:"ram_estimate_mb,omitempty"`
	DashboardURL  string            `json:"dashboard_url,omitempty"`
	Dev           *DevConfig        `json:"dev,omitempty"`
	StartOrder    int               `json:"start_order,omitempty"`
}

// DevConfig defines how to run a service in local development mode.
// Used by `keel dev <service> <local-path>`.
type DevConfig struct {
	// Command is the command (as an argument list) to run inside the dev container.
	// Using a list avoids shell-quoting issues with complex commands.
	// Examples:
	//   ["air"]
	//   ["sh", "-c", "envsubst < etc/app.conf.template > /etc/app.conf && air"]
	//   ["npm", "run", "dev"]
	Command []string `json:"command,omitempty"`

	// CapAdd lists Linux capabilities to add to the container (docker run --cap-add).
	// Example: ["NET_BIND_SERVICE"] — needed when the service binds to ports < 1024.
	CapAdd []string `json:"cap_add,omitempty"`

	// Dockerfile contains the lines of a Dockerfile used to build the dev image.
	// keel builds this image on first run (and rebuilds when it changes).
	// The local source path is used as the build context, so COPY commands work.
	// The WORKDIR instruction determines where the local code will be mounted.
	Dockerfile []string `json:"dockerfile,omitempty"`
}

// PortConfig defines internal and external port mappings.
type PortConfig struct {
	Internal int `json:"internal"`
	External int `json:"external"`
}

// HealthCheck defines how to check if a service is healthy.
type HealthCheck struct {
	Type        string `json:"type"` // "command" or "http"
	Command     string `json:"command,omitempty"`
	URL         string `json:"url,omitempty"`
	Interval    int    `json:"interval"`
	Retries     int    `json:"retries"`
	StartPeriod int    `json:"start_period"`
}

// LogSource defines where to read logs for a service.
type LogSource struct {
	Name     string `json:"name"`               // e.g. "container", "app"
	Type     string `json:"type"`               // "docker" or "file"
	Path     string `json:"path,omitempty"`     // path inside container (requires docker exec)
	HostPath string `json:"host_path,omitempty"` // path on the host (readable even when container is down)
}

// ContainerStatus represents the runtime state of a Docker container.
type ContainerStatus string

const (
	StatusRunning    ContainerStatus = "running"
	StatusRestarting ContainerStatus = "restarting"
	StatusStopped    ContainerStatus = "stopped"
	StatusUnhealthy  ContainerStatus = "unhealthy"
	StatusMissing    ContainerStatus = "missing"
)
