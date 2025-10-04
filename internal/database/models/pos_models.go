package models

import "time"

type OrderDocument struct {
	ID             int64      `gorm:"primaryKey;autoIncrement"`
	DocumentNumber string     `gorm:"uniqueIndex;not null"`
	CashierId      int64      `gorm:"not null"`
	OrdersDate     *time.Time `gorm:"not null"`
	DocumentType   int32      `gorm:"not null"`
	PaymentTypeId  *int32     // optional

	Subtotal       string `gorm:"type:varchar(32);not null"`
	TaxAmount      string `gorm:"type:varchar(32);not null"`
	DiscountAmount string `gorm:"type:varchar(32);not null"`
	TotalAmount    string `gorm:"type:varchar(32);not null"`
	PaidAmount     string `gorm:"type:varchar(32);not null"`
	ChangeAmount   string `gorm:"type:varchar(32);not null"`
	PaidStatus     int32  `gorm:"not null"`

	AdditionalInfo *string `gorm:"type:text"`
	Notes          *string `gorm:"type:text"`

	CreatedAt time.Time
	UpdatedAt time.Time

	OrderItems  []OrderItem  `gorm:"foreignKey:DocumentId"`
	PaymentType *PaymentType `gorm:"foreignKey:PaymentTypeId;references:ID"`
}

type OrderItem struct {
	ID                  int64 `gorm:"primaryKey;autoIncrement"`
	DocumentId          int64 `gorm:"index;not null"`
	ProductId           int32 `gorm:"not null"`
	ServingEmployeeId   *int64
	Quantity            int32  `gorm:"not null"`
	UnitPrice           string `gorm:"type:varchar(32);not null"`
	PriceBeforeDiscount string `gorm:"type:varchar(32);not null"`
	DiscountId          *int32
	DiscountAmount      string `gorm:"type:varchar(32);not null"`
	LineTotal           string `gorm:"type:varchar(32);not null"`
	CommissionAmount    string `gorm:"type:varchar(32);not null"`
	CreatedAt           time.Time

	Product  *Product  `gorm:"foreignKey:ProductId"`
	Discount *Discount `gorm:"foreignKey:DiscountId"`
}

type PaymentType struct {
	ID                int32  `gorm:"primaryKey;autoIncrement"`
	PaymentName       string `gorm:"type:varchar(64);not null"`
	IsActive          bool   `gorm:"not null"`
	ProcessingFeeRate string `gorm:"type:varchar(32);not null"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Discount struct {
	ID                     int32  `gorm:"primaryKey;autoIncrement"`
	DiscountName           string `gorm:"type:varchar(64);not null"`
	DiscountType           int32  `gorm:"not null"`
	DiscountValue          string `gorm:"type:varchar(32);not null"`
	ProductId              *int32
	ProductGroupId         *int32
	MinQuantity            int32 `gorm:"not null"`
	MaxUsagePerTransaction *int32
	ValidFrom              *time.Time
	ValidUntil             *time.Time
	IsActive               bool `gorm:"not null"`
	CreatedAt              time.Time
	UpdatedAt              time.Time

	Product      *Product      `gorm:"foreignKey:ProductId"`
	ProductGroup *ProductGroup `gorm:"foreignKey:ProductGroupId"`
}

type Product struct {
	ID                      int32  `gorm:"primaryKey;autoIncrement"`
	ProductCode             string `gorm:"type:varchar(32);uniqueIndex;not null"`
	ProductName             string `gorm:"type:varchar(128);not null"`
	ProductPrice            string `gorm:"type:varchar(32);not null"`
	CostPrice               string `gorm:"type:varchar(32);not null"`
	ProductGroupId          *int32
	CommissionEligible      bool `gorm:"not null"`
	RequiresServiceEmployee bool `gorm:"not null"`
	IsActive                bool `gorm:"not null"`
	CreatedAt               time.Time
	UpdatedAt               time.Time

	ProductGroup *ProductGroup `gorm:"foreignKey:ProductGroupId"`
}

type ProductGroup struct {
	ID               int32  `gorm:"primaryKey;autoIncrement"`
	ProductGroupName string `gorm:"type:varchar(128);not null"`
	ParentGroupId    *int32
	Color            *string `gorm:"type:varchar(32)"`
	ImageUrl         *string `gorm:"type:varchar(256)"`
	CommissionRate   string  `gorm:"type:varchar(32);not null"`
	IsActive         bool    `gorm:"not null"`
	CreatedAt        time.Time
	UpdatedAt        time.Time

	ParentGroup *ProductGroup  `gorm:"foreignKey:ParentGroupId"`
	ChildGroups []ProductGroup `gorm:"foreignKey:ParentGroupId"`
	Products    []Product      `gorm:"foreignKey:ProductGroupId"`
}

type Cart struct {
	ID             int64  `gorm:"primaryKey;autoIncrement"`
	CashierId      int64  `gorm:"not null;index"`
	Status         int32  `gorm:"not null;default:0"`
	Subtotal       string `gorm:"type:varchar(32);default:'0.00'"`
	TaxAmount      string `gorm:"type:varchar(32);default:'0.00'"`
	DiscountAmount string `gorm:"type:varchar(32);default:'0.00'"`
	TotalAmount    string `gorm:"type:varchar(32);default:'0.00'"`
	CreatedAt      time.Time
	UpdatedAt      time.Time

	CartItems []CartItem `gorm:"foreignKey:CartId"`
}

type CartItem struct {
	ID                int64 `gorm:"primaryKey;autoIncrement"`
	CartId            int64 `gorm:"not null;index"`
	ProductId         int32 `gorm:"not null"`
	ServingEmployeeId *int64
	Quantity          int32  `gorm:"not null"`
	UnitPrice         string `gorm:"type:varchar(32);not null"`
	DiscountId        *int32
	DiscountAmount    string `gorm:"type:varchar(32);default:'0.00'"`
	LineTotal         string `gorm:"type:varchar(32);not null"`
	CreatedAt         time.Time

	Product  *Product  `gorm:"foreignKey:ProductId"`
	Discount *Discount `gorm:"foreignKey:DiscountId"`
}
