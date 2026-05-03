package migration

import (
	"github.com/helthtech/core-users/internal/model"
	"gorm.io/gorm"
)

func Run(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.ProvisionalUser{},
		&model.User{},
		&model.UserSession{},
	)
}
