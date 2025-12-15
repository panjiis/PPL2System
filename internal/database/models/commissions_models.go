package models

import "time"

type CommissionCalculation struct {
	ID                     int64    `gorm:"primaryKey;autoIncrement"`
	EmployeeID             int64    `gorm:"index;not null"`
	Employee               Employee `gorm:"foreignKey:EmployeeID"`
	CalculationPeriodStart string   `gorm:"not null"`
	CalculationPeriodEnd   string   `gorm:"not null"`
	TotalSales             string   `gorm:"type:decimal(18,2);not null"`
	BaseCommission         string   `gorm:"type:decimal(18,2);not null"`
	BonusCommission        string   `gorm:"type:decimal(18,2);not null"`
	TotalCommission        string   `gorm:"type:decimal(18,2);not null"`
	Status                 int32    `gorm:"index;not null"`
	CalculatedBy           int64    `gorm:"not null"`
	ApprovedBy             *int64
	Notes                  *string    `gorm:"type:text"`
	CreatedAt              *time.Time `gorm:"autoCreateTime"`
	UpdatedAt              *time.Time `gorm:"autoUpdateTime"`

	CommissionDetails []CommissionDetail `gorm:"foreignKey:CommissionCalculationID"`
	CommissionPayment CommissionPayment  `gorm:"foreignKey:CommissionCalculationID"`
}

type CommissionDetail struct {
	ID                      int64      `gorm:"primaryKey;autoIncrement"`
	CommissionCalculationID int64      `gorm:"index;not null"`
	OrderItemID             int64      `gorm:"not null"`
	ProductCode             string     `gorm:"size:100;not null;index"`
	SalesAmount             string     `gorm:"type:decimal(18,2);not null"`
	CommissionRate          string     `gorm:"type:decimal(18,2);not null"`
	CommissionAmount        string     `gorm:"type:decimal(18,2);not null"`
	CreatedAt               *time.Time `gorm:"autoCreateTime"`
}

type CommissionPayment struct {
	ID                      int64  `gorm:"primaryKey;autoIncrement"`
	CommissionCalculationID int64  `gorm:"uniqueIndex;not null"`
	EmployeeID              int64  `gorm:"not null"`
	PaymentAmount           string `gorm:"type:decimal(18,2);not null"`
	PaymentDate             string `gorm:"not null"`
	PaymentMethod           string `gorm:"not null"`
	ReferenceNumber         *string
	PaidBy                  int64      `gorm:"not null"`
	Notes                   *string    `gorm:"type:text"`
	CreatedAt               *time.Time `gorm:"autoCreateTime"`
}

type SalesDataItem struct {
	ID                  int64      `gorm:"primaryKey;autoIncrement"`
	OrderItemID         int64      `gorm:"uniqueIndex;not null"`
	OrderDocumentNumber *string    `gorm:"size:50;index"`
	OrderDate           *time.Time `gorm:"index"`
	EmployeeID          int64      `gorm:"index;not null"`
	ProductCode         string     `gorm:"size:50;index;not null"`
	ProductName         *string    `gorm:"size:255"`
	SalesAmount         string     `gorm:"type:decimal(18,2);not null"`
	IsReturned          bool       `gorm:"default:false"`
	CreatedAt           *time.Time `gorm:"autoCreateTime"` 
}
