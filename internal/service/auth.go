package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/helthtech/core-users/internal/model"
	"github.com/helthtech/core-users/internal/repository"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	repo   *repository.UserRepository
	jwt    *JWTService
	rdb    *redis.Client
	nc     *nats.Conn
	keyTTL time.Duration
}

func NewAuthService(repo *repository.UserRepository, jwt *JWTService, rdb *redis.Client, nc *nats.Conn, keyTTL time.Duration) *AuthService {
	return &AuthService{
		repo:   repo,
		jwt:    jwt,
		rdb:    rdb,
		nc:     nc,
		keyTTL: keyTTL,
	}
}

const authKeyPrefix = "browser_auth:"
const authTokenPrefix = "browser_token:"
const passwordAlphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ23456789"

func (s *AuthService) PrepareAuth(ctx context.Context) (string, error) {
	key := uuid.New().String()
	err := s.rdb.Set(ctx, authKeyPrefix+key, "pending", s.keyTTL).Err()
	if err != nil {
		return "", fmt.Errorf("redis set auth key: %w", err)
	}
	return key, nil
}

func (s *AuthService) ResolveAuthKey(ctx context.Context, key string) (bool, error) {
	val, err := s.rdb.Get(ctx, authKeyPrefix+key).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("redis get auth key: %w", err)
	}
	return val == "pending", nil
}

func (s *AuthService) CreateProvisionalUser(ctx context.Context) (*model.ProvisionalUser, error) {
	return s.repo.CreateProvisionalUser(ctx)
}

type AuthTokenMessage struct {
	Key    string `json:"key"`
	Token  string `json:"token"`
	UserID string `json:"user_id"`
}

func (s *AuthService) VerifyPhone(ctx context.Context, phoneE164 string, provisionalUserID uuid.UUID, authKey string) (userID uuid.UUID, token, initialPassword string, isNew bool, err error) {
	existing, dbErr := s.repo.GetUserByPhone(ctx, phoneE164)
	if dbErr == nil && existing != nil {
		userID = existing.ID
		isNew = false
	} else {
		now := time.Now()
		plain, hashErr := generatePassword()
		if hashErr != nil {
			return uuid.Nil, "", "", false, fmt.Errorf("generate password: %w", hashErr)
		}
		hashed, hashErr := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
		if hashErr != nil {
			return uuid.Nil, "", "", false, fmt.Errorf("hash password: %w", hashErr)
		}
		hashedStr := string(hashed)
		u := &model.User{
			ID:              uuid.New(),
			PhoneE164:       phoneE164,
			PhoneVerifiedAt: &now,
			PasswordHash:    &hashedStr,
		}
		if err = s.repo.CreateUser(ctx, u); err != nil {
			return uuid.Nil, "", "", false, fmt.Errorf("create user: %w", err)
		}
		userID = u.ID
		isNew = true
		initialPassword = plain
	}

	token, sessionID, hash, expiresAt, err := s.jwt.Sign(userID, "user")
	if err != nil {
		return uuid.Nil, "", "", false, err
	}

	session := &model.UserSession{
		ID:        sessionID,
		UserID:    userID,
		TokenHash: hash,
		ExpiresAt: expiresAt,
	}
	if err = s.repo.CreateSession(ctx, session); err != nil {
		return uuid.Nil, "", "", false, fmt.Errorf("create session: %w", err)
	}

	if provisionalUserID != uuid.Nil {
		_ = s.repo.DeleteProvisionalUser(ctx, provisionalUserID)
	}

	if authKey != "" {
		s.publishAuthToken(authKey, token, userID.String())
		// Also store in Redis for polling clients; expires after the key TTL.
		_ = s.rdb.Set(ctx, authTokenPrefix+authKey, token+"|"+userID.String(), s.keyTTL).Err()
		_ = s.rdb.Del(ctx, authKeyPrefix+authKey).Err()
	}

	return userID, token, initialPassword, isNew, nil
}

// CheckAuthToken returns the JWT token if the browser challenge has been completed.
// The entry is consumed (deleted) on first read.
func (s *AuthService) CheckAuthToken(ctx context.Context, key string) (token, userID string, err error) {
	val, redisErr := s.rdb.GetDel(ctx, authTokenPrefix+key).Result()
	if redisErr == redis.Nil {
		return "", "", nil
	}
	if redisErr != nil {
		return "", "", fmt.Errorf("redis get auth token: %w", redisErr)
	}
	// val = "token|userID"
	parts := splitOnce(val, "|")
	return parts[0], parts[1], nil
}

// LoginWithPassword verifies a phone+password combination and returns a JWT.
func (s *AuthService) LoginWithPassword(ctx context.Context, phoneE164, password string) (userID uuid.UUID, token string, err error) {
	u, err := s.repo.GetUserByPhone(ctx, phoneE164)
	if err != nil || u == nil {
		return uuid.Nil, "", fmt.Errorf("user not found")
	}
	if u.PasswordHash == nil {
		return uuid.Nil, "", fmt.Errorf("password not set")
	}
	if bcErr := bcrypt.CompareHashAndPassword([]byte(*u.PasswordHash), []byte(password)); bcErr != nil {
		return uuid.Nil, "", fmt.Errorf("invalid password")
	}

	token, sessionID, hash, expiresAt, err := s.jwt.Sign(u.ID, "user")
	if err != nil {
		return uuid.Nil, "", err
	}
	session := &model.UserSession{
		ID:        sessionID,
		UserID:    u.ID,
		TokenHash: hash,
		ExpiresAt: expiresAt,
	}
	if err = s.repo.CreateSession(ctx, session); err != nil {
		return uuid.Nil, "", fmt.Errorf("create session: %w", err)
	}
	return u.ID, token, nil
}

// ChangePassword sets a new bcrypt password for the user.
func (s *AuthService) ChangePassword(ctx context.Context, userID uuid.UUID, newPassword string) error {
	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	h := string(hashed)
	return s.repo.UpdateUserFields(ctx, userID, map[string]any{"password_hash": h})
}

func generatePassword() (string, error) {
	b := make([]byte, 10)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(passwordAlphabet))))
		if err != nil {
			return "", err
		}
		b[i] = passwordAlphabet[n.Int64()]
	}
	return string(b), nil
}

func splitOnce(s, sep string) [2]string {
	for i := 0; i < len(s)-len(sep)+1; i++ {
		if s[i:i+len(sep)] == sep {
			return [2]string{s[:i], s[i+len(sep):]}
		}
	}
	return [2]string{s, ""}
}

func (s *AuthService) publishAuthToken(key, token, userID string) {
	if s.nc == nil {
		return
	}
	msg := AuthTokenMessage{Key: key, Token: token, UserID: userID}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	_ = s.nc.Publish("auth.token."+key, data)
}

func (s *AuthService) DeleteProvisionalUser(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteProvisionalUser(ctx, id)
}
