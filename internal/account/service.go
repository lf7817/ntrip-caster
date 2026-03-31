package account

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"ntrip-caster/internal/rtcm"
)

// Service provides CRUD operations for users and mountpoint records.
type Service struct {
	db *sql.DB

	// Auth cache: maps "username:passwordHash" to cached result
	// This dramatically reduces bcrypt CPU usage during mass connections
	authCache   map[string]*authCacheEntry
	authCacheMu sync.RWMutex
}

// authCacheEntry holds a cached authentication result.
type authCacheEntry struct {
	user      *User
	expiresAt time.Time
}

const authCacheTTL = 5 * time.Minute

// NewService creates an account service backed by db.
func NewService(db *sql.DB) *Service {
	return &Service{
		db:        db,
		authCache: make(map[string]*authCacheEntry),
	}
}

// --- Users ---

// CreateUser creates a new user with a bcrypt-hashed password.
func (s *Service) CreateUser(username, password, role string) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	res, err := s.db.Exec(
		"INSERT INTO users (username, password_hash, role, enabled) VALUES (?, ?, ?, 1)",
		username, string(hash), role,
	)
	if err != nil {
		return nil, fmt.Errorf("insert user: %w", err)
	}
	id, _ := res.LastInsertId()
	return &User{ID: id, Username: username, Role: role, Enabled: true}, nil
}

// Authenticate checks username/password and returns the user if valid.
// Uses an in-memory cache to avoid expensive bcrypt operations during mass connections.
func (s *Service) Authenticate(username, password string) (*User, error) {
	// Check cache first
	cacheKey := username + ":" + password
	s.authCacheMu.RLock()
	if entry, ok := s.authCache[cacheKey]; ok {
		if time.Now().Before(entry.expiresAt) {
			s.authCacheMu.RUnlock()
			return entry.user, nil
		}
	}
	s.authCacheMu.RUnlock()

	// Not in cache or expired, do full authentication
	u, err := s.GetUserByName(username)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, nil
	}
	if !u.Enabled {
		return nil, nil
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, nil
	}

	// Cache the successful result
	s.authCacheMu.Lock()
	s.authCache[cacheKey] = &authCacheEntry{
		user:      u,
		expiresAt: time.Now().Add(authCacheTTL),
	}
	s.authCacheMu.Unlock()

	return u, nil
}

// GetUserByName looks up a user by username.
func (s *Service) GetUserByName(username string) (*User, error) {
	row := s.db.QueryRow("SELECT id, username, password_hash, role, enabled FROM users WHERE username = ?", username)
	u := &User{}
	var enabled int
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &enabled); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query user: %w", err)
	}
	u.Enabled = enabled != 0
	return u, nil
}

// GetUser retrieves a user by ID.
func (s *Service) GetUser(id int64) (*User, error) {
	row := s.db.QueryRow("SELECT id, username, password_hash, role, enabled FROM users WHERE id = ?", id)
	u := &User{}
	var enabled int
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &enabled); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query user: %w", err)
	}
	u.Enabled = enabled != 0
	return u, nil
}

// ListUsers returns all users.
func (s *Service) ListUsers() ([]*User, error) {
	rows, err := s.db.Query("SELECT id, username, password_hash, role, enabled FROM users ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		u := &User{}
		var enabled int
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &enabled); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		u.Enabled = enabled != 0
		users = append(users, u)
	}
	return users, rows.Err()
}

// ListUsersPaginated returns users with pagination and filtering.
func (s *Service) ListUsersPaginated(page, limit int, search, role string, enabled *bool) ([]*User, int, error) {
	// Build WHERE clause
	where := "WHERE 1=1"
	args := []any{}

	if search != "" {
		where += " AND username LIKE ?"
		args = append(args, "%"+search+"%")
	}
	if role != "" {
		where += " AND role = ?"
		args = append(args, role)
	}
	if enabled != nil {
		where += " AND enabled = ?"
		args = append(args, *enabled)
	}

	// Get total count
	var total int
	countQuery := "SELECT COUNT(*) FROM users " + where
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	// Get paginated rows
	offset := (page - 1) * limit
	args = append(args, limit, offset)
	query := "SELECT id, username, password_hash, role, enabled FROM users " + where + " ORDER BY id LIMIT ? OFFSET ?"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list users paginated: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		u := &User{}
		var en int
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &en); err != nil {
			return nil, 0, fmt.Errorf("scan user: %w", err)
		}
		u.Enabled = en != 0
		users = append(users, u)
	}
	return users, total, rows.Err()
}

// UpdateUser updates role and enabled status.
func (s *Service) UpdateUser(id int64, role string, enabled bool) error {
	en := 0
	if enabled {
		en = 1
	}
	_, err := s.db.Exec("UPDATE users SET role = ?, enabled = ? WHERE id = ?", role, en, id)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	// Clear auth cache since user status changed
	s.authCacheMu.Lock()
	s.authCache = make(map[string]*authCacheEntry)
	s.authCacheMu.Unlock()
	return nil
}

// UpdatePassword changes a user's password.
func (s *Service) UpdatePassword(id int64, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	_, err = s.db.Exec("UPDATE users SET password_hash = ? WHERE id = ?", string(hash), id)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	// Clear auth cache since password changed
	s.authCacheMu.Lock()
	s.authCache = make(map[string]*authCacheEntry)
	s.authCacheMu.Unlock()
	return nil
}

// DeleteUser removes a user by ID.
func (s *Service) DeleteUser(id int64) error {
	_, err := s.db.Exec("DELETE FROM users WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	// Clear auth cache since user was deleted
	s.authCacheMu.Lock()
	s.authCache = make(map[string]*authCacheEntry)
	s.authCacheMu.Unlock()
	return nil
}

// --- Mountpoints ---

// CreateMountPointRow inserts a mountpoint record.
func (s *Service) CreateMountPointRow(name, description, format string) (*MountPointRow, error) {
	res, err := s.db.Exec(
		"INSERT INTO mountpoints (name, description, format) VALUES (?, ?, ?)",
		name, description, format,
	)
	if err != nil {
		return nil, fmt.Errorf("insert mountpoint: %w", err)
	}
	id, _ := res.LastInsertId()
	return &MountPointRow{ID: id, Name: name, Description: description, Format: format, Enabled: true, SourceAuthMode: "user_binding"}, nil
}

// VerifyMountPointSourceSecret checks whether the given secret matches the
// mountpoint's stored source_secret_hash. If no secret is configured, it
// returns false.
func (s *Service) VerifyMountPointSourceSecret(mountpointName, secret string) (bool, error) {
	if mountpointName == "" || secret == "" {
		return false, nil
	}
	var hash sql.NullString
	if err := s.db.QueryRow(
		"SELECT source_secret_hash FROM mountpoints WHERE name = ?",
		mountpointName,
	).Scan(&hash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("query mountpoint source secret: %w", err)
	}
	if !hash.Valid || hash.String == "" {
		return false, nil
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash.String), []byte(secret)); err != nil {
		return false, nil
	}
	return true, nil
}

// SetMountPointSourceSecret stores (bcrypt-hashed) source secret for a mountpoint.
// Passing an empty secret clears the stored secret.
func (s *Service) SetMountPointSourceSecret(mountpointID int64, secret string) error {
	if secret == "" {
		_, err := s.db.Exec("UPDATE mountpoints SET source_secret_hash = NULL WHERE id = ?", mountpointID)
		if err != nil {
			return fmt.Errorf("clear mountpoint source secret: %w", err)
		}
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash mountpoint source secret: %w", err)
	}
	_, err = s.db.Exec("UPDATE mountpoints SET source_secret_hash = ? WHERE id = ?", string(hash), mountpointID)
	if err != nil {
		return fmt.Errorf("update mountpoint source secret: %w", err)
	}
	return nil
}

// GetMountPointRow retrieves a mountpoint by name.
func (s *Service) GetMountPointRow(name string) (*MountPointRow, error) {
	row := s.db.QueryRow(
		"SELECT id, name, description, enabled, format, source_auth_mode, write_queue, write_timeout_ms, max_clients FROM mountpoints WHERE name = ?", name,
	)
	return scanMountPointRow(row)
}

// GetMountPointRowByID retrieves a mountpoint by ID.
func (s *Service) GetMountPointRowByID(id int64) (*MountPointRow, error) {
	row := s.db.QueryRow(
		"SELECT id, name, description, enabled, format, source_auth_mode, write_queue, write_timeout_ms, max_clients FROM mountpoints WHERE id = ?", id,
	)
	return scanMountPointRow(row)
}

func scanMountPointRow(row *sql.Row) (*MountPointRow, error) {
	mp := &MountPointRow{}
	var enabled int
	var wq, wt, mc sql.NullInt64
	if err := row.Scan(&mp.ID, &mp.Name, &mp.Description, &enabled, &mp.Format, &mp.SourceAuthMode, &wq, &wt, &mc); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query mountpoint: %w", err)
	}
	mp.Enabled = enabled != 0
	if wq.Valid {
		v := int(wq.Int64)
		mp.WriteQueue = &v
	}
	if wt.Valid {
		v := int(wt.Int64)
		mp.WriteTimeoutMs = &v
	}
	if mc.Valid {
		mp.MaxClients = int(mc.Int64)
	}
	return mp, nil
}

// ListMountPointRows returns all mountpoint records.
func (s *Service) ListMountPointRows() ([]*MountPointRow, error) {
	rows, err := s.db.Query(
		"SELECT id, name, description, enabled, format, source_auth_mode, write_queue, write_timeout_ms, max_clients FROM mountpoints ORDER BY id",
	)
	if err != nil {
		return nil, fmt.Errorf("list mountpoints: %w", err)
	}
	defer rows.Close()

	var list []*MountPointRow
	for rows.Next() {
		mp := &MountPointRow{}
		var enabled int
		var wq, wt, mc sql.NullInt64
		if err := rows.Scan(&mp.ID, &mp.Name, &mp.Description, &enabled, &mp.Format, &mp.SourceAuthMode, &wq, &wt, &mc); err != nil {
			return nil, fmt.Errorf("scan mountpoint: %w", err)
		}
		mp.Enabled = enabled != 0
		if wq.Valid {
			v := int(wq.Int64)
			mp.WriteQueue = &v
		}
		if wt.Valid {
			v := int(wt.Int64)
			mp.WriteTimeoutMs = &v
		}
		if mc.Valid {
			mp.MaxClients = int(mc.Int64)
		}
		list = append(list, mp)
	}
	return list, rows.Err()
}

// ListMountPointRowsPaginated returns mountpoints with pagination and filtering.
func (s *Service) ListMountPointRowsPaginated(page, limit int, search, format string, enabled *bool) ([]*MountPointRow, int, error) {
	// Build WHERE clause
	where := "WHERE 1=1"
	args := []any{}

	if search != "" {
		where += " AND name LIKE ?"
		args = append(args, "%"+search+"%")
	}
	if format != "" {
		where += " AND format = ?"
		args = append(args, format)
	}
	if enabled != nil {
		where += " AND enabled = ?"
		args = append(args, *enabled)
	}

	// Get total count
	var total int
	countQuery := "SELECT COUNT(*) FROM mountpoints " + where
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count mountpoints: %w", err)
	}

	// Get paginated rows
	offset := (page - 1) * limit
	args = append(args, limit, offset)
	query := "SELECT id, name, description, enabled, format, source_auth_mode, write_queue, write_timeout_ms, max_clients FROM mountpoints " + where + " ORDER BY id LIMIT ? OFFSET ?"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list mountpoints paginated: %w", err)
	}
	defer rows.Close()

	var list []*MountPointRow
	for rows.Next() {
		mp := &MountPointRow{}
		var en int
		var wq, wt, mc sql.NullInt64
		if err := rows.Scan(&mp.ID, &mp.Name, &mp.Description, &en, &mp.Format, &mp.SourceAuthMode, &wq, &wt, &mc); err != nil {
			return nil, 0, fmt.Errorf("scan mountpoint: %w", err)
		}
		mp.Enabled = en != 0
		if wq.Valid {
			v := int(wq.Int64)
			mp.WriteQueue = &v
		}
		if wt.Valid {
			v := int(wt.Int64)
			mp.WriteTimeoutMs = &v
		}
		if mc.Valid {
			mp.MaxClients = int(mc.Int64)
		}
		list = append(list, mp)
	}
	return list, total, rows.Err()
}

// UpdateMountPointRow updates a mountpoint record.
func (s *Service) UpdateMountPointRow(id int64, description, format string, enabled bool) error {
	en := 0
	if enabled {
		en = 1
	}
	_, err := s.db.Exec(
		"UPDATE mountpoints SET description = ?, format = ?, enabled = ? WHERE id = ?",
		description, format, en, id,
	)
	if err != nil {
		return fmt.Errorf("update mountpoint: %w", err)
	}
	return nil
}

// SetMountPointMaxClients updates the max_clients field for a mountpoint.
func (s *Service) SetMountPointMaxClients(id int64, maxClients int) error {
	_, err := s.db.Exec("UPDATE mountpoints SET max_clients = ? WHERE id = ?", maxClients, id)
	if err != nil {
		return fmt.Errorf("update mountpoint max_clients: %w", err)
	}
	return nil
}

// DeleteMountPointRow deletes a mountpoint record.
func (s *Service) DeleteMountPointRow(id int64) error {
	_, err := s.db.Exec("DELETE FROM mountpoints WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete mountpoint: %w", err)
	}
	return nil
}

// --- Bindings ---

// AddBinding creates a user-mountpoint access binding.
func (s *Service) AddBinding(userID, mountpointID int64) error {
	_, err := s.db.Exec(
		"INSERT OR IGNORE INTO user_mountpoint_bindings (user_id, mountpoint_id) VALUES (?, ?)",
		userID, mountpointID,
	)
	if err != nil {
		return fmt.Errorf("add binding: %w", err)
	}
	return nil
}

// ListBindings returns all bindings with joined user/mountpoint names.
func (s *Service) ListBindings() ([]*Binding, error) {
	rows, err := s.db.Query(`
		SELECT b.id, b.user_id, b.mountpoint_id, u.username, m.name
		FROM user_mountpoint_bindings b
		JOIN users u ON b.user_id = u.id
		JOIN mountpoints m ON b.mountpoint_id = m.id
		ORDER BY u.username, m.name`)
	if err != nil {
		return nil, fmt.Errorf("list bindings: %w", err)
	}
	defer rows.Close()

	var list []*Binding
	for rows.Next() {
		b := &Binding{}
		if err := rows.Scan(&b.ID, &b.UserID, &b.MountPointID, &b.Username, &b.MountPointName); err != nil {
			return nil, fmt.Errorf("scan binding: %w", err)
		}
		list = append(list, b)
	}
	return list, rows.Err()
}

// ListBindingsByUser returns all bindings for a specific user.
func (s *Service) ListBindingsByUser(userID int64) ([]*Binding, error) {
	rows, err := s.db.Query(`
		SELECT b.id, b.user_id, b.mountpoint_id, u.username, m.name
		FROM user_mountpoint_bindings b
		JOIN users u ON b.user_id = u.id
		JOIN mountpoints m ON b.mountpoint_id = m.id
		WHERE b.user_id = ?
		ORDER BY m.name`, userID)
	if err != nil {
		return nil, fmt.Errorf("list user bindings: %w", err)
	}
	defer rows.Close()

	var list []*Binding
	for rows.Next() {
		b := &Binding{}
		if err := rows.Scan(&b.ID, &b.UserID, &b.MountPointID, &b.Username, &b.MountPointName); err != nil {
			return nil, fmt.Errorf("scan binding: %w", err)
		}
		list = append(list, b)
	}
	return list, rows.Err()
}

// HasBinding checks if a user has access to a mountpoint.
func (s *Service) HasBinding(userID int64, mountpointName string) (bool, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM user_mountpoint_bindings b
		JOIN mountpoints m ON b.mountpoint_id = m.id
		WHERE b.user_id = ? AND m.name = ?`,
		userID, mountpointName,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check binding: %w", err)
	}
	return count > 0, nil
}

// RemoveBinding removes a specific binding.
func (s *Service) RemoveBinding(id int64) error {
	_, err := s.db.Exec("DELETE FROM user_mountpoint_bindings WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("remove binding: %w", err)
	}
	return nil
}

// EnsureAdmin creates a default admin user if no admin exists.
func (s *Service) EnsureAdmin(username, password string) error {
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&count); err != nil {
		return fmt.Errorf("count admins: %w", err)
	}
	if count > 0 {
		return nil
	}
	_, err := s.CreateUser(username, password, RoleAdmin)
	return err
}

// --- Antenna Position ---

// UpdateMountPointAntennaPosition 持久化天线位置到数据库。
func (s *Service) UpdateMountPointAntennaPosition(name string, pos *rtcm.AntennaPosition) error {
	if pos == nil {
		return nil
	}

	_, err := s.db.Exec(`
		UPDATE mountpoints
		SET antenna_lat = ?, antenna_lon = ?, antenna_height = ?, antenna_updated_at = ?
		WHERE name = ?`,
		pos.Latitude, pos.Longitude, pos.Height, pos.UpdatedAt, name)

	if err != nil {
		return fmt.Errorf("update antenna position: %w", err)
	}
	return nil
}

// GetMountPointAntennaPosition 读取历史位置。
func (s *Service) GetMountPointAntennaPosition(name string) (*rtcm.AntennaPosition, error) {
	row := s.db.QueryRow(`
		SELECT antenna_lat, antenna_lon, antenna_height, antenna_updated_at
		FROM mountpoints WHERE name = ?`, name)

	var lat, lon, h sql.NullFloat64
	var ts sql.NullInt64

	if err := row.Scan(&lat, &lon, &h, &ts); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query antenna position: %w", err)
	}

	if !lat.Valid || !lon.Valid {
		return nil, nil
	}

	var height float64
	if h.Valid {
		height = h.Float64
	}

	var updatedAt int64
	if ts.Valid {
		updatedAt = ts.Int64
	}

	return &rtcm.AntennaPosition{
		Latitude:  lat.Float64,
		Longitude: lon.Float64,
		Height:    height,
		UpdatedAt: updatedAt,
	}, nil
}
