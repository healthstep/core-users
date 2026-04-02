package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/helthtech/core-users/internal/model"
	"github.com/helthtech/core-users/internal/repository"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
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

func (s *AuthService) VerifyPhone(ctx context.Context, phoneE164 string, provisionalUserID uuid.UUID, authKey string) (userID uuid.UUID, token string, isNew bool, err error) {
	existing, dbErr := s.repo.GetUserByPhone(ctx, phoneE164)
	if dbErr == nil && existing != nil {
		userID = existing.ID
		isNew = false
	} else {
		now := time.Now()
		u := &model.User{
			ID:              uuid.New(),
			PhoneE164:       phoneE164,
			PhoneVerifiedAt: &now,
		}
		if err = s.repo.CreateUser(ctx, u); err != nil {
			return uuid.Nil, "", false, fmt.Errorf("create user: %w", err)
		}
		userID = u.ID
		isNew = true
	}

	token, sessionID, hash, expiresAt, err := s.jwt.Sign(userID, "user")
	if err != nil {
		return uuid.Nil, "", false, err
	}

	session := &model.UserSession{
		ID:        sessionID,
		UserID:    userID,
		TokenHash: hash,
		ExpiresAt: expiresAt,
	}
	if err = s.repo.CreateSession(ctx, session); err != nil {
		return uuid.Nil, "", false, fmt.Errorf("create session: %w", err)
	}

	if provisionalUserID != uuid.Nil {
		_ = s.repo.DeleteProvisionalUser(ctx, provisionalUserID)
	}

	if authKey != "" {
		s.publishAuthToken(authKey, token, userID.String())
		_ = s.rdb.Del(ctx, authKeyPrefix+authKey).Err()
	}

	return userID, token, isNew, nil
}

func (s *AuthService) publishAuthToken(key, token, userID string) {
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
