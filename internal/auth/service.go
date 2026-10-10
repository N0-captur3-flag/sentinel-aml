package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	DefaultRole       = "analyst"
	defaultSessionTTL = 12 * time.Hour
	sessionTokenBytes = 32
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthenticated    = errors.New("unauthenticated")
	ErrDisabledUser       = errors.New("user is disabled")
	ErrInvalidInput       = errors.New("invalid input")
)

type Store interface {
	CreateUser(context.Context, CreateUserParams) (User, error)
	FindUserByUsername(context.Context, string) (User, error)
	FindUserByID(context.Context, int64) (User, error)
	CreateSession(context.Context, CreateSessionParams) (Session, error)
	FindSessionByTokenHash(context.Context, string) (Session, error)
}

type ServiceConfig struct {
	SessionTTL time.Duration
	Clock      func() time.Time
}

type Service struct {
	store      Store
	sessionTTL time.Duration
	clock      func() time.Time
}

type AuthResult struct {
	User      User
	RawToken  string
	ExpiresAt time.Time
}

func NewService(store Store, cfg ServiceConfig) *Service {
	sessionTTL := cfg.SessionTTL
	if sessionTTL <= 0 {
		sessionTTL = defaultSessionTTL
	}
	clock := cfg.Clock
	if clock == nil {
		clock = time.Now
	}
	return &Service{store: store, sessionTTL: sessionTTL, clock: clock}
}

func (s *Service) Register(ctx context.Context, username, password string) (AuthResult, error) {
	if err := ctx.Err(); err != nil {
		return AuthResult{}, err
	}
	username = strings.TrimSpace(username)
	if username == "" || strings.TrimSpace(password) == "" {
		return AuthResult{}, fmt.Errorf("%w: username and password are required", ErrInvalidInput)
	}

	passwordHash, err := HashPassword(password)
	if err != nil {
		return AuthResult{}, err
	}

	user, err := s.store.CreateUser(ctx, CreateUserParams{
		Username:     username,
		PasswordHash: passwordHash,
		Role:         DefaultRole,
		Status:       "active",
	})
	if err != nil {
		return AuthResult{}, err
	}

	return s.createSession(ctx, user)
}

func (s *Service) Login(ctx context.Context, username, password string) (AuthResult, error) {
	if err := ctx.Err(); err != nil {
		return AuthResult{}, err
	}
	username = strings.TrimSpace(username)
	if username == "" || strings.TrimSpace(password) == "" {
		return AuthResult{}, ErrInvalidCredentials
	}

	user, err := s.store.FindUserByUsername(ctx, username)
	if errors.Is(err, ErrUserNotFound) {
		return AuthResult{}, ErrInvalidCredentials
	}
	if err != nil {
		return AuthResult{}, err
	}
	if user.Status != "active" {
		return AuthResult{}, ErrDisabledUser
	}
	if !CheckPassword(password, user.PasswordHash) {
		return AuthResult{}, ErrInvalidCredentials
	}

	return s.createSession(ctx, user)
}

func (s *Service) CurrentUser(ctx context.Context, rawToken string) (User, error) {
	if err := ctx.Err(); err != nil {
		return User{}, err
	}
	if strings.TrimSpace(rawToken) == "" {
		return User{}, ErrUnauthenticated
	}

	session, err := s.store.FindSessionByTokenHash(ctx, HashSessionToken(rawToken))
	if errors.Is(err, ErrSessionNotFound) {
		return User{}, ErrUnauthenticated
	}
	if err != nil {
		return User{}, err
	}

	user, err := s.store.FindUserByID(ctx, session.UserID)
	if errors.Is(err, ErrUserNotFound) {
		return User{}, ErrUnauthenticated
	}
	if err != nil {
		return User{}, err
	}
	if user.Status != "active" {
		return User{}, ErrDisabledUser
	}
	return user, nil
}

func (s *Service) createSession(ctx context.Context, user User) (AuthResult, error) {
	rawToken, err := GenerateSessionToken()
	if err != nil {
		return AuthResult{}, err
	}
	expiresAt := s.clock().UTC().Add(s.sessionTTL)
	session, err := s.store.CreateSession(ctx, CreateSessionParams{
		UserID:    user.ID,
		TokenHash: HashSessionToken(rawToken),
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{User: user, RawToken: rawToken, ExpiresAt: session.ExpiresAt}, nil
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

func CheckPassword(password, passwordHash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) == nil
}

func GenerateSessionToken() (string, error) {
	buf := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func HashSessionToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

func ConstantTimeTokenHashEqual(left, right string) bool {
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}
