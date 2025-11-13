package models

import "time"

type InventoryProduct struct {
	ID            int32  `gorm:"primaryKey"`
	ProductCode   string `gorm:"size:100;uniqueIndex"`
	ProductName   string `gorm:"size:255"`
	ProductTypeID int32
	SupplierID    int32
	UnitOfMeasure string `gorm:"size:50"`
	ReorderLevel  int32
	MaxStockLevel int32
	IsActive      bool
	CreatedAt     time.Time
	UpdatedAt     time.Time

	ProductType *ProductType `gorm:"foreignKey:ProductTypeID"`
	Supplier    *Supplier    `gorm:"foreignKey:SupplierID"`
	Stocks      []Stock      `gorm:"foreignKey:ProductID"`
}

type Warehouse struct {
	ID            int32   `gorm:"primaryKey"`
	WarehouseCode string  `gorm:"size:100;uniqueIndex"`
	WarehouseName string  `gorm:"size:255"`
	Location      *string `gorm:"size:255"`
	ManagerID     *int64
	IsActive      bool
	CreatedAt     time.Time
	UpdatedAt     time.Time

	Stocks []Stock `gorm:"foreignKey:WarehouseID"`
}

type ProductType struct {
	ID              int32   `gorm:"primaryKey"`
	ProductTypeName string  `gorm:"size:100"`
	Description     *string `gorm:"size:255"`
	CreatedAt       time.Time
	UpdatedAt       time.Time

	Products []InventoryProduct `gorm:"foreignKey:ProductTypeID"`
}

type Supplier struct {
	ID            int32   `gorm:"primaryKey"`
	SupplierCode  string  `gorm:"size:100;uniqueIndex"`
	SupplierName  string  `gorm:"size:255"`
	ContactPerson *string `gorm:"size:100"`
	Phone         *string `gorm:"size:50"`
	Email         *string `gorm:"size:100"`
	Address       *string `gorm:"size:255"`
	IsActive      bool
	CreatedAt     time.Time
	UpdatedAt     time.Time

	Products []InventoryProduct `gorm:"foreignKey:SupplierID"`
}

type Stock struct {
	ID                int64 `gorm:"primaryKey"`
	ProductID         int32
	WarehouseID       int32
	AvailableQuantity int32
	ReservedQuantity  int32
	UnitCost          string  `gorm:"size:50"`
	LastRestockDate   *string `gorm:"size:50"`
	CreatedAt         time.Time
	UpdatedAt         time.Time

	Product   *InventoryProduct `gorm:"foreignKey:ProductID"`
	Warehouse *Warehouse        `gorm:"foreignKey:WarehouseID"`
}

type StockMovement struct {
	ID            int64 `gorm:"primaryKey"`
	ProductID     int32
	WarehouseID   int32
	MovementType  int32
	Quantity      int32
	UnitCost      *string `gorm:"size:50"`
	ReferenceType int32
	ReferenceID   *string `gorm:"size:100"`
	Notes         *string `gorm:"size:255"`
	CreatedBy     int64
	CreatedAt     time.Time
}
