package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/helthtech/core-users/internal/model"
	"gorm.io/gorm"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) CreateProvisionalUser(ctx context.Context) (*model.ProvisionalUser, error) {
	u := &model.ProvisionalUser{ID: uuid.New()}
	return u, r.db.WithContext(ctx).Create(u).Error
}

func (r *UserRepository) DeleteProvisionalUser(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.ProvisionalUser{}, "id = ?", id).Error
}

func (r *UserRepository) GetUserByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&u).Error
	return &u, err
}

func (r *UserRepository) GetUserByPhone(ctx context.Context, phone string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Where("phone_e164 = ?", phone).First(&u).Error
	return &u, err
}

func (r *UserRepository) CreateUser(ctx context.Context, u *model.User) error {
	return r.db.WithContext(ctx).Create(u).Error
}

func (r *UserRepository) UpdateUser(ctx context.Context, u *model.User) error {
	return r.db.WithContext(ctx).Save(u).Error
}

func (r *UserRepository) CreateSession(ctx context.Context, s *model.UserSession) error {
	return r.db.WithContext(ctx).Create(s).Error
}

func (r *UserRepository) GetSessionByTokenHash(ctx context.Context, hash string) (*model.UserSession, error) {
	var s model.UserSession
	err := r.db.WithContext(ctx).Where("token_hash = ? AND revoked_at IS NULL", hash).First(&s).Error
	return &s, err
}

func (r *UserRepository) UpdateUserFields(ctx context.Context, id uuid.UUID, fields map[string]any) error {
	return r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).Updates(fields).Error
}
