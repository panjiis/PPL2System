package models

import "time"

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
