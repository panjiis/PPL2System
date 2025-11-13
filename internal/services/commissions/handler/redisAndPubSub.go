package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	proto "syntra-system/proto/protogen/commissions"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type EventType string

const (
	COMMISSION_CALCULATION_CACHE_PREFIX = "commission_calculation:"
	COMMISSION_REPORT_CACHE_PREFIX      = "commission_report:"
	EventOrderCreated                   = "order.created"
	EventOrderUpdated                   = "order.updated"
	EventOrderVoided                    = "order.voided"
	EventOrderReturned                  = "order.returned"
	EventOrderPaid                      = "payment.processed"
)

// --- GORM Models ---
type CommissionCalculation struct {
	ID         int64 `gorm:"primaryKey;autoIncrement"`
	EmployeeID int64 `gorm:"index;not null"`
	// Employee               Employee `gorm:"foreignKey:EmployeeID"`
	CalculationPeriodStart string `gorm:"not null"`
	CalculationPeriodEnd   string `gorm:"not null"`
	TotalSales             string `gorm:"type:decimal(18,2);not null"`
	BaseCommission         string `gorm:"type:decimal(18,2);not null"`
	BonusCommission        string `gorm:"type:decimal(18,2);not null"`
	TotalCommission        string `gorm:"type:decimal(18,2);not null"`
	Status                 int32  `gorm:"index;not null"`
	CalculatedBy           int64  `gorm:"not null"`
	CalculatedByName       string `gorm:"-"`
	ApprovedBy             *int64
	ApprovedByName         *string    `gorm:"-"`
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
	ProductCode             string     `gorm:"size:50;not null;index"`
	SalesAmount             string     `gorm:"type:decimal(18,2);not null"`
	CommissionRate          string     `gorm:"type:decimal(5,4);not null"`
	CommissionAmount        string     `gorm:"type:decimal(18,2);not null"`
	CreatedAt               *time.Time `gorm:"autoCreateTime"`
}

type CommissionPayment struct {
	ID                      int64  `gorm:"primaryKey;autoIncrement"`
	CommissionCalculationID int64  `gorm:"uniqueIndex;not null"`
	EmployeeID              int64  `gorm:"not null"`
	PaymentAmount           string `gorm:"type:decimal(18,2);not null"`
	PaymentDate             string `gorm:"not null"`
	PaymentMethod           string `gorm:"not null"` // Menggunakan string untuk enum di DB
	ReferenceNumber         *string
	PaidBy                  int64      `gorm:"not null"`
	Notes                   *string    `gorm:"type:text"`
	CreatedAt               *time.Time `gorm:"autoCreateTime"`
}

type SalesDataItem struct {
	ID                  int64     `gorm:"primaryKey;autoIncrement"`
	OrderItemID         int64     `gorm:"uniqueIndex;not null"` // ID asli dari pos.order_items
	OrderDocumentNumber string    `gorm:"size:50;index"`        // <--- TAMBAHKAN INI
	OrderDate           time.Time `gorm:"index"`
	EmployeeID          int64     `gorm:"index;not null"`         // Karyawan yang melayani
	ProductCode         string    `gorm:"size:50;index;not null"` // <--- KITA PAKAI INI
	ProductName         string    `gorm:"size:255"`               // <--- TAMBAHKAN INI
	SalesAmount         string    `gorm:"type:decimal(18,2);not null"`
	IsReturned          bool      `gorm:"default:false"`
	CreatedAt           time.Time
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

	ProductGroup *ProductGroup  `gorm:"foreignKey:ProductGroupId"`
	OrderItems   []POSOrderItem `gorm:"foreignKey:ProductCode;references:ProductCode"`
}

type Discount struct {
	Id                     int32   `gorm:"primaryKey;autoIncrement"`
	DiscountName           string  `gorm:"type:varchar(64);not null"`
	DiscountType           int32   `gorm:"not null"`
	DiscountValue          string  `gorm:"type:varchar(32);not null"`
	ProductCode            *string `gorm:"type:varchar(32)"`
	ProductGroupId         *int32  `gorm:"default:null"`
	MinQuantity            int32   `gorm:"not null"`
	MaxUsagePerTransaction *int32
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

// --- Struct Helper ---
type EmployeeCommissionInfo struct {
	ID             int64
	CommissionType int32  `gorm:"column:commission_type"`
	CommissionRate string `gorm:"column:commission_rate"`
}

type CommissionTierInfo struct {
	MinSalesAmount string `gorm:"column:min_sales_amount"`
	MaxSalesAmount string `gorm:"column:max_sales_amount"`
	CommissionRate string `gorm:"column:commission_rate"`
}

type OrderItemData struct {
	ID                  int64  `gorm:"column:id"`
	ProductID           int32  `gorm:"column:product_id"`
	LineTotal           string `gorm:"column:line_total"`
	OrderDocumentNumber string `gorm:"column:document_number"`
	ProductName         string `gorm:"column:product_name"`
}

type calculationResult struct {
	TotalSales      decimal.Decimal            `json:"totalSales"`
	TotalCommission decimal.Decimal            `json:"totalCommission"`
	BaseCommission  decimal.Decimal            `json:"baseCommission"`
	BonusCommission decimal.Decimal            `json:"bonusCommission"`
	Details         []CommissionDetail         `json:"details"`
	Breakdown       *proto.CommissionBreakdown `json:"breakdown"`
}

// -- Events --
type Employee struct {
	Found          bool   `json:"found"`
	Error          string `json:"error,omitempty"`
	EmployeeName   string `json:"employee_name,omitempty"`
	Email          string `json:"email,omitempty"`
	Position       string `json:"position,omitempty"`
	CommissionRate string `json:"commission_rate,omitempty"`
	CommissionType int32  `json:"commission_type,omitempty"`
}

type POSOrderEvent struct {
	EventType      string         `json:"event_type"`
	OrderID        int64          `json:"order_id"`
	DocumentNumber string         `json:"document_number"`
	CashierID      int64          `json:"cashier_id"`
	TotalAmount    string         `json:"total_amount"`
	PaidStatus     int32          `json:"paid_status"`
	DocumentType   int32          `json:"document_type"`
	Timestamp      time.Time      `json:"timestamp"`
	OrderItems     []POSOrderItem `json:"order_items"`
}

// commission/redisAndPubSub.go

// PERBAIKI: Ubah tag JSON agar cocok dengan field GORM (huruf kapital)
type POSOrderItem struct {
	ID                int64  `json:"ID"`                          // <-- Cocokkan dengan GORM
	ProductCode       string `json:"ProductCode"`                 // <-- Cocokkan dengan GORM
	ProductName       string `json:"ProductName"`                 // <-- Cocokkan dengan GORM
	ServingEmployeeID *int64 `json:"ServingEmployeeId,omitempty"` // <-- Cocokkan dengan GORM
	LineTotal         string `json:"LineTotal"`                   // <-- Cocokkan dengan GORM
}

// PERBAIKI: Ubah tag JSON agar cocok dengan field GORM (huruf kapital)
type POSOrderDocument struct {
	OrdersDate *time.Time     `json:"OrdersDate"`
	OrderItems []POSOrderItem `json:"OrderItems"`
}

type OrderEvent struct {
	EventType      EventType       `json:"event_type"`
	OrderID        int64           `json:"order_id"`
	DocumentNumber string          `json:"document_number"`
	CashierID      int64           `json:"cashier_id"`
	TotalAmount    string          `json:"total_amount"`
	PaidStatus     int32           `json:"paid_status"`
	DocumentType   int32           `json:"document_type"`
	Timestamp      time.Time       `json:"timestamp"`
	OrderData      *OrderDataEvent `json:"order_data,omitempty"`
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
	CostPrice           string `json:"cost_price"`
}

// Struct payload baru yang diratakan
type OrderDataEvent struct {
	ID             int64            `json:"ID"`
	DocumentNumber string           `json:"DocumentNumber"`
	CashierId      int64            `json:"CashierId"`
	OrdersDate     *time.Time       `json:"OrdersDate"`
	TaxAmount      string           `json:"TaxAmount"`
	TotalAmount    string           `json:"TotalAmount"` // Tambahkan ini juga
	OrderItems     []OrderItemEvent `json:"OrderItems"`  // Menggunakan slice event
}

type Manager struct {
	Found       bool   `json:"found"`
	Error       string `json:"error,omitempty"`
	ManagerName string `json:"manager_name,omitempty"`
	Email       string `json:"email,omitempty"`
}

func (c *CommissionHandler) InvalidateCommissionCaches(ctx context.Context, calcIDs ...int64) {
	// Hapus cache yang bersifat umum atau agregat
	// _ = c.redis.Del(ctx, "some_general_commission_report_key")

	// Hapus cache untuk setiap kalkulasi yang spesifik
	for _, id := range calcIDs {
		cacheKey := fmt.Sprintf("%s%d", COMMISSION_CALCULATION_CACHE_PREFIX, id)
		_ = c.redis.Del(ctx, cacheKey)

		// Anda juga bisa menghapus cache laporan yang terkait, jika ada
		// reportCacheKey := fmt.Sprintf("%s%d", COMMISSION_REPORT_CACHE_PREFIX, employeeID)
		// _ = c.redis.Del(ctx, reportCacheKey)
	}
}

func (c *CommissionHandler) SubscribeToEmployeeEvents(ctx context.Context) error {
	_, err := c.nats.Subscribe("employee.>", func(msg *nats.Msg) {
		subject := msg.Subject
		log.Printf("Received employee event from subject: %s", subject)
	})
	if err != nil {
		return err
	}

	log.Println("Successfully subscribed to employee events")
	<-ctx.Done()
	return ctx.Err()
}

func (c *CommissionHandler) SubscribeToPosEvents(ctx context.Context) error {
	_, err := c.nats.Subscribe("pos.>", func(msg *nats.Msg) {
		subject := msg.Subject
		log.Printf("Received POS event from subject: %s", subject)
	})
	if err != nil {
		return err
	}

	log.Println("Successfully subscribed to POS events")
	<-ctx.Done()
	return ctx.Err()
}

func (c *CommissionHandler) SubscribeToOrderEvents() error {
	subjectPaid := "pos.order.events.payment.processed"
	_, err := c.nats.Subscribe(subjectPaid, c.handleOrderPaidEvent)
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", subjectPaid, err)
	}

	log.Println("Successfully subscribed to POS order events")
	return nil
}

// -- NATS Events Handler --

// -- User Events Handler --
func (c *CommissionHandler) GetEmployeeDetails(ctx context.Context, employeeID int64) (*Employee, error) {
	employeeId, err := json.Marshal(map[string]interface{}{
		"employee_id": employeeID,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to marshal request data: %v", err)
	}

	msg, err := c.nats.RequestWithContext(ctx, "employee.get", employeeId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to request employee data via NATS: %v", err)
	}

	var employee Employee
	if err := json.Unmarshal(msg.Data, &employee); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to parse employee response: %v", err)
	}

	if !employee.Found {
		return nil, status.Errorf(codes.NotFound, "Employee with ID %d not found", employeeID)
	}

	return &employee, nil
}

func (c *CommissionHandler) GetEmployeeNamesBatch(ctx context.Context, employeeIDs []int64) (map[int64]string, error) {
	employeeNameMap := make(map[int64]string)

	for _, empID := range employeeIDs {
		employee, err := c.GetEmployeeDetails(ctx, empID)
		if err != nil {
			continue
		}
		employeeNameMap[empID] = employee.EmployeeName
	}

	return employeeNameMap, nil
}

// -- POS Events Handler --
func (c *CommissionHandler) handlePOSEvent(ctx context.Context, payload string) error {
	var event POSOrderEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return fmt.Errorf("failed to unmarshal POS event: %w", err)
	}

	log.Printf("Received POS event: %s for order %s", event.EventType, event.DocumentNumber)

	switch event.EventType {
	case EventOrderPaid:
		return c.handleOrderPaid(ctx, event)
	case EventOrderVoided:
		return c.handleOrderVoidedOrReturned(ctx, event)
	case EventOrderReturned:
		return c.handleOrderVoidedOrReturned(ctx, event)
	default:
		log.Printf("Ignoring event type: %s", event.EventType)
	}

	return nil
}

func (c *CommissionHandler) handleOrderPaid(ctx context.Context, event POSOrderEvent) error {
	employeeItems := make(map[int64][]POSOrderItem)

	for _, item := range event.OrderItems {
		employeeID := event.CashierID
		if item.ServingEmployeeID != nil {
			employeeID = *item.ServingEmployeeID
		}

		employeeItems[employeeID] = append(employeeItems[employeeID], item)
	}

	for employeeID, items := range employeeItems {
		if err := c.processEmployeeOrderItems(ctx, employeeID, items, event); err != nil {
			log.Printf("Error processing order items for employee %d: %v", employeeID, err)
		}
	}

	return nil
}

func (c *CommissionHandler) processEmployeeOrderItems(ctx context.Context, employeeID int64, items []POSOrderItem, event POSOrderEvent) error {
	periodStart := event.Timestamp.Format("2006-01-02")
	periodEnd := event.Timestamp.Format("2006-01-02")

	var existingCalc CommissionCalculation
	err := c.db.WithContext(ctx).
		Where("employee_id = ? AND calculation_period_start = ? AND calculation_period_end = ?",
			employeeID, periodStart, periodEnd).
		First(&existingCalc).Error

	if err != nil && err != gorm.ErrRecordNotFound {
		return fmt.Errorf("failed to query existing calculation: %w", err)
	}

	result, err := c.CalculateCommissionLogic(ctx, employeeID, periodStart, periodEnd)
	if err != nil {
		return fmt.Errorf("failed to calculate commission: %w", err)
	}

	if err == gorm.ErrRecordNotFound {
		calculationModel := CommissionCalculation{
			EmployeeID:             employeeID,
			CalculationPeriodStart: periodStart,
			CalculationPeriodEnd:   periodEnd,
			TotalSales:             result.TotalSales.StringFixed(2),
			BaseCommission:         result.BaseCommission.StringFixed(2),
			BonusCommission:        result.BonusCommission.StringFixed(2),
			TotalCommission:        result.TotalCommission.StringFixed(2),
			Status:                 int32(proto.CommissionStatus_COMMISSION_STATUS_CALCULATED),
			CalculatedBy:           event.CashierID,
			CommissionDetails:      result.Details,
			CreatedAt:              &event.Timestamp,
			UpdatedAt:              &event.Timestamp,
		}

		if err := c.db.WithContext(ctx).Create(&calculationModel).Error; err != nil {
			return fmt.Errorf("failed to create commission calculation: %w", err)
		}

		c.InvalidateCommissionCaches(ctx, calculationModel.ID)

	} else {
		err = c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("commission_calculation_id = ?", existingCalc.ID).Delete(&CommissionDetail{}).Error; err != nil {
				return fmt.Errorf("failed to delete old details: %w", err)
			}

			updates := map[string]interface{}{
				"total_sales":      result.TotalSales.StringFixed(2),
				"base_commission":  result.BaseCommission.StringFixed(2),
				"bonus_commission": result.BonusCommission.StringFixed(2),
				"total_commission": result.TotalCommission.StringFixed(2),
				"updated_at":       event.Timestamp,
			}
			if err := tx.Model(&CommissionCalculation{}).Where("id = ?", existingCalc.ID).Updates(updates).Error; err != nil {
				return fmt.Errorf("failed to update calculation header: %w", err)
			}

			for i := range result.Details {
				result.Details[i].CommissionCalculationID = existingCalc.ID
			}
			if len(result.Details) > 0 {
				if err := tx.Create(&result.Details).Error; err != nil {
					return fmt.Errorf("failed to create new details: %w", err)
				}
			}

			return nil
		})

		if err != nil {
			return fmt.Errorf("failed to update commission calculation: %w", err)
		}

		c.InvalidateCommissionCaches(ctx, existingCalc.ID)
	}

	log.Printf("Successfully processed commission for employee %d: sales=%.2f, commission=%.2f",
		employeeID, result.TotalSales.InexactFloat64(), result.TotalCommission.InexactFloat64())

	return nil
}

func (c *CommissionHandler) handleOrderVoidedOrReturned(ctx context.Context, event POSOrderEvent) error {
	employees := make(map[int64]bool)
	for _, item := range event.OrderItems {
		if item.ServingEmployeeID != nil {
			employees[*item.ServingEmployeeID] = true
		} else {
			employees[event.CashierID] = true
		}
	}

	for employeeID := range employees {
		periodDate := event.Timestamp.Format("2006-01-02")

		result, err := c.CalculateCommissionLogic(ctx, employeeID, periodDate, periodDate)
		if err != nil {
			log.Printf("Failed to recalculate commission for employee %d after void: %v", employeeID, err)
			continue
		}

		var existingCalc CommissionCalculation
		err = c.db.WithContext(ctx).
			Where("employee_id = ? AND calculation_period_start = ? AND calculation_period_end = ?",
				employeeID, periodDate, periodDate).
			First(&existingCalc).Error

		if err != nil && err != gorm.ErrRecordNotFound {
			log.Printf("Failed to get existing calculation for employee %d: %v", employeeID, err)
			continue
		}

		if err == gorm.ErrRecordNotFound {
			continue
		}

		err = c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("commission_calculation_id = ?", existingCalc.ID).Delete(&CommissionDetail{}).Error; err != nil {
				return fmt.Errorf("failed to delete old details: %w", err)
			}

			updates := map[string]interface{}{
				"total_sales":      result.TotalSales.StringFixed(2),
				"base_commission":  result.BaseCommission.StringFixed(2),
				"bonus_commission": result.BonusCommission.StringFixed(2),
				"total_commission": result.TotalCommission.StringFixed(2),
				"updated_at":       event.Timestamp,
			}
			if err := tx.Model(&CommissionCalculation{}).Where("id = ?", existingCalc.ID).Updates(updates).Error; err != nil {
				return fmt.Errorf("failed to update calculation header: %w", err)
			}

			for i := range result.Details {
				result.Details[i].CommissionCalculationID = existingCalc.ID
			}
			if len(result.Details) > 0 {
				if err := tx.Create(&result.Details).Error; err != nil {
					return fmt.Errorf("failed to create new details: %w", err)
				}
			}

			return nil
		})

		if err != nil {
			log.Printf("Failed to update calculation for employee %d after void: %v", employeeID, err)
		} else {
			c.InvalidateCommissionCaches(ctx, existingCalc.ID)
		}
	}

	return nil
}

// handleOrderPaidEvent menyimpan data penjualan baru ke tabel lokal
// commission/redisAndPubSub.go
func (c *CommissionHandler) handleOrderPaidEvent(msg *nats.Msg) {
	log.Printf("Received POS event from subject: %s", msg.Subject)

	var event OrderEvent // Gunakan struct OrderEvent BARU
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		log.Printf("ERROR: Failed to unmarshal paid event: %v. Payload: %s", err, string(msg.Data))
		return
	}

	// === PERIKSA STRUKTUR BARU ===
	if event.OrderData == nil || event.OrderData.OrdersDate == nil {
		log.Printf("ERROR: Received paid event with nil OrderData or OrderDate, skipping. Doc: %s", event.DocumentNumber)
		return
	}

	if event.OrderData.OrderItems == nil {
		log.Println("WARNING: Event received, but OrderItems array is nil.")
		return
	}

	log.Printf("DEBUG: Menerima %d item dari event.", len(event.OrderData.OrderItems))
	var itemsToSave []SalesDataItem

	// =======================================================
	// == DEBUGGING ==
	// =======================================================
	itemsJSON, err := json.MarshalIndent(event.OrderData.OrderItems, "", "  ")
	if err != nil {
		log.Printf("DEBUG: Error marshaling OrderItems for logging: %v", err)
	} else {
		log.Println("--- DEBUG: Isi dari event.OrderData.OrderItems ---")
		log.Println(string(itemsJSON)) // Cetak JSON yang rapi
		log.Println("--- DEBUG: AKHIR DARI OrderItems ---")
	}
	// =======================================================

	// === LOOP MELALUI STRUKTUR BARU ===
	for _, item := range event.OrderData.OrderItems {
		if item.ServingEmployeeID == nil || *item.ServingEmployeeID <= 0 {
			continue // Lewati jika tidak ada karyawan
		}

		itemsToSave = append(itemsToSave, SalesDataItem{
			OrderItemID:         item.ID,
			OrderDocumentNumber: event.DocumentNumber,
			OrderDate:           *event.OrderData.OrdersDate,
			EmployeeID:          *item.ServingEmployeeID,
			ProductCode:         item.ProductCode,
			ProductName:         item.ProductName,
			SalesAmount:         item.LineTotal,
			IsReturned:          false,
			CreatedAt:           time.Now(),
		})
	}

	if len(itemsToSave) == 0 {
		log.Println("No commissionable items found in this order.")
		return
	}

	// Simpan ke database (kode Anda yang sudah ada)
	if err := c.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&itemsToSave).Error; err != nil {
		log.Printf("ERROR: Failed to save sales data items: %v", err)
	} else {
		log.Printf("SUCCESS: Saved %d sales data items for Doc: %s", len(itemsToSave), event.DocumentNumber)
	}
}

// -- Manager Events Handler --
func (c *CommissionHandler) GetManagerDetails(ctx context.Context, managerID int64) (*Manager, error) {
	managerId, err := json.Marshal(map[string]interface{}{
		"manager_id": managerID,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to marshal request  %v", err)
	}

	msg, err := c.nats.RequestWithContext(ctx, "employee.manager.get", managerId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to request manager data via NATS: %v", err)
	}

	var response struct {
		Found       bool   `json:"found"`
		Error       string `json:"error,omitempty"`
		ManagerName string `json:"manager_name,omitempty"`
		Email       string `json:"email,omitempty"`
	}

	if err := json.Unmarshal(msg.Data, &response); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to parse manager response: %v", err)
	}

	if !response.Found {
		return nil, status.Errorf(codes.NotFound, "Manager with ID %d not found", managerID)
	}

	manager := &Manager{
		Found:       response.Found,
		Error:       response.Error,
		ManagerName: response.ManagerName,
		Email:       response.Email,
	}

	log.Println(manager.ManagerName)

	return manager, nil
}

func (c *CommissionHandler) GetManagerNamesBatch(ctx context.Context, managerIDs []int64) (map[int64]string, error) {
	managerNameMap := make(map[int64]string)

	for _, empID := range managerIDs {
		employee, err := c.GetManagerDetails(ctx, empID)
		if err != nil {
			continue
		}
		managerNameMap[empID] = employee.ManagerName
	}

	return managerNameMap, nil
}
