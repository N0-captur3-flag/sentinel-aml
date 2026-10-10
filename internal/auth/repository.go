package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrDuplicateUsername = errors.New("duplicate username")
	ErrUserNotFound      = errors.New("user not found")
	ErrSessionNotFound   = errors.New("session not found")
	ErrInvalidRole       = errors.New("invalid role")
	ErrInvalidStatus     = errors.New("invalid status")
)

var allowedRoles = map[string]struct{}{
	"analyst":        {},
	"senior_analyst": {},
	"supervisor":     {},
	"auditor":        {},
	"system_worker":  {},
}

var allowedStatuses = map[string]struct{}{
	"active":   {},
	"disabled": {},
}

type rowScanner interface {
	Scan(dest ...any) error
}

type execResult interface {
	RowsAffected() (int64, error)
}

type queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) rowScanner
}

// Repository contains auth persistence methods. Models stay internal to auth and are not DTOs.
type Repository struct {
	db queryer
}

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         string
	Status       string
	CreatedAt    time.Time
}

type CreateUserParams struct {
	Username     string
	PasswordHash string
	Role         string
	Status       string
}

type Session struct {
	ID        int64
	UserID    int64
	TokenHash string
	CreatedAt time.Time
	ExpiresAt time.Time
	RevokedAt sql.NullTime
}

type CreateSessionParams struct {
	UserID    int64
	TokenHash string
	ExpiresAt time.Time
}

func NewRepository(db *sql.DB) *Repository {
	return newRepository(sqlDBAdapter{db: db})
}

func newRepository(db queryer) *Repository {
	return &Repository{db: db}
}

type sqlDBAdapter struct {
	db *sql.DB
}

func (a sqlDBAdapter) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return a.db.ExecContext(ctx, query, args...)
}

func (a sqlDBAdapter) QueryRowContext(ctx context.Context, query string, args ...any) rowScanner {
	return a.db.QueryRowContext(ctx, query, args...)
}

func (r *Repository) CreateUser(ctx context.Context, params CreateUserParams) (User, error) {
	if err := ctx.Err(); err != nil {
		return User{}, err
	}
	if err := validateUserParams(params); err != nil {
		return User{}, err
	}

	status := params.Status
	if status == "" {
		status = "active"
	}

	var user User
	err := r.db.QueryRowContext(ctx, `
INSERT INTO users (username, password_hash, role, status)
VALUES ($1, $2, $3, $4)
RETURNING id, username, password_hash, role, status, created_at`,
		params.Username, params.PasswordHash, params.Role, status,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.Status, &user.CreatedAt)
	if isUniqueViolation(err) {
		return User{}, ErrDuplicateUsername
	}
	if err != nil {
		return User{}, fmt.Errorf("create user: %w", err)
	}

	return user, nil
}

func (r *Repository) FindUserByUsername(ctx context.Context, username string) (User, error) {
	if err := ctx.Err(); err != nil {
		return User{}, err
	}
	var user User
	err := r.db.QueryRowContext(ctx, `
SELECT id, username, password_hash, role, status, created_at
FROM users
WHERE username = $1`,
		username,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.Status, &user.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("find user by username: %w", err)
	}
	return user, nil
}

func (r *Repository) FindUserByID(ctx context.Context, id int64) (User, error) {
	if err := ctx.Err(); err != nil {
		return User{}, err
	}
	var user User
	err := r.db.QueryRowContext(ctx, `
SELECT id, username, password_hash, role, status, created_at
FROM users
WHERE id = $1`,
		id,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.Status, &user.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("find user by id: %w", err)
	}
	return user, nil
}

func (r *Repository) CreateSession(ctx context.Context, params CreateSessionParams) (Session, error) {
	if err := ctx.Err(); err != nil {
		return Session{}, err
	}
	if strings.TrimSpace(params.TokenHash) == "" {
		return Session{}, errors.New("token hash is required")
	}
	if params.ExpiresAt.IsZero() {
		return Session{}, errors.New("session expiry is required")
	}

	var session Session
	err := r.db.QueryRowContext(ctx, `
INSERT INTO sessions (user_id, token_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING id, user_id, token_hash, created_at, expires_at, revoked_at`,
		params.UserID, params.TokenHash, params.ExpiresAt,
	).Scan(&session.ID, &session.UserID, &session.TokenHash, &session.CreatedAt, &session.ExpiresAt, &session.RevokedAt)
	if err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}
	return session, nil
}

func (r *Repository) FindSessionByTokenHash(ctx context.Context, tokenHash string) (Session, error) {
	if err := ctx.Err(); err != nil {
		return Session{}, err
	}
	var session Session
	err := r.db.QueryRowContext(ctx, `
SELECT id, user_id, token_hash, created_at, expires_at, revoked_at
FROM sessions
WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()`,
		tokenHash,
	).Scan(&session.ID, &session.UserID, &session.TokenHash, &session.CreatedAt, &session.ExpiresAt, &session.RevokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrSessionNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("find session by token hash: %w", err)
	}
	return session, nil
}

func (r *Repository) RevokeSession(ctx context.Context, tokenHash string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `
UPDATE sessions
SET revoked_at = now()
WHERE token_hash = $1 AND revoked_at IS NULL`,
		tokenHash,
	)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	if result != nil {
		affected, err := result.RowsAffected()
		if err == nil && affected == 0 {
			return ErrSessionNotFound
		}
	}
	return nil
}

func validateUserParams(params CreateUserParams) error {
	if strings.TrimSpace(params.Username) == "" {
		return errors.New("username is required")
	}
	if strings.TrimSpace(params.PasswordHash) == "" {
		return errors.New("password hash is required")
	}
	if _, ok := allowedRoles[params.Role]; !ok {
		return ErrInvalidRole
	}
	status := params.Status
	if status == "" {
		status = "active"
	}
	if _, ok := allowedStatuses[status]; !ok {
		return ErrInvalidStatus
	}
	return nil
}

type sqlStateError interface {
	SQLState() string
}

func isUniqueViolation(err error) bool {
	var stateErr sqlStateError
	if errors.As(err, &stateErr) && stateErr.SQLState() == "23505" {
		return true
	}
	return strings.Contains(strings.ToLower(fmt.Sprint(err)), "duplicate key")
}
