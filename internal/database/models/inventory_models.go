package models

import "time"

type InventoryProduct struct {
	ProductCode   string `gorm:"type:varchar(100);primaryKey"`
	ProductName   string `gorm:"type:varchar(255);not null"`
	ProductTypeID int32  `gorm:"not null"`
	SupplierID    int32  `gorm:"not null"`
	UnitOfMeasure string `gorm:"type:varchar(50)"`
	ReorderLevel  int32  `gorm:"default:0"`
	MaxStockLevel int32  `gorm:"default:0"`
	CreatedAt     time.Time
	UpdatedAt     time.Time

	ProductType *ProductType `gorm:"foreignKey:ProductTypeID;references:ID"`
	Supplier    *Supplier    `gorm:"foreignKey:SupplierID;references:ID"`
	Stocks      []Stock      `gorm:"foreignKey:ProductCode;references:ProductCode"`
}

func (InventoryProduct) TableName() string {
	return "inventory_products"
}

type Warehouse struct {
	ID            int32   `gorm:"primaryKey;autoIncrement"`
	WarehouseCode string  `gorm:"type:varchar(100);uniqueIndex;not null"`
	WarehouseName string  `gorm:"type:varchar(255);not null"`
	Location      *string `gorm:"type:varchar(255)"`
	ManagerID     *int64
	IsActive      bool `gorm:"default:true"`
	CreatedAt     time.Time
	UpdatedAt     time.Time

	Stocks []Stock `gorm:"foreignKey:WarehouseID;references:ID"`
}

func (Warehouse) TableName() string {
	return "warehouses"
}

type ProductType struct {
	ID              int32   `gorm:"primaryKey;autoIncrement"`
	ProductTypeName string  `gorm:"type:varchar(100);not null"`
	ProductTypeCode string  `gorm:"type:varchar(100);uniqueIndex;not null"`
	LastProductId   int32   `gorm:"default:0"`
	Description     *string `gorm:"type:varchar(255)"`
	CreatedAt       time.Time
	UpdatedAt       time.Time

	Products []InventoryProduct `gorm:"foreignKey:ProductTypeID;references:ID"`
}

func (ProductType) TableName() string {
	return "product_types"
}

type Supplier struct {
	ID            int32   `gorm:"primaryKey;autoIncrement"`
	SupplierCode  string  `gorm:"type:varchar(100);uniqueIndex;not null"`
	SupplierName  string  `gorm:"type:varchar(255);not null"`
	ContactPerson *string `gorm:"type:varchar(100)"`
	Phone         *string `gorm:"type:varchar(50)"`
	Email         *string `gorm:"type:varchar(100)"`
	Address       *string `gorm:"type:varchar(255)"`
	IsActive      bool    `gorm:"default:true"`
	CreatedAt     time.Time
	UpdatedAt     time.Time

	Products []InventoryProduct `gorm:"foreignKey:SupplierID;references:ID"`
}

func (Supplier) TableName() string {
	return "suppliers"
}

type Stock struct {
	ID                int64   `gorm:"primaryKey;autoIncrement"`
	ProductCode       string  `gorm:"type:varchar(100);not null;index"`
	WarehouseID       int32   `gorm:"index"`
	AvailableQuantity int32   `gorm:"default:0"`
	ReservedQuantity  int32   `gorm:"default:0"`
	UnitCost          string  `gorm:"type:varchar(50)"`
	LastRestockDate   *string `gorm:"type:varchar(50)"`
	CreatedAt         time.Time
	UpdatedAt         time.Time

	Product   *InventoryProduct `gorm:"foreignKey:ProductCode;references:ProductCode"`
	Warehouse *Warehouse        `gorm:"foreignKey:WarehouseID;references:ID"`
}

func (Stock) TableName() string {
	return "stocks"
}

type StockMovement struct {
	ID            int64   `gorm:"primaryKey;autoIncrement"`
	ProductCode   string  `gorm:"type:varchar(100);not null;index"`
	WarehouseID   int32   `gorm:"not null;index"`
	MovementType  int32   `gorm:"not null"` // 1=IN, 2=OUT, 3=TRANSFER, 4=ADJUSTMENT
	Quantity      int32   `gorm:"not null"`
	UnitCost      *string `gorm:"type:varchar(50)"`
	ReferenceType int32   // 1=PO, 2=SO, 3=TRANSFER, etc.
	ReferenceID   *string `gorm:"type:varchar(100)"`
	Notes         *string `gorm:"type:varchar(255)"`
	CreatedBy     int64   `gorm:"not null"`
	CreatedAt     time.Time
}

func (StockMovement) TableName() string {
	return "stock_movements"
}
