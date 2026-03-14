// Package account provides user and mountpoint persistence backed by SQLite.
package account

// User represents a stored user account.
type User struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"`
	Enabled      bool   `json:"enabled"`
}

// MountPointRow represents a mountpoint row from the database.
type MountPointRow struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	Enabled         bool   `json:"enabled"`
	Format          string `json:"format"`
	SourceAuthMode  string `json:"source_auth_mode"`
	WriteQueue      *int   `json:"write_queue,omitempty"`
	WriteTimeoutMs  *int   `json:"write_timeout_ms,omitempty"`
}

// Binding represents a user-mountpoint permission binding.
type Binding struct {
	ID             int64  `json:"id"`
	UserID         int64  `json:"user_id"`
	MountPointID   int64  `json:"mountpoint_id"`
	Permission     string `json:"permission"` // publish / subscribe / admin
	Username       string `json:"username,omitempty"`
	MountPointName string `json:"mountpoint_name,omitempty"`
}

// Roles
const (
	RoleAdmin = "admin"
	RoleBase  = "base"
	RoleRover = "rover"
)

// Permissions
const (
	PermPublish   = "publish"
	PermSubscribe = "subscribe"
	PermAdmin     = "admin"
)
