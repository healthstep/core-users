package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/helthtech/core-users/internal/model"
	"github.com/helthtech/core-users/internal/repository"
)

type UserService struct {
	repo *repository.UserRepository
	jwt  *JWTService
}

func NewUserService(repo *repository.UserRepository, jwt *JWTService) *UserService {
	return &UserService{repo: repo, jwt: jwt}
}

func (s *UserService) GetUser(ctx context.Context, id uuid.UUID) (*model.User, error) {
	return s.repo.GetUserByID(ctx, id)
}

func (s *UserService) UpdateUser(ctx context.Context, id uuid.UUID, updates map[string]any) (*model.User, error) {
	u, err := s.repo.GetUserByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	if v, ok := updates["display_name"]; ok {
		u.DisplayName = v.(string)
	}
	if v, ok := updates["locale"]; ok {
		u.Locale = v.(string)
	}
	if v, ok := updates["timezone"]; ok {
		u.Timezone = v.(string)
	}
	if v, ok := updates["birth_date"]; ok {
		u.BirthDate = v.(string)
	}
	if v, ok := updates["sex"]; ok {
		u.Sex = v.(string)
	}
	if v, ok := updates["onboarding_completed"]; ok && v.(bool) {
		now := time.Now()
		u.OnboardingCompletedAt = &now
	}

	if err = s.repo.UpdateUser(ctx, u); err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}
	return u, nil
}

func (s *UserService) ValidateToken(tokenStr string) (valid bool, userID string, userType string, err error) {
	claims, err := s.jwt.Validate(tokenStr)
	if err != nil {
		return false, "", "", nil
	}
	return true, claims.UserID, claims.UserType, nil
}
