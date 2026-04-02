package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ProvisionalUser struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	CreatedAt time.Time
}

func (ProvisionalUser) TableName() string { return "provisional_users" }

type User struct {
	ID                    uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	PhoneE164             string     `gorm:"uniqueIndex;type:text;not null"`
	PhoneVerifiedAt       *time.Time `gorm:"type:timestamptz"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
	OnboardingCompletedAt *time.Time `gorm:"type:timestamptz"`
	Locale                string     `gorm:"type:text"`
	Timezone              string     `gorm:"type:text"`
	DisplayName           string     `gorm:"type:text"`
	BirthDate             string     `gorm:"type:text"`
	Sex                   string     `gorm:"type:text"`
}

func (User) TableName() string { return "users" }

func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return nil
}

type UserSession struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID    uuid.UUID  `gorm:"type:uuid;not null;index"`
	TokenHash string     `gorm:"type:text;uniqueIndex;not null"`
	ExpiresAt time.Time  `gorm:"type:timestamptz;not null"`
	RevokedAt *time.Time `gorm:"type:timestamptz"`
	CreatedAt time.Time
}

func (UserSession) TableName() string { return "user_sessions" }
