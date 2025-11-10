package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

type EventType string

const (
	POS_CACHE_PREFIX                      = "pos:"
	POS_PRODUCT_CACHE_KEY                 = "pos:product"
	POS_PRODUCT_GROUP_CACHE_KEY           = "pos:product-group"
	EventOrderCreated           EventType = "order.created"
	EventOrderUpdated           EventType = "order.updated"
	EventOrderVoided            EventType = "order.voided"
	EventOrderReturned          EventType = "order.returned"
	EventPaymentProcessed       EventType = "payment.processed"
	CACHE_TTL_SHORT                       = 5 * time.Minute
	CACHE_TTL_MEDIUM                      = 30 * time.Minute
	CACHE_TTL_LONG                        = 2 * time.Hour
)

//-- GORM MODEL --

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
	ID                  int64  `gorm:"primaryKey;autoIncrement"`
	DocumentId          int64  `gorm:"index;not null"`
	ProductCode         string `gorm:"not null"`
	ServingEmployeeId   *int64
	Quantity            int32  `gorm:"not null"`
	UnitPrice           string `gorm:"type:varchar(32);not null"`
	PriceBeforeDiscount string `gorm:"type:varchar(32);not null"`
	DiscountId          *int32
	DiscountAmount      string `gorm:"type:varchar(32);not null"`
	LineTotal           string `gorm:"type:varchar(32);not null"`
	CommissionAmount    string `gorm:"type:varchar(32);not null"`
	CreatedAt           time.Time

	Product  *Product  `gorm:"foreignKey:ProductCode;references:ProductCode;->"`
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
	Id                     int32   `gorm:"primaryKey;autoIncrement"`
	DiscountName           string  `gorm:"type:varchar(64);not null"`
	DiscountType           int32   `gorm:"not null"`
	DiscountValue          string  `gorm:"type:varchar(32);not null"`
	ProductCode            *string `gorm:"type:varchar(32)"`
	ProductGroupId         *int32  `gorm:"default:null"`
	MinQuantity            int32   `gorm:"not null"`
	MaxUsagePerTransaction *int64
	ValidFrom              *time.Time
	ValidUntil             *time.Time
	IsActive               bool `gorm:"not null"`
	CreatedAt              time.Time
	UpdatedAt              time.Time
	BuyQuantity            *int32 `gorm:"column:buy_quantity"`
	GetQuantity            *int32 `gorm:"column:get_quantity"`

	Product      *Product      `gorm:"foreignKey:ProductCode;references:ProductCode"`
	ProductGroup *ProductGroup `gorm:"foreignKey:ProductGroupId;references:ID"`
}

type Product struct {
	ProductCode             string  `gorm:"type:varchar(32);primaryKey;not null"`
	ProductName             string  `gorm:"type:varchar(128);not null"`
	ProductPrice            string  `gorm:"type:varchar(32);not null"`
	CostPrice               string  `gorm:"type:varchar(32);not null"`
	ImageUrl                *string `gorm:"type:varchar(256)"`
	Color                   *string `gorm:"type:varchar(32)"`
	ProductGroupId          *int32
	CommissionEligible      bool `gorm:"not null"`
	RequiresServiceEmployee bool `gorm:"not null"`
	IsActive                bool `gorm:"not null"`
	CreatedAt               time.Time
	UpdatedAt               time.Time

	ProductGroup *ProductGroup `gorm:"foreignKey:ProductGroupId"`
	OrderItems   []OrderItem   `gorm:"foreignKey:ProductCode;references:ProductCode"`
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
	ID                int64  `gorm:"primaryKey;autoIncrement"`
	CartId            int64  `gorm:"not null;index"`
	ProductCode       string `gorm:"not null"`
	ServingEmployeeId *int64
	Quantity          int32  `gorm:"not null"`
	UnitPrice         string `gorm:"type:varchar(32);not null"`
	DiscountId        *int32
	DiscountAmount    string `gorm:"type:varchar(32);default:'0.00'"`
	LineTotal         string `gorm:"type:varchar(32);not null"`
	CreatedAt         time.Time

	Product  *Product  `gorm:"foreignKey:ProductCode;references:ProductCode"`
	Discount *Discount `gorm:"foreignKey:DiscountId"`
}

type OrderEvent struct {
	EventType      EventType      `json:"event_type"`
	OrderID        int64          `json:"order_id"`
	DocumentNumber string         `json:"document_number"`
	CashierID      int64          `json:"cashier_id"`
	TotalAmount    string         `json:"total_amount"`
	PaidStatus     int32          `json:"paid_status"`
	DocumentType   int32          `json:"document_type"`
	Timestamp      time.Time      `json:"timestamp"`
	OrderData      *OrderDocument `json:"order_data,omitempty"`
}

type OrderItemEvent struct {
	ID                  int64  `json:"id"`
	DocumentID          int64  `json:"document_id"`
	ProductCode         string `json:"product_code"`
	ProductName         string `json:"product_name"`
	ServingEmployeeID   *int64 `json:"serving_employee_id,omitempty"`
	Quantity            int32  `json:"quantity"`
	UnitPrice           string `json:"unit_price"`
	PriceBeforeDiscount string `json:"price_before_discount"`
	DiscountID          *int32 `json:"discount_id,omitempty"`
	DiscountAmount      string `json:"discount_amount"`
	LineTotal           string `json:"line_total"`
	CommissionAmount    string `json:"commission_amount"`
}

// func (s *POSHandler) publishOrderEvent(ctx context.Context, event OrderEvent) error {
// 	eventJSON, err := json.Marshal(event)
// 	if err != nil {
// 		return fmt.Errorf("failed to marshal event: %w", err)
// 	}

// 	channel := fmt.Sprintf("pos.order.events.%s", event.EventType)
// 	if err := s.redis.Publish(ctx, channel, eventJSON).Err(); err != nil {
// 		return fmt.Errorf("failed to publish event: %w", err)
// 	}

// 	if err := s.redis.Publish(ctx, "pos.order.events.all", eventJSON).Err(); err != nil {
// 		return fmt.Errorf("failed to publish to all channel: %w", err)
// 	}

// 	return nil
// }

func (s *POSHandler) publishOrderEvent(ctx context.Context, event OrderEvent) error {
	log.Println("test-publishorderevent")
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Gunakan s.nats, bukan s.redis
	// Pastikan s.nats sudah diinisialisasi di NewPOSHandler (dan itu sudah ada di handler.go)

	channel := fmt.Sprintf("pos.order.events.%s", event.EventType)
	if err := s.nats.Publish(channel, eventJSON); err != nil { // <-- UBAH KE NATS
		return fmt.Errorf("failed to publish event to NATS: %w", err)
	}

	if err := s.nats.Publish("pos.order.events.all", eventJSON); err != nil { // <-- UBAH KE NATS
		return fmt.Errorf("failed to publish to NATS 'all' channel: %w", err)
	}

	// Tambahkan log ini untuk konfirmasi
	log.Printf("DEBUG POS: Berhasil publish ke NATS, channel=%s", channel)

	return nil
}

func (s *POSHandler) createOrderEvent(order *OrderDocument, eventType EventType) OrderEvent {
	items := make([]OrderItemEvent, len(order.OrderItems))
	for i, item := range order.OrderItems {
		productName := ""
		if item.Product != nil {
			productName = item.Product.ProductName
		}

		items[i] = OrderItemEvent{
			ID:                  item.ID,
			DocumentID:          item.DocumentId,
			ProductCode:         item.ProductCode,
			ProductName:         productName,
			ServingEmployeeID:   item.ServingEmployeeId,
			Quantity:            item.Quantity,
			UnitPrice:           item.UnitPrice,
			PriceBeforeDiscount: item.PriceBeforeDiscount,
			DiscountID:          item.DiscountId,
			DiscountAmount:      item.DiscountAmount,
			LineTotal:           item.LineTotal,
			CommissionAmount:    item.CommissionAmount,
		}
	}

	return OrderEvent{
		EventType:      eventType,
		OrderID:        order.ID,
		DocumentNumber: order.DocumentNumber,
		CashierID:      order.CashierId,
		TotalAmount:    order.TotalAmount,
		PaidStatus:     order.PaidStatus,
		DocumentType:   order.DocumentType,
		Timestamp:      time.Now(),
		OrderData:      order,
	}
}
