package database

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type StringArray []string

func (a *StringArray) Scan(value interface{}) error {
	if value == nil {
		*a = []string{}
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to scan StringArray: %v", value)
	}

	return json.Unmarshal(bytes, a)
}

func (a StringArray) Value() (driver.Value, error) {
	if a == nil {
		return "[]", nil
	}
	return json.Marshal(a)
}

func NewConnection(dsn string) (*gorm.DB, error) {
	if dsn == "" {
		log.Fatal("DSN is required")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("failed to get sql.DB: %v", err)
	}

	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(time.Hour)

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping DB: %w", err)
	}

	return db, nil
}

// User Database
type User struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	Username  string `gorm:"uniqueIndex;not null"`
	Email     string `gorm:"uniqueIndex;not null"`
	Password  string `gorm:"not null"`
	Firstname string `gorm:"not null"`
	Lastname  string `gorm:"not null"`
	RoleID    int32  `gorm:"not null"`
	Role      Role   `gorm:"foreignKey:RoleID"`
	IsActive  bool   `gorm:"default:false"`
	LastLogin *time.Time
	CreatedAt *time.Time `gorm:"autoCreateTime"`
	UpdatedAt *time.Time `gorm:"autoUpdateTime"`
}

type Role struct {
	ID          int32      `gorm:"primaryKey;autoIncrement"`
	RoleName    string     `gorm:"uniqueIndex;not null"`
	AccessLevel int32      `gorm:"not null"`
	Permissions string     `gorm:"type:text"`
	CreatedAt   *time.Time `gorm:"autoCreateTime"`
	UpdatedAt   *time.Time `gorm:"autoUpdateTime"`
}

type Employee struct {
	ID             int64  `gorm:"primaryKey;autoIncrement"`
	EmployeeName   string `gorm:"not null"`
	Position       string `gorm:"column:position"`
	Phone          string
	Email          string
	Address        string `gorm:"type:text"`
	HireDate       string
	BaseSalary     string     `gorm:"not null"`
	CommissionRate string     `gorm:"not null"`
	CommissionType int32      `gorm:"not null"`
	IsActive       bool       `gorm:"default:false"`
	CreatedAt      *time.Time `gorm:"autoCreateTime"`
	UpdatedAt      *time.Time `gorm:"autoUpdateTime"`

	CommissionTiers []CommissionTier `gorm:"foreignKey:EmployeeID"`
}

type CommissionTier struct {
	ID             int32  `gorm:"primaryKey;autoIncrement"`
	EmployeeID     int64  `gorm:"not null"`
	MinSalesAmount string `gorm:"not null"`
	MaxSalesAmount string
	CommissionRate string     `gorm:"not null"`
	CreatedAt      *time.Time `gorm:"autoCreateTime"`
	UpdatedAt      *time.Time `gorm:"autoUpdateTime"`
}

func MigrateUserDB(db *gorm.DB) error {
	db.AutoMigrate(&User{})
	db.AutoMigrate(&Role{})
	db.AutoMigrate(&Employee{})
	db.AutoMigrate(&CommissionTier{})
	return nil
}
