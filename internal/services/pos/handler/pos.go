package handler

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"

	proto "syntra-system/proto/protogen/pos"
)

const (
	POS_CACHE_PREFIX            = "pos:"
	POS_PRODUCT_CACHE_KEY       = "pos:product"
	POS_PRODUCT_GROUP_CACHE_KEY = "pos:product-group"
	EventOrderCreated           = "order.created"
	EventOrderUpdated           = "order.updated"
	EventOrderVoided            = "order.voided"
	EventOrderReturned          = "order.returned"
	EventPaymentProcessed       = "payment.processed"
	CACHE_TTL_SHORT             = 5 * time.Minute
	CACHE_TTL_MEDIUM            = 30 * time.Minute
	CACHE_TTL_LONG              = 2 * time.Hour
)

// --- Helpers ---
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func int32Ptr(i int32) *int32 {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}

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

func timeNowOrZero(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

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

// -- Handler --
type POSHandler struct {
	proto.UnimplementedPOSServiceServer
	db    *gorm.DB
	redis *redis.Client
}

func NewPOSHandler(db *gorm.DB, redisClient *redis.Client) *POSHandler {
	return &POSHandler{
		db:    db,
		redis: redisClient,
	}
}

func (s *POSHandler) InvalidatePOSCaches(ctx context.Context, productIDs ...int64) {
	_ = s.redis.Del(ctx, POS_PRODUCT_CACHE_KEY, POS_PRODUCT_GROUP_CACHE_KEY)

	for _, id := range productIDs {
		cacheKeys := fmt.Sprintf("%s%d", POS_CACHE_PREFIX, id)
		_ = s.redis.Del(ctx, cacheKeys)
	}
}

// -- MODEL TO PROTO HANDLER --
func (s *POSHandler) orderDocumentToProto(doc OrderDocument) *proto.OrderDocument {
	orderItems := make([]*proto.OrderItem, 0, len(doc.OrderItems))
	for _, item := range doc.OrderItems {
		orderItems = append(orderItems, s.orderItemToProto(item))
	}

	var paymentType *proto.PaymentType
	if doc.PaymentType != nil {
		paymentType = s.paymentTypeToProto(*doc.PaymentType)
	}

	return &proto.OrderDocument{
		Id:             doc.ID,
		DocumentNumber: doc.DocumentNumber,
		CashierId:      doc.CashierId,
		OrdersDate:     timestamppb.New(timeNowOrZero(doc.OrdersDate)),
		DocumentType:   proto.DocumentType(doc.DocumentType),
		PaymentTypeId:  doc.PaymentTypeId,

		Subtotal:       doc.Subtotal,
		TaxAmount:      doc.TaxAmount,
		DiscountAmount: doc.DiscountAmount,
		TotalAmount:    doc.TotalAmount,
		PaidAmount:     doc.PaidAmount,
		ChangeAmount:   doc.ChangeAmount,
		PaidStatus:     proto.PaidStatus(doc.PaidStatus),

		AdditionalInfo: doc.AdditionalInfo,
		Notes:          doc.Notes,

		CreatedAt:   timestamppb.New(doc.CreatedAt),
		UpdatedAt:   timestamppb.New(doc.UpdatedAt),
		OrderItems:  orderItems,
		PaymentType: paymentType,
	}
}

func (s *POSHandler) orderItemToProto(item OrderItem) *proto.OrderItem {
	var product *proto.Product
	if item.Product != nil {
		product = s.productToProto(*item.Product)
	}
	var discount *proto.Discount
	if item.Discount != nil {
		discount = s.discountToProto(*item.Discount)
	}

	return &proto.OrderItem{
		Id:                  item.ID,
		DocumentId:          item.DocumentId,
		ProductId:           item.ProductId,
		ServingEmployeeId:   item.ServingEmployeeId,
		Quantity:            item.Quantity,
		UnitPrice:           item.UnitPrice,
		PriceBeforeDiscount: item.PriceBeforeDiscount,
		DiscountId:          item.DiscountId,
		DiscountAmount:      item.DiscountAmount,
		LineTotal:           item.LineTotal,
		CommissionAmount:    item.CommissionAmount,
		CreatedAt:           timestamppb.New(item.CreatedAt),
		Product:             product,
		Discount:            discount,
	}
}

func (s *POSHandler) paymentTypeToProto(p PaymentType) *proto.PaymentType {
	return &proto.PaymentType{
		Id:                p.ID,
		PaymentName:       p.PaymentName,
		IsActive:          p.IsActive,
		ProcessingFeeRate: p.ProcessingFeeRate,
		CreatedAt:         timestamppb.New(p.CreatedAt),
		UpdatedAt:         timestamppb.New(p.UpdatedAt),
	}
}

func (s *POSHandler) discountToProto(d Discount) *proto.Discount {
	var product *proto.Product
	if d.Product != nil {
		product = s.productToProto(*d.Product)
	}
	var productGroup *proto.ProductGroup
	if d.ProductGroup != nil {
		productGroup = s.productGroupToProto(*d.ProductGroup)
	}

	return &proto.Discount{
		Id:                     d.ID,
		DiscountName:           d.DiscountName,
		DiscountType:           proto.DiscountType(d.DiscountType),
		DiscountValue:          d.DiscountValue,
		ProductId:              d.ProductId,
		ProductGroupId:         d.ProductGroupId,
		MinQuantity:            d.MinQuantity,
		MaxUsagePerTransaction: d.MaxUsagePerTransaction,
		ValidFrom:              timestamppb.New(timeNowOrZero(d.ValidFrom)),
		ValidUntil:             timestamppb.New(timeNowOrZero(d.ValidUntil)),
		IsActive:               d.IsActive,
		CreatedAt:              timestamppb.New(d.CreatedAt),
		UpdatedAt:              timestamppb.New(d.UpdatedAt),
		Product:                product,
		ProductGroup:           productGroup,
	}
}

func (s *POSHandler) productToProto(p Product) *proto.Product {
	var productGroup *proto.ProductGroup
	if p.ProductGroup != nil {
		productGroup = s.productGroupToProto(*p.ProductGroup)
	}

	return &proto.Product{
		Id:                      p.ID,
		ProductCode:             p.ProductCode,
		ProductName:             p.ProductName,
		ProductPrice:            p.ProductPrice,
		CostPrice:               p.CostPrice,
		ProductGroupId:          p.ProductGroupId,
		CommissionEligible:      p.CommissionEligible,
		RequiresServiceEmployee: p.RequiresServiceEmployee,
		IsActive:                p.IsActive,
		CreatedAt:               timestamppb.New(p.CreatedAt),
		UpdatedAt:               timestamppb.New(p.UpdatedAt),
		ProductGroup:            productGroup,
	}
}

func (s *POSHandler) productGroupToProto(pg ProductGroup) *proto.ProductGroup {
	childGroups := make([]*proto.ProductGroup, 0, len(pg.ChildGroups))
	for _, c := range pg.ChildGroups {
		childGroups = append(childGroups, s.productGroupToProto(c))
	}

	products := make([]*proto.Product, 0, len(pg.Products))
	for _, prod := range pg.Products {
		products = append(products, s.productToProto(prod))
	}

	return &proto.ProductGroup{
		Id:               pg.ID,
		ProductGroupName: pg.ProductGroupName,
		ParentGroupId:    pg.ParentGroupId,
		Color:            pg.Color,
		ImageUrl:         pg.ImageUrl,
		CommissionRate:   pg.CommissionRate,
		IsActive:         pg.IsActive,
		CreatedAt:        timestamppb.New(pg.CreatedAt),
		UpdatedAt:        timestamppb.New(pg.UpdatedAt),
		ParentGroup:      nil,
		ChildGroups:      childGroups,
		Products:         products,
	}
}

func (s *POSHandler) cartToProto(cart Cart) *proto.Cart {
	cartItems := make([]*proto.CartItem, 0, len(cart.CartItems))
	for _, item := range cart.CartItems {
		cartItems = append(cartItems, s.cartItemToProto(item))
	}

	return &proto.Cart{
		CartId:         strconv.FormatInt(cart.ID, 10),
		CashierId:      cart.CashierId,
		Items:          cartItems,
		Subtotal:       cart.Subtotal,
		TaxAmount:      cart.TaxAmount,
		DiscountAmount: cart.DiscountAmount,
		TotalAmount:    cart.TotalAmount,
		CreatedAt:      timestamppb.New(cart.CreatedAt),
		UpdatedAt:      timestamppb.New(cart.UpdatedAt),
	}
}

func (s *POSHandler) cartItemToProto(item CartItem) *proto.CartItem {
	var product *proto.Product
	if item.Product != nil {
		product = s.productToProto(*item.Product)
	}

	var discount *proto.Discount
	if item.Discount != nil {
		discount = s.discountToProto(*item.Discount)
	}

	return &proto.CartItem{
		ItemId:            strconv.FormatInt(item.ID, 10),
		ProductId:         item.ProductId,
		ServingEmployeeId: item.ServingEmployeeId,
		Quantity:          item.Quantity,
		UnitPrice:         item.UnitPrice,
		DiscountId:        item.DiscountId,
		DiscountAmount:    item.DiscountAmount,
		LineTotal:         item.LineTotal,
		Product:           product,
		Discount:          discount,
	}
}

// -- POS PRODUCTS --

func (s *POSHandler) GetProduct(ctx context.Context, req *proto.GetProductRequest) (*proto.GetProductResponse, error) {
	var product Product

	if req.GetId() == 0 {
		return &proto.GetProductResponse{
			Success: false,
			Message: strPtr("Product_id must be provided"),
		}, nil
	}

	if err := s.db.Find(req.GetId()).First(&product).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.GetProductResponse{
				Success: false,
				Message: strPtr("Product not found"),
			}, err
		} else {

			return &proto.GetProductResponse{
				Success: false,
				Message: strPtr("database error"),
			}, err
		}
	}

	return &proto.GetProductResponse{
		Success: true,
		Product: s.productToProto(product),
	}, nil
}

func (s *POSHandler) GetProductByCode(crx context.Context, req *proto.GetProductByCodeRequest) (*proto.GetProductByCodeResponse, error) {
	var product Product

	if req.GetProductCode() == "" {
		return &proto.GetProductByCodeResponse{
			Success: false,
			Message: strPtr("Product_id must be provided"),
		}, nil
	}

	if err := s.db.Where("product_code = ?", req.GetProductCode()).First(&product).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.GetProductByCodeResponse{
				Success: false,
				Message: strPtr("Product not found"),
			}, err
		} else {

			return &proto.GetProductByCodeResponse{
				Success: false,
				Message: strPtr("database error"),
			}, err
		}
	}

	return &proto.GetProductByCodeResponse{
		Success: true,
		Product: s.productToProto(product),
	}, nil
}

func (s *POSHandler) ListProducts(ctx context.Context, req *proto.ListProductsRequest) (*proto.ListProductsResponse, error) {
	var products []Product
	var total int64

	query := s.db.Model(&Product{}).Preload("ProductGroup")

	if req.IsActive != nil {
		query = query.Where("is_active = ?", req.GetIsActive())
	} else if req.ProductGroupId != nil {
		query = query.Where("product_group _id = ?", req.GetProductGroupId())
	} else if req.SearchTerm != nil {
		searchTerm := "%" + req.GetSearchTerm() + "%"
		query = query.Where(
			"product_code ILIKE ? OR product_name ILIKE ?",
			searchTerm, searchTerm,
		)
	}

	if err := query.Count(&total).Error; err != nil {
		return &proto.ListProductsResponse{
			Success: false,
			Message: strPtr("database error"),
		}, err
	}

	pageSize := int(req.GetPagination().GetPageSize())
	if pageSize <= 0 {
		pageSize = 10
	}

	pageNumber := 1
	if token := req.GetPagination().GetPageToken(); token != "" {
		if n, err := strconv.Atoi(token); err == nil && n > 0 {
			pageNumber = n
		}
	}

	offset := (pageNumber - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Find(&products).Error; err != nil {
		return &proto.ListProductsResponse{
			Success: false,
			Message: strPtr("database error"),
		}, err
	}

	protoProducts := make([]*proto.Product, len(products))
	for i, prod := range products {
		protoProducts[i] = s.productToProto(prod)
	}

	nextPageToken := ""
	if int64(pageNumber*pageSize) < total {
		nextPageToken = strconv.Itoa(pageNumber + 1)
	}

	return &proto.ListProductsResponse{
		Success:  true,
		Products: protoProducts,
		Pagination: &proto.PaginationResponse{
			NextPageToken: nextPageToken,
			TotalCount:    int32(total),
		},
	}, nil
}

// -- Product Groups --

func (s *POSHandler) ListProductGroups(ctx context.Context, req *proto.ListProductGroupsRequest) (*proto.ListProductGroupsResponse, error) {
	var productGroups []ProductGroup
	var total int64

	query := s.db.Model(&ProductGroup{}).Preload("Products")

	if req.IsActive != nil {
		query = query.Where("is_active = ?", req.GetIsActive())
	} else if req.ParentGroupId != nil {
		query = query.Where("parent_group = ?", req.GetParentGroupId())
	}

	if err := query.Count(&total).Error; err != nil {
		return &proto.ListProductGroupsResponse{
			Success: false,
			Message: strPtr("database error"),
		}, err
	}

	pageSize := int(req.GetPagination().GetPageSize())
	if pageSize <= 0 {
		pageSize = 10
	}

	pageNumber := 1
	if token := req.GetPagination().GetPageToken(); token != "" {
		if n, err := strconv.Atoi(token); err == nil && n > 0 {
			pageNumber = n
		}
	}

	offset := (pageNumber - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Find(&productGroups).Error; err != nil {
		return &proto.ListProductGroupsResponse{
			Success: false,
			Message: strPtr("database error"),
		}, err
	}

	protoProductGroups := make([]*proto.ProductGroup, len(productGroups))
	for i, pg := range productGroups {
		protoProductGroups[i] = s.productGroupToProto(pg)
	}

	nextPageToken := ""
	if int64(pageNumber*pageSize) < total {
		nextPageToken = strconv.Itoa(pageNumber + 1)
	}

	return &proto.ListProductGroupsResponse{
		Success:       true,
		ProductGroups: protoProductGroups,
		Pagination: &proto.PaginationResponse{
			NextPageToken: nextPageToken,
			TotalCount:    int32(total),
		},
	}, nil
}

// -- Payment Method --

func (s *POSHandler) ListPaymentTypes(ctx context.Context, req *proto.ListPaymentTypesRequest) (*proto.ListPaymentTypesResponse, error) {
	var paymentTypes []PaymentType

	query := s.db.Model(&PaymentType{})
	if req.IsActive != nil {
		query = query.Where("is_active = ?", req.GetIsActive())
	}

	if err := query.Find(&paymentTypes).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.ListPaymentTypesResponse{
				Success: false,
				Message: strPtr("Payment Type not found"),
			}, err
		}
		return &proto.ListPaymentTypesResponse{
			Success: false,
			Message: strPtr("database error"),
		}, err
	}

	protoPaymentTypes := make([]*proto.PaymentType, len(paymentTypes))
	for i, pt := range paymentTypes {
		protoPaymentTypes[i] = s.paymentTypeToProto(pt)
	}

	return &proto.ListPaymentTypesResponse{
		Success:      true,
		PaymentTypes: protoPaymentTypes,
	}, nil
}

// -- Payment Process --
func (s *POSHandler) ProcessPayment(ctx context.Context, req *proto.ProcessPaymentRequest) (*proto.ProcessPaymentResponse, error) {
	var order OrderDocument

	changeAmount := strconv.FormatFloat(0, 'f', 2, 64)

	if req.GetOrderId() == 0 {
		return &proto.ProcessPaymentResponse{
			Success: false,
			Message: strPtr("order_id required"),
		}, nil
	}

	if err := s.db.Where("id = ?", req.GetOrderId()).First(&order).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.ProcessPaymentResponse{
				Success: false,
				Message: strPtr("Order Not Found"),
			}, nil
		}
		return &proto.ProcessPaymentResponse{
			Success: false,
			Message: strPtr("database error"),
		}, err
	}

	if order.PaidStatus == 1 {
		return &proto.ProcessPaymentResponse{
			Success: false,
			Message: strPtr("Order already paid"),
		}, nil
	}

	if req.GetPaymentTypeId() == 1 {
		paidAmount, err := strconv.ParseFloat(req.GetPaidAmount(), 64)
		if err != nil {
			return &proto.ProcessPaymentResponse{
				Success: false,
				Message: strPtr("Invalid paid amount format"),
			}, nil
		}

		totalAmount, err := strconv.ParseFloat(order.TotalAmount, 64)
		if err != nil {
			return &proto.ProcessPaymentResponse{
				Success: false,
				Message: strPtr("Invalid total amount"),
			}, err
		}

		if paidAmount < totalAmount {
			return &proto.ProcessPaymentResponse{
				Success: false,
				Message: strPtr("Insufficient payment amount"),
			}, nil
		}

		paymentChange := paidAmount - totalAmount
		changeAmount = strconv.FormatFloat(paymentChange, 'f', 2, 64)
	}

	order.PaidStatus = 1
	order.PaymentTypeId = int32Ptr(req.PaymentTypeId)

	if err := s.db.Save(&order).Error; err != nil {
		return &proto.ProcessPaymentResponse{
			Success: false,
			Message: strPtr("Failed to update order: " + err.Error()),
		}, err
	}

	return &proto.ProcessPaymentResponse{
		Success:       true,
		Message:       strPtr("Payment processed successfully"),
		OrderDocument: s.orderDocumentToProto(order),
		ChangeAmount:  changeAmount,
	}, nil
}

// -- Discount --
func (s *POSHandler) ListDiscounts(ctx context.Context, req *proto.ListDiscountsRequest) (*proto.ListDiscountsResponse, error) {
	var discounts []Discount
	var total int64

	query := s.db.Model(&Discount{}).
		Preload("Product.ProductGroup").
		Preload("ProductGroup")

	if req.IsActive != nil {
		query = query.Where("is_active = ?", req.GetIsActive())
	}

	if req.ProductId != nil {
		query = query.Where("discounts.product_id = ?", req.GetProductId())
	}

	if req.SearchTerm != nil && req.GetSearchTerm() != "" {
		searchTerm := "%" + req.GetSearchTerm() + "%"

		query = query.
			Joins("LEFT JOIN products ON products.id = discounts.product_id").
			Joins("LEFT JOIN product_groups ON product_groups.id = discounts.product_group_id").
			Where(
				"discounts.discount_name ILIKE ? OR products.product_name ILIKE ? OR product_groups.product_group_name ILIKE ?",
				searchTerm, searchTerm, searchTerm,
			)
	}

	if err := query.Distinct("discounts.id").Count(&total).Error; err != nil {
		return &proto.ListDiscountsResponse{
			Success: false,
			Message: strPtr("Database error counting discounts"),
		}, err
	}

	pageSize := int(req.GetPagination().GetPageSize())
	if pageSize <= 0 {
		pageSize = 10
	}

	pageNumber := 1
	if token := req.GetPagination().GetPageToken(); token != "" {
		if n, err := strconv.Atoi(token); err == nil && n > 0 {
			pageNumber = n
		}
	}

	offset := (pageNumber - 1) * pageSize

	if err := query.Distinct("discounts.*").Offset(offset).Limit(pageSize).Find(&discounts).Error; err != nil {
		return &proto.ListDiscountsResponse{
			Success: false,
			Message: strPtr("Database error fetching discounts"),
		}, err
	}

	protoDiscounts := make([]*proto.Discount, len(discounts))
	for i, disc := range discounts {
		protoDiscounts[i] = s.discountToProto(disc)
	}

	nextPageToken := ""
	if int64(pageNumber*pageSize) < total {
		nextPageToken = strconv.Itoa(pageNumber + 1)
	}

	return &proto.ListDiscountsResponse{
		Success:   true,
		Discounts: protoDiscounts,
		Pagination: &proto.PaginationResponse{
			NextPageToken: nextPageToken,
			TotalCount:    int32(total),
		},
	}, nil
}

func (s *POSHandler) ValidateDiscount(ctx context.Context, req *proto.ValidateDiscountRequest) (*proto.ValidateDiscountResponse, error) {
	if req.GetDiscountId() == 0 {
		return &proto.ValidateDiscountResponse{
			Success:                  false,
			Message:                  strPtr("discount_id required"),
			IsValid:                  false,
			Reason:                   strPtr("Discount ID is required"),
			CalculatedDiscountAmount: "0.00",
		}, nil
	}

	var discount Discount
	if err := s.db.Where("id = ?", req.GetDiscountId()).
		Preload("Product.ProductGroup").
		Preload("ProductGroup").
		First(&discount).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.ValidateDiscountResponse{
				Success:                  false,
				Message:                  strPtr("Discount not found"),
				IsValid:                  false,
				Reason:                   strPtr("Discount does not exist"),
				CalculatedDiscountAmount: "0.00",
			}, nil
		}
		return &proto.ValidateDiscountResponse{
			Success:                  false,
			Message:                  strPtr("Database error"),
			IsValid:                  false,
			CalculatedDiscountAmount: "0.00",
		}, err
	}

	if !discount.IsActive {
		return &proto.ValidateDiscountResponse{
			Success:                  true,
			IsValid:                  false,
			Reason:                   strPtr("Discount is not active"),
			CalculatedDiscountAmount: "0.00",
		}, nil
	}

	now := time.Now()
	if discount.ValidFrom != nil && now.Before(*discount.ValidFrom) {
		return &proto.ValidateDiscountResponse{
			Success:                  true,
			IsValid:                  false,
			Reason:                   strPtr(fmt.Sprintf("Discount will be valid from %s", discount.ValidFrom.Format("2006-01-02 15:04:05"))),
			CalculatedDiscountAmount: "0.00",
		}, nil
	}

	if discount.ValidUntil != nil && now.After(*discount.ValidUntil) {
		return &proto.ValidateDiscountResponse{
			Success:                  true,
			IsValid:                  false,
			Reason:                   strPtr(fmt.Sprintf("Discount expired on %s", discount.ValidUntil.Format("2006-01-02 15:04:05"))),
			CalculatedDiscountAmount: "0.00",
		}, nil
	}

	if req.ProductId != nil {
		productId := req.GetProductId()

		var product Product
		if err := s.db.Where("id = ?", productId).
			Preload("ProductGroup").
			First(&product).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return &proto.ValidateDiscountResponse{
					Success:                  true,
					IsValid:                  false,
					Reason:                   strPtr("Product not found"),
					CalculatedDiscountAmount: "0.00",
				}, nil
			}
			return &proto.ValidateDiscountResponse{
				Success:                  false,
				Message:                  strPtr("Database error"),
				IsValid:                  false,
				CalculatedDiscountAmount: "0.00",
			}, err
		}

		if !product.IsActive {
			return &proto.ValidateDiscountResponse{
				Success:                  true,
				IsValid:                  false,
				Reason:                   strPtr("Product is not active"),
				CalculatedDiscountAmount: "0.00",
			}, nil
		}

		if discount.ProductId != nil {
			if *discount.ProductId != productId {
				return &proto.ValidateDiscountResponse{
					Success:                  true,
					IsValid:                  false,
					Reason:                   strPtr(fmt.Sprintf("Discount only applies to product ID %d", *discount.ProductId)),
					CalculatedDiscountAmount: "0.00",
				}, nil
			}
		}

		if discount.ProductGroupId != nil {
			if product.ProductGroupId == nil {
				return &proto.ValidateDiscountResponse{
					Success:                  true,
					IsValid:                  false,
					Reason:                   strPtr("Product does not belong to any group"),
					CalculatedDiscountAmount: "0.00",
				}, nil
			}

			if *product.ProductGroupId != *discount.ProductGroupId {
				return &proto.ValidateDiscountResponse{
					Success:                  true,
					IsValid:                  false,
					Reason:                   strPtr(fmt.Sprintf("Discount only applies to product group ID %d", *discount.ProductGroupId)),
					CalculatedDiscountAmount: "0.00",
				}, nil
			}
		}
	}

	quantity := int32(1)
	if req.Quantity != nil {
		quantity = req.GetQuantity()
	}

	if quantity <= 0 {
		return &proto.ValidateDiscountResponse{
			Success:                  true,
			IsValid:                  false,
			Reason:                   strPtr("Quantity must be greater than 0"),
			CalculatedDiscountAmount: "0.00",
		}, nil
	}

	if quantity < discount.MinQuantity {
		return &proto.ValidateDiscountResponse{
			Success: true,
			IsValid: false,
			Reason: strPtr(fmt.Sprintf(
				"Minimum quantity required: %d (current: %d)",
				discount.MinQuantity,
				quantity,
			)),
			CalculatedDiscountAmount: "0.00",
		}, nil
	}

	calculatedAmount := "0.00"

	if req.ProductId != nil {
		var product Product
		if err := s.db.Where("id = ?", req.GetProductId()).First(&product).Error; err == nil {
			unitPrice, _ := strconv.ParseFloat(product.ProductPrice, 64)
			quantityFloat := float64(quantity)
			discountValue, _ := strconv.ParseFloat(discount.DiscountValue, 64)

			var discountAmount float64

			switch discount.DiscountType {
			case 1: // DISCOUNT_TYPE_PERCENTAGE
				subtotal := unitPrice * quantityFloat
				discountAmount = subtotal * (discountValue / 100)

			case 2: // DISCOUNT_TYPE_FIXED_AMOUNT
				discountAmount = discountValue * quantityFloat

			case 3: // DISCOUNT_TYPE_BUY_X_GET_Y
				if quantity >= discount.MinQuantity {
					freeItems := int(quantityFloat/float64(discount.MinQuantity)) * int(discountValue)
					discountAmount = unitPrice * float64(freeItems)
				}

			default:
				discountAmount = 0
			}

			totalPrice := unitPrice * quantityFloat
			if discountAmount > totalPrice {
				discountAmount = totalPrice
			}

			calculatedAmount = strconv.FormatFloat(discountAmount, 'f', 2, 64)
		}
	}

	return &proto.ValidateDiscountResponse{
		Success:                  true,
		Message:                  strPtr("Discount is valid and can be applied"),
		IsValid:                  true,
		CalculatedDiscountAmount: calculatedAmount,
	}, nil
}

// -- Cart Related --
func (s *POSHandler) CreateCart(ctx context.Context, req *proto.CreateCartRequest) (*proto.CreateCartResponse, error) {
	if req.GetCashierId() == 0 {
		return &proto.CreateCartResponse{
			Success: false,
			Message: strPtr("cashier_id required"),
		}, nil
	}

	cart := Cart{
		CashierId:   req.GetCashierId(),
		Status:      0,
		CreatedAt:   time.Now(),
		TotalAmount: "0.00",
	}

	if err := s.db.Create(&cart).Error; err != nil {
		return &proto.CreateCartResponse{
			Success: false,
			Message: strPtr("Failed to create cart: " + err.Error()),
		}, err
	}

	return &proto.CreateCartResponse{
		Success: true,
		Message: strPtr("Cart created successfully"),
		Cart:    s.cartToProto(cart),
	}, nil
}

func (s *POSHandler) GetCart(ctx context.Context, req *proto.GetCartRequest) (*proto.GetCartResponse, error) {
	if req.GetCartId() == "" {
		return &proto.GetCartResponse{
			Success: false,
			Message: strPtr("cart_id required"),
		}, nil
	}

	cartId, err := strconv.ParseInt(req.GetCartId(), 10, 64)
	if err != nil {
		return &proto.GetCartResponse{
			Success: false,
			Message: strPtr("Invalid cart_id format"),
		}, nil
	}

	var cart Cart
	if err := s.db.Where("id = ?", cartId).
		Preload("CartItems.Product.ProductGroup").
		Preload("CartItems.Discount").
		First(&cart).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.GetCartResponse{
				Success: false,
				Message: strPtr("Cart not found"),
			}, nil
		}
		return &proto.GetCartResponse{
			Success: false,
			Message: strPtr("Database error"),
		}, err
	}

	return &proto.GetCartResponse{
		Success: true,
		Cart:    s.cartToProto(cart),
	}, nil
}

func (s *POSHandler) AddItemToCart(ctx context.Context, req *proto.AddItemToCartRequest) (*proto.AddItemToCartResponse, error) {
	if req.GetCartId() == "" {
		return &proto.AddItemToCartResponse{
			Success: false,
			Message: strPtr("cart_id required"),
		}, nil
	}

	if req.GetProductId() == 0 {
		return &proto.AddItemToCartResponse{
			Success: false,
			Message: strPtr("product_id required"),
		}, nil
	}

	if req.GetQuantity() <= 0 {
		return &proto.AddItemToCartResponse{
			Success: false,
			Message: strPtr("quantity must be greater than 0"),
		}, nil
	}

	cartId, err := strconv.ParseInt(req.GetCartId(), 10, 64)
	if err != nil {
		return &proto.AddItemToCartResponse{
			Success: false,
			Message: strPtr("Invalid cart_id format"),
		}, nil
	}

	var cart Cart
	if err := s.db.Where("id = ? AND status = ?", cartId, 0).First(&cart).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.AddItemToCartResponse{
				Success: false,
				Message: strPtr("Cart not found or inactive"),
			}, nil
		}
		return &proto.AddItemToCartResponse{
			Success: false,
			Message: strPtr("Database error"),
		}, err
	}

	var product Product
	if err := s.db.Where("id = ? AND is_active = ?", req.GetProductId(), true).
		Preload("ProductGroup").
		First(&product).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.AddItemToCartResponse{
				Success: false,
				Message: strPtr("Product not found or inactive"),
			}, nil
		}
		return &proto.AddItemToCartResponse{
			Success: false,
			Message: strPtr("Database error"),
		}, err
	}

	if product.RequiresServiceEmployee && req.ServingEmployeeId == nil {
		return &proto.AddItemToCartResponse{
			Success: false,
			Message: strPtr("This product requires a service employee"),
		}, nil
	}

	var existingItem CartItem
	err = s.db.Where("cart_id = ? AND product_id = ?", cartId, req.GetProductId()).
		First(&existingItem).Error

	if err == nil {
		existingItem.Quantity += req.GetQuantity()

		unitPrice, _ := strconv.ParseFloat(existingItem.UnitPrice, 64)
		lineTotal := unitPrice * float64(existingItem.Quantity)
		existingItem.LineTotal = strconv.FormatFloat(lineTotal, 'f', 2, 64)

		if err := s.db.Save(&existingItem).Error; err != nil {
			return &proto.AddItemToCartResponse{
				Success: false,
				Message: strPtr("Failed to update cart item: " + err.Error()),
			}, err
		}
	} else if err == gorm.ErrRecordNotFound {
		unitPrice, _ := strconv.ParseFloat(product.ProductPrice, 64)
		lineTotal := unitPrice * float64(req.GetQuantity())

		cartItem := CartItem{
			CartId:            cartId,
			ProductId:         req.GetProductId(),
			ServingEmployeeId: req.ServingEmployeeId,
			Quantity:          req.GetQuantity(),
			UnitPrice:         product.ProductPrice,
			DiscountAmount:    "0.00",
			LineTotal:         strconv.FormatFloat(lineTotal, 'f', 2, 64),
			CreatedAt:         time.Now(),
		}

		if err := s.db.Create(&cartItem).Error; err != nil {
			return &proto.AddItemToCartResponse{
				Success: false,
				Message: strPtr("Failed to add item to cart: " + err.Error()),
			}, err
		}
	} else {
		return &proto.AddItemToCartResponse{
			Success: false,
			Message: strPtr("Database error"),
		}, err
	}

	if err := s.recalculateCartTotals(ctx, cartId); err != nil {
		return &proto.AddItemToCartResponse{
			Success: false,
			Message: strPtr("Failed to recalculate totals: " + err.Error()),
		}, err
	}

	if err := s.db.Where("id = ?", cartId).
		Preload("CartItems.Product.ProductGroup").
		Preload("CartItems.Discount").
		First(&cart).Error; err != nil {
		return &proto.AddItemToCartResponse{
			Success: false,
			Message: strPtr("Failed to reload cart"),
		}, err
	}

	return &proto.AddItemToCartResponse{
		Success: true,
		Message: strPtr("Item added to cart successfully"),
		Cart:    s.cartToProto(cart),
	}, nil
}

func (s *POSHandler) RemoveItemFromCart(ctx context.Context, req *proto.RemoveItemFromCartRequest) (*proto.RemoveItemFromCartResponse, error) {
	if req.GetCartId() == "" {
		return &proto.RemoveItemFromCartResponse{
			Success: false,
			Message: strPtr("cart_id required"),
		}, nil
	}

	if req.GetItemId() == "" {
		return &proto.RemoveItemFromCartResponse{
			Success: false,
			Message: strPtr("item_id required"),
		}, nil
	}

	cartId, err := strconv.ParseInt(req.GetCartId(), 10, 64)
	if err != nil {
		return &proto.RemoveItemFromCartResponse{
			Success: false,
			Message: strPtr("Invalid cart_id format"),
		}, nil
	}

	itemId, err := strconv.ParseInt(req.GetItemId(), 10, 64)
	if err != nil {
		return &proto.RemoveItemFromCartResponse{
			Success: false,
			Message: strPtr("Invalid item_id format"),
		}, nil
	}

	var cart Cart
	if err := s.db.Where("id = ? AND status = ?", cartId, 0).First(&cart).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.RemoveItemFromCartResponse{
				Success: false,
				Message: strPtr("Cart not found or inactive"),
			}, nil
		}
		return &proto.RemoveItemFromCartResponse{
			Success: false,
			Message: strPtr("Database error"),
		}, err
	}

	result := s.db.Where("id = ? AND cart_id = ?", itemId, cartId).Delete(&CartItem{})
	if result.Error != nil {
		return &proto.RemoveItemFromCartResponse{
			Success: false,
			Message: strPtr("Failed to remove item: " + result.Error.Error()),
		}, result.Error
	}

	if result.RowsAffected == 0 {
		return &proto.RemoveItemFromCartResponse{
			Success: false,
			Message: strPtr("Cart item not found"),
		}, nil
	}

	if err := s.recalculateCartTotals(ctx, cartId); err != nil {
		return &proto.RemoveItemFromCartResponse{
			Success: false,
			Message: strPtr("Failed to recalculate totals: " + err.Error()),
		}, err
	}

	if err := s.db.Where("id = ?", cartId).
		Preload("CartItems.Product.ProductGroup").
		Preload("CartItems.Discount").
		First(&cart).Error; err != nil {
		return &proto.RemoveItemFromCartResponse{
			Success: false,
			Message: strPtr("Failed to reload cart"),
		}, err
	}

	return &proto.RemoveItemFromCartResponse{
		Success: true,
		Message: strPtr("Item removed from cart successfully"),
		Cart:    s.cartToProto(cart),
	}, nil
}

func (s *POSHandler) ApplyDiscount(ctx context.Context, req *proto.ApplyDiscountRequest) (*proto.ApplyDiscountResponse, error) {
	if req.GetCartId() == "" {
		return &proto.ApplyDiscountResponse{
			Success: false,
			Message: strPtr("cart_id required"),
		}, nil
	}

	if req.GetDiscountId() == 0 {
		return &proto.ApplyDiscountResponse{
			Success: false,
			Message: strPtr("discount_id required"),
		}, nil
	}

	cartId, err := strconv.ParseInt(req.GetCartId(), 10, 64)
	if err != nil {
		return &proto.ApplyDiscountResponse{
			Success: false,
			Message: strPtr("Invalid cart_id format"),
		}, nil
	}

	var cart Cart
	if err := s.db.Where("id = ? AND status = ?", cartId, 0).First(&cart).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.ApplyDiscountResponse{
				Success: false,
				Message: strPtr("Cart not found or inactive"),
			}, nil
		}
		return &proto.ApplyDiscountResponse{
			Success: false,
			Message: strPtr("Database error"),
		}, err
	}

	var discount Discount
	if err := s.db.Where("id = ? AND is_active = ?", req.GetDiscountId(), true).
		First(&discount).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.ApplyDiscountResponse{
				Success: false,
				Message: strPtr("Discount not found or inactive"),
			}, nil
		}
		return &proto.ApplyDiscountResponse{
			Success: false,
			Message: strPtr("Database error"),
		}, err
	}

	now := time.Now()
	if discount.ValidFrom != nil && now.Before(*discount.ValidFrom) {
		return &proto.ApplyDiscountResponse{
			Success: false,
			Message: strPtr("Discount is not yet valid"),
		}, nil
	}
	if discount.ValidUntil != nil && now.After(*discount.ValidUntil) {
		return &proto.ApplyDiscountResponse{
			Success: false,
			Message: strPtr("Discount has expired"),
		}, nil
	}

	var itemIds []int64
	if len(req.ItemIds) > 0 {
		for _, idStr := range req.ItemIds {
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				continue
			}
			itemIds = append(itemIds, id)
		}
	} else {
		var items []CartItem
		query := s.db.Where("cart_id = ?", cartId)

		if discount.ProductId != nil {
			query = query.Where("product_id = ?", *discount.ProductId)
		} else if discount.ProductGroupId != nil {
			query = query.Joins("JOIN products ON products.id = cart_items.product_id").
				Where("products.product_group_id = ?", *discount.ProductGroupId)
		}

		if err := query.Find(&items).Error; err != nil {
			return &proto.ApplyDiscountResponse{
				Success: false,
				Message: strPtr("Failed to find eligible items"),
			}, err
		}

		for _, item := range items {
			itemIds = append(itemIds, item.ID)
		}
	}

	if len(itemIds) == 0 {
		return &proto.ApplyDiscountResponse{
			Success: false,
			Message: strPtr("No eligible items found for this discount"),
		}, nil
	}

	for _, itemId := range itemIds {
		var item CartItem
		if err := s.db.Where("id = ? AND cart_id = ?", itemId, cartId).
			Preload("Product").
			First(&item).Error; err != nil {
			continue
		}

		if item.Quantity < discount.MinQuantity {
			continue
		}

		discountAmount := s.calculateDiscountAmount(discount, item)

		item.DiscountId = &discount.ID
		item.DiscountAmount = discountAmount

		unitPrice, _ := strconv.ParseFloat(item.UnitPrice, 64)
		discountAmt, _ := strconv.ParseFloat(discountAmount, 64)
		lineTotal := (unitPrice * float64(item.Quantity)) - discountAmt
		item.LineTotal = strconv.FormatFloat(lineTotal, 'f', 2, 64)

		s.db.Save(&item)
	}

	if err := s.recalculateCartTotals(ctx, cartId); err != nil {
		return &proto.ApplyDiscountResponse{
			Success: false,
			Message: strPtr("Failed to recalculate totals: " + err.Error()),
		}, err
	}

	if err := s.db.Where("id = ?", cartId).
		Preload("CartItems.Product.ProductGroup").
		Preload("CartItems.Discount").
		First(&cart).Error; err != nil {
		return &proto.ApplyDiscountResponse{
			Success: false,
			Message: strPtr("Failed to reload cart"),
		}, err
	}

	return &proto.ApplyDiscountResponse{
		Success: true,
		Message: strPtr("Discount applied successfully"),
		Cart:    s.cartToProto(cart),
	}, nil
}

func (s *POSHandler) calculateDiscountAmount(discount Discount, item CartItem) string {
	unitPrice, _ := strconv.ParseFloat(item.UnitPrice, 64)
	discountValue, _ := strconv.ParseFloat(discount.DiscountValue, 64)

	var discountAmount float64

	switch discount.DiscountType {
	case 1:
		discountAmount = (unitPrice * float64(item.Quantity)) * (discountValue / 100)
	case 2:
		discountAmount = discountValue * float64(item.Quantity)
	case 3:
		discountAmount = 0
	default:
		discountAmount = 0
	}

	return strconv.FormatFloat(discountAmount, 'f', 2, 64)
}

func (s *POSHandler) recalculateCartTotals(ctx context.Context, cartId int64) error {
	var items []CartItem
	if err := s.db.Where("cart_id = ?", cartId).Find(&items).Error; err != nil {
		return err
	}

	var subtotal, totalDiscount float64
	for _, item := range items {
		lineTotal, _ := strconv.ParseFloat(item.LineTotal, 64)
		discount, _ := strconv.ParseFloat(item.DiscountAmount, 64)

		subtotal += lineTotal + discount
		totalDiscount += discount
	}

	taxRate := 0.10
	taxAmount := (subtotal - totalDiscount) * taxRate
	totalAmount := subtotal - totalDiscount + taxAmount

	return s.db.Model(&Cart{}).Where("id = ?", cartId).Updates(map[string]interface{}{
		"subtotal":        strconv.FormatFloat(subtotal, 'f', 2, 64),
		"discount_amount": strconv.FormatFloat(totalDiscount, 'f', 2, 64),
		"tax_amount":      strconv.FormatFloat(taxAmount, 'f', 2, 64),
		"total_amount":    strconv.FormatFloat(totalAmount, 'f', 2, 64),
		"updated_at":      time.Now(),
	}).Error
}

// -- Orders Related --
func (s *POSHandler) CreateOrder(ctx context.Context, req *proto.CreateOrderRequest) (*proto.CreateOrderResponse, error) {
	if req.GetDocumentNumber() == "" {
		return &proto.CreateOrderResponse{
			Success: false,
			Message: strPtr("document_number required"),
		}, nil
	}

	if req.GetCashierId() == 0 {
		return &proto.CreateOrderResponse{
			Success: false,
			Message: strPtr("cashier_id required"),
		}, nil
	}

	if len(req.GetOrderItems()) == 0 {
		return &proto.CreateOrderResponse{
			Success: false,
			Message: strPtr("order must have at least one item"),
		}, nil
	}

	var existingOrder OrderDocument
	err := s.db.Where("document_number = ?", req.GetDocumentNumber()).First(&existingOrder).Error
	if err == nil {
		return &proto.CreateOrderResponse{
			Success: false,
			Message: strPtr("Document number already exists"),
		}, nil
	} else if err != gorm.ErrRecordNotFound {
		return &proto.CreateOrderResponse{
			Success: false,
			Message: strPtr("Database error"),
		}, err
	}

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	now := time.Now()
	var subtotal, totalDiscount, totalTax float64

	order := OrderDocument{
		DocumentNumber: req.GetDocumentNumber(),
		CashierId:      req.GetCashierId(),
		OrdersDate:     &now,
		DocumentType:   int32(req.GetDocumentType()),
		PaidAmount:     "0.00",
		ChangeAmount:   "0.00",
		PaidStatus:     int32(proto.PaidStatus_PAID_STATUS_PENDING),
		AdditionalInfo: req.AdditionalInfo,
		Notes:          req.Notes,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := tx.Create(&order).Error; err != nil {
		tx.Rollback()
		return &proto.CreateOrderResponse{
			Success: false,
			Message: strPtr("Failed to create order: " + err.Error()),
		}, err
	}

	for _, itemReq := range req.GetOrderItems() {
		var product Product
		if err := tx.Where("id = ? AND is_active = ?", itemReq.GetProductId(), true).
			Preload("ProductGroup").
			First(&product).Error; err != nil {
			tx.Rollback()
			if err == gorm.ErrRecordNotFound {
				return &proto.CreateOrderResponse{
					Success: false,
					Message: strPtr(fmt.Sprintf("Product %d not found or inactive", itemReq.GetProductId())),
				}, nil
			}
			return &proto.CreateOrderResponse{
				Success: false,
				Message: strPtr("Database error"),
			}, err
		}

		if product.RequiresServiceEmployee && itemReq.ServingEmployeeId == nil {
			tx.Rollback()
			return &proto.CreateOrderResponse{
				Success: false,
				Message: strPtr(fmt.Sprintf("Product '%s' requires a service employee", product.ProductName)),
			}, nil
		}

		unitPrice, _ := strconv.ParseFloat(product.ProductPrice, 64)
		quantity := float64(itemReq.GetQuantity())
		lineSubtotal := unitPrice * quantity

		var discountAmount float64
		var discountId *int32
		if itemReq.DiscountId != nil {
			var discount Discount
			if err := tx.Where("id = ? AND is_active = ?", *itemReq.DiscountId, true).
				First(&discount).Error; err == nil {

				if discount.ProductId != nil && *discount.ProductId != itemReq.GetProductId() {
					tx.Rollback()
					return &proto.CreateOrderResponse{
						Success: false,
						Message: strPtr(fmt.Sprintf("Discount %d does not apply to product %d", *itemReq.DiscountId, itemReq.GetProductId())),
					}, nil
				}

				if itemReq.GetQuantity() < discount.MinQuantity {
					tx.Rollback()
					return &proto.CreateOrderResponse{
						Success: false,
						Message: strPtr(fmt.Sprintf("Discount requires minimum quantity of %d", discount.MinQuantity)),
					}, nil
				}

				discountValue, _ := strconv.ParseFloat(discount.DiscountValue, 64)
				switch discount.DiscountType {
				case 1: // Percentage
					discountAmount = lineSubtotal * (discountValue / 100)
				case 2: // Fixed Amount
					discountAmount = discountValue * quantity
				case 3: // Buy X Get Y
					if itemReq.GetQuantity() >= discount.MinQuantity {
						freeItems := int(quantity/float64(discount.MinQuantity)) * int(discountValue)
						discountAmount = unitPrice * float64(freeItems)
					}
				}
				discountId = itemReq.DiscountId
			}
		}

		lineTotal := lineSubtotal - discountAmount

		commissionAmount := "0.00"
		if product.CommissionEligible && product.ProductGroup != nil {
			commissionRate, _ := strconv.ParseFloat(product.ProductGroup.CommissionRate, 64)
			commission := lineTotal * (commissionRate / 100)
			commissionAmount = strconv.FormatFloat(commission, 'f', 2, 64)
		}

		orderItem := OrderItem{
			DocumentId:          order.ID,
			ProductId:           itemReq.GetProductId(),
			ServingEmployeeId:   itemReq.ServingEmployeeId,
			Quantity:            itemReq.GetQuantity(),
			UnitPrice:           product.ProductPrice,
			PriceBeforeDiscount: strconv.FormatFloat(lineSubtotal, 'f', 2, 64),
			DiscountId:          discountId,
			DiscountAmount:      strconv.FormatFloat(discountAmount, 'f', 2, 64),
			LineTotal:           strconv.FormatFloat(lineTotal, 'f', 2, 64),
			CommissionAmount:    commissionAmount,
			CreatedAt:           now,
		}

		if err := tx.Create(&orderItem).Error; err != nil {
			tx.Rollback()
			return &proto.CreateOrderResponse{
				Success: false,
				Message: strPtr("Failed to create order item: " + err.Error()),
			}, err
		}

		subtotal += lineSubtotal
		totalDiscount += discountAmount
	}

	taxRate := 0.10
	totalTax = (subtotal - totalDiscount) * taxRate
	totalAmount := subtotal - totalDiscount + totalTax

	order.Subtotal = strconv.FormatFloat(subtotal, 'f', 2, 64)
	order.TaxAmount = strconv.FormatFloat(totalTax, 'f', 2, 64)
	order.DiscountAmount = strconv.FormatFloat(totalDiscount, 'f', 2, 64)
	order.TotalAmount = strconv.FormatFloat(totalAmount, 'f', 2, 64)

	if err := tx.Save(&order).Error; err != nil {
		tx.Rollback()
		return &proto.CreateOrderResponse{
			Success: false,
			Message: strPtr("Failed to update order totals: " + err.Error()),
		}, err
	}

	if err := tx.Commit().Error; err != nil {
		return &proto.CreateOrderResponse{
			Success: false,
			Message: strPtr("Failed to commit transaction: " + err.Error()),
		}, err
	}

	if err := s.db.Where("id = ?", order.ID).
		Preload("OrderItems.Product.ProductGroup").
		Preload("OrderItems.Discount").
		Preload("PaymentType").
		First(&order).Error; err != nil {
		return &proto.CreateOrderResponse{
			Success: false,
			Message: strPtr("Failed to reload order"),
		}, err
	}

	s.publishOrderEvent(ctx, OrderEvent{
		EventType:      EventOrderCreated,
		OrderID:        order.ID,
		DocumentNumber: order.DocumentNumber,
		CashierID:      order.CashierId,
		TotalAmount:    order.TotalAmount,
		PaidStatus:     order.PaidStatus,
		DocumentType:   order.DocumentType,
		Timestamp:      time.Now(),
		OrderData:      &order,
	})

	return &proto.CreateOrderResponse{
		Success:       true,
		Message:       strPtr("Order created successfully"),
		OrderDocument: s.orderDocumentToProto(order),
	}, nil
}

func (s *POSHandler) CreateOrderFromCart(ctx context.Context, req *proto.CreateOrderFromCartRequest) (*proto.CreateOrderFromCartResponse, error) {
	if req.GetCartId() == "" {
		return &proto.CreateOrderFromCartResponse{
			Success: false,
			Message: strPtr("cart_id required"),
		}, nil
	}

	if req.GetDocumentNumber() == "" {
		return &proto.CreateOrderFromCartResponse{
			Success: false,
			Message: strPtr("document_number required"),
		}, nil
	}

	cartId, err := strconv.ParseInt(req.GetCartId(), 10, 64)
	if err != nil {
		return &proto.CreateOrderFromCartResponse{
			Success: false,
			Message: strPtr("Invalid cart_id format"),
		}, nil
	}

	var existingOrder OrderDocument
	err = s.db.Where("document_number = ?", req.GetDocumentNumber()).First(&existingOrder).Error
	if err == nil {
		return &proto.CreateOrderFromCartResponse{
			Success: false,
			Message: strPtr("Document number already exists"),
		}, nil
	} else if err != gorm.ErrRecordNotFound {
		return &proto.CreateOrderFromCartResponse{
			Success: false,
			Message: strPtr("Database error"),
		}, err
	}

	var cart Cart
	if err := s.db.Where("id = ? AND status = ?", cartId, 0).
		Preload("CartItems.Product.ProductGroup").
		Preload("CartItems.Discount").
		First(&cart).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.CreateOrderFromCartResponse{
				Success: false,
				Message: strPtr("Cart not found or already processed"),
			}, nil
		}
		return &proto.CreateOrderFromCartResponse{
			Success: false,
			Message: strPtr("Database error"),
		}, err
	}

	if len(cart.CartItems) == 0 {
		return &proto.CreateOrderFromCartResponse{
			Success: false,
			Message: strPtr("Cart is empty"),
		}, nil
	}

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	now := time.Now()
	order := OrderDocument{
		DocumentNumber: req.GetDocumentNumber(),
		CashierId:      cart.CashierId,
		OrdersDate:     &now,
		DocumentType:   int32(proto.DocumentType_DOCUMENT_TYPE_SALE),
		Subtotal:       cart.Subtotal,
		TaxAmount:      cart.TaxAmount,
		DiscountAmount: cart.DiscountAmount,
		TotalAmount:    cart.TotalAmount,
		PaidAmount:     "0.00",
		ChangeAmount:   "0.00",
		PaidStatus:     int32(proto.PaidStatus_PAID_STATUS_PENDING),
		AdditionalInfo: req.AdditionalInfo,
		Notes:          req.Notes,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := tx.Create(&order).Error; err != nil {
		tx.Rollback()
		return &proto.CreateOrderFromCartResponse{
			Success: false,
			Message: strPtr("Failed to create order: " + err.Error()),
		}, err
	}

	for _, cartItem := range cart.CartItems {

		commissionAmount := "0.00"
		if cartItem.Product != nil && cartItem.Product.CommissionEligible && cartItem.Product.ProductGroup != nil {
			commissionRate, _ := strconv.ParseFloat(cartItem.Product.ProductGroup.CommissionRate, 64)
			lineTotal, _ := strconv.ParseFloat(cartItem.LineTotal, 64)
			commission := lineTotal * (commissionRate / 100)
			commissionAmount = strconv.FormatFloat(commission, 'f', 2, 64)
		}

		unitPrice, _ := strconv.ParseFloat(cartItem.UnitPrice, 64)
		priceBeforeDiscount := unitPrice * float64(cartItem.Quantity)

		orderItem := OrderItem{
			DocumentId:          order.ID,
			ProductId:           cartItem.ProductId,
			ServingEmployeeId:   cartItem.ServingEmployeeId,
			Quantity:            cartItem.Quantity,
			UnitPrice:           cartItem.UnitPrice,
			PriceBeforeDiscount: strconv.FormatFloat(priceBeforeDiscount, 'f', 2, 64),
			DiscountId:          cartItem.DiscountId,
			DiscountAmount:      cartItem.DiscountAmount,
			LineTotal:           cartItem.LineTotal,
			CommissionAmount:    commissionAmount,
			CreatedAt:           now,
		}

		if err := tx.Create(&orderItem).Error; err != nil {
			tx.Rollback()
			return &proto.CreateOrderFromCartResponse{
				Success: false,
				Message: strPtr("Failed to create order items: " + err.Error()),
			}, err
		}
	}

	if err := tx.Model(&Cart{}).Where("id = ?", cartId).Update("status", 1).Error; err != nil {
		tx.Rollback()
		return &proto.CreateOrderFromCartResponse{
			Success: false,
			Message: strPtr("Failed to update cart status: " + err.Error()),
		}, err
	}

	if err := tx.Commit().Error; err != nil {
		return &proto.CreateOrderFromCartResponse{
			Success: false,
			Message: strPtr("Failed to commit transaction: " + err.Error()),
		}, err
	}

	if err := s.db.Where("id = ?", order.ID).
		Preload("OrderItems.Product.ProductGroup").
		Preload("OrderItems.Discount").
		Preload("PaymentType").
		First(&order).Error; err != nil {
		return &proto.CreateOrderFromCartResponse{
			Success: false,
			Message: strPtr("Failed to reload order"),
		}, err
	}

	s.publishOrderEvent(ctx, OrderEvent{
		EventType:      EventOrderCreated,
		OrderID:        order.ID,
		DocumentNumber: order.DocumentNumber,
		CashierID:      order.CashierId,
		TotalAmount:    order.TotalAmount,
		PaidStatus:     order.PaidStatus,
		DocumentType:   order.DocumentType,
		Timestamp:      time.Now(),
		OrderData:      &order,
	})

	return &proto.CreateOrderFromCartResponse{
		Success:       true,
		Message:       strPtr("Order created successfully from cart"),
		OrderDocument: s.orderDocumentToProto(order),
	}, nil
}

func (s *POSHandler) GetOrder(ctx context.Context, req *proto.GetOrderRequest) (*proto.GetOrderResponse, error) {
	if req.GetId() == 0 {
		return &proto.GetOrderResponse{
			Success: false,
			Message: strPtr("order id required"),
		}, nil
	}

	var order OrderDocument
	if err := s.db.Where("id = ?", req.GetId()).
		Preload("OrderItems.Product.ProductGroup").
		Preload("OrderItems.Discount").
		Preload("PaymentType").
		First(&order).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.GetOrderResponse{
				Success: false,
				Message: strPtr("Order not found"),
			}, nil
		}
		return &proto.GetOrderResponse{
			Success: false,
			Message: strPtr("Database error"),
		}, err
	}

	return &proto.GetOrderResponse{
		Success:       true,
		OrderDocument: s.orderDocumentToProto(order),
	}, nil
}

func (s *POSHandler) ListOrders(ctx context.Context, req *proto.ListOrdersRequest) (*proto.ListOrdersResponse, error) {
	var orders []OrderDocument
	var total int64

	query := s.db.Model(&OrderDocument{}).
		Preload("OrderItems.Product.ProductGroup").
		Preload("OrderItems.Discount").
		Preload("PaymentType")

	if req.CashierId != nil {
		query = query.Where("cashier_id = ?", req.GetCashierId())
	}

	if req.DocumentType != nil {
		query = query.Where("document_type = ?", req.GetDocumentType())
	}

	if req.PaidStatus != nil {
		query = query.Where("paid_status = ?", req.GetPaidStatus())
	}

	if req.DateRange != nil {
		if req.DateRange.StartDate != "" {
			startDate, err := time.Parse("2006-01-02", req.DateRange.StartDate)
			if err == nil {
				query = query.Where("orders_date >= ?", startDate)
			}
		}
		if req.DateRange.EndDate != "" {
			endDate, err := time.Parse("2006-01-02", req.DateRange.EndDate)
			if err == nil {
				endDate = endDate.AddDate(0, 0, 1)
				query = query.Where("orders_date < ?", endDate)
			}
		}
	}

	if err := query.Count(&total).Error; err != nil {
		return &proto.ListOrdersResponse{
			Success: false,
			Message: strPtr("Database error counting orders"),
		}, err
	}

	pageSize := int(req.GetPagination().GetPageSize())
	if pageSize <= 0 {
		pageSize = 20
	}

	pageNumber := 1
	if token := req.GetPagination().GetPageToken(); token != "" {
		if n, err := strconv.Atoi(token); err == nil && n > 0 {
			pageNumber = n
		}
	}

	offset := (pageNumber - 1) * pageSize

	if err := query.Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&orders).Error; err != nil {
		return &proto.ListOrdersResponse{
			Success: false,
			Message: strPtr("Database error fetching orders"),
		}, err
	}

	protoOrders := make([]*proto.OrderDocument, len(orders))
	for i, order := range orders {
		protoOrders[i] = s.orderDocumentToProto(order)
	}

	nextPageToken := ""
	if int64(pageNumber*pageSize) < total {
		nextPageToken = strconv.Itoa(pageNumber + 1)
	}

	return &proto.ListOrdersResponse{
		Success:        true,
		OrderDocuments: protoOrders,
		Pagination: &proto.PaginationResponse{
			NextPageToken: nextPageToken,
			TotalCount:    int32(total),
		},
	}, nil
}

func (s *POSHandler) VoidOrder(ctx context.Context, req *proto.VoidOrderRequest) (*proto.VoidOrderResponse, error) {
	if req.GetId() == 0 {
		return &proto.VoidOrderResponse{
			Success: false,
			Message: strPtr("order id required"),
		}, nil
	}

	if req.GetVoidedBy() == 0 {
		return &proto.VoidOrderResponse{
			Success: false,
			Message: strPtr("voided_by (cashier_id) required"),
		}, nil
	}

	if req.GetReason() == "" {
		return &proto.VoidOrderResponse{
			Success: false,
			Message: strPtr("void reason required"),
		}, nil
	}

	var order OrderDocument
	if err := s.db.Where("id = ?", req.GetId()).
		Preload("OrderItems").
		First(&order).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.VoidOrderResponse{
				Success: false,
				Message: strPtr("Order not found"),
			}, nil
		}
		return &proto.VoidOrderResponse{
			Success: false,
			Message: strPtr("Database error"),
		}, err
	}

	if order.DocumentType == int32(proto.DocumentType_DOCUMENT_TYPE_VOID) {
		return &proto.VoidOrderResponse{
			Success: false,
			Message: strPtr("Order is already voided"),
		}, nil
	}

	if order.PaidStatus == int32(proto.PaidStatus_PAID_STATUS_PAID) {
		return &proto.VoidOrderResponse{
			Success: false,
			Message: strPtr("Cannot void a paid order. Use return instead."),
		}, nil
	}

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	now := time.Now()
	updates := map[string]interface{}{
		"document_type": int32(proto.DocumentType_DOCUMENT_TYPE_VOID),
		"notes":         req.GetReason(),
		"updated_at":    now,
	}

	if err := tx.Model(&OrderDocument{}).Where("id = ?", req.GetId()).Updates(updates).Error; err != nil {
		tx.Rollback()
		return &proto.VoidOrderResponse{
			Success: false,
			Message: strPtr("Failed to void order: " + err.Error()),
		}, err
	}

	if err := tx.Commit().Error; err != nil {
		return &proto.VoidOrderResponse{
			Success: false,
			Message: strPtr("Failed to commit transaction: " + err.Error()),
		}, err
	}

	if err := s.db.Where("id = ?", req.GetId()).
		Preload("OrderItems.Product.ProductGroup").
		Preload("OrderItems.Discount").
		Preload("PaymentType").
		First(&order).Error; err != nil {
		return &proto.VoidOrderResponse{
			Success: false,
			Message: strPtr("Failed to reload order"),
		}, err
	}

	s.publishOrderEvent(ctx, OrderEvent{
		EventType:      EventOrderVoided,
		OrderID:        order.ID,
		DocumentNumber: order.DocumentNumber,
		CashierID:      req.GetVoidedBy(),
		TotalAmount:    order.TotalAmount,
		PaidStatus:     order.PaidStatus,
		DocumentType:   order.DocumentType,
		Timestamp:      time.Now(),
		OrderData:      &order,
	})

	return &proto.VoidOrderResponse{
		Success:       true,
		Message:       strPtr("Order voided successfully"),
		OrderDocument: s.orderDocumentToProto(order),
	}, nil
}

func (s *POSHandler) ReturnOrder(ctx context.Context, req *proto.ReturnOrderRequest) (*proto.ReturnOrderResponse, error) {
	if req.GetOriginalOrderId() == 0 {
		return &proto.ReturnOrderResponse{
			Success: false,
			Message: strPtr("original_order_id required"),
		}, nil
	}
	if req.GetProcessedBy() == 0 {
		return &proto.ReturnOrderResponse{
			Success: false,
			Message: strPtr("processed_by (cashier_id) required"),
		}, nil
	}
	if len(req.GetItemIds()) == 0 {
		return &proto.ReturnOrderResponse{
			Success: false,
			Message: strPtr("at least one item_id required for return"),
		}, nil
	}

	var originalOrder OrderDocument
	if err := s.db.Where("id = ?", req.GetOriginalOrderId()).
		Preload("OrderItems.Product.ProductGroup").
		Preload("OrderItems.Discount").
		First(&originalOrder).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.ReturnOrderResponse{
				Success: false,
				Message: strPtr("Original order not found"),
			}, nil
		}
		return &proto.ReturnOrderResponse{
			Success: false,
			Message: strPtr("Database error"),
		}, err
	}

	if originalOrder.PaidStatus != int32(proto.PaidStatus_PAID_STATUS_PAID) {
		return &proto.ReturnOrderResponse{
			Success: false,
			Message: strPtr("Can only return paid orders"),
		}, nil
	}

	if originalOrder.DocumentType == int32(proto.DocumentType_DOCUMENT_TYPE_VOID) {
		return &proto.ReturnOrderResponse{
			Success: false,
			Message: strPtr("Cannot return a voided order"),
		}, nil
	}

	var itemsToReturn []OrderItem
	if err := s.db.Where("id IN ? AND document_id = ?", req.GetItemIds(), req.GetOriginalOrderId()).
		Preload("Product.ProductGroup").
		Preload("Discount").
		Find(&itemsToReturn).Error; err != nil {
		return &proto.ReturnOrderResponse{
			Success: false,
			Message: strPtr("Failed to fetch items: " + err.Error()),
		}, err
	}

	if len(itemsToReturn) == 0 {
		return &proto.ReturnOrderResponse{
			Success: false,
			Message: strPtr("No valid items found for return"),
		}, nil
	}

	if len(itemsToReturn) != len(req.GetItemIds()) {
		return &proto.ReturnOrderResponse{
			Success: false,
			Message: strPtr("Some item IDs are invalid or don't belong to this order"),
		}, nil
	}

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var returnSubtotal, returnDiscount, returnTax float64
	for _, item := range itemsToReturn {
		priceBeforeDiscount, _ := strconv.ParseFloat(item.PriceBeforeDiscount, 64)
		discountAmount, _ := strconv.ParseFloat(item.DiscountAmount, 64)

		returnSubtotal += priceBeforeDiscount
		returnDiscount += discountAmount
	}

	taxRate := 0.10
	returnTax = (returnSubtotal - returnDiscount) * taxRate
	returnTotal := returnSubtotal - returnDiscount + returnTax

	now := time.Now()
	returnDoc := OrderDocument{
		DocumentNumber: fmt.Sprintf("RET-%s", originalOrder.DocumentNumber),
		CashierId:      req.GetProcessedBy(),
		OrdersDate:     &now,
		DocumentType:   int32(proto.DocumentType_DOCUMENT_TYPE_RETURN),
		Subtotal:       strconv.FormatFloat(returnSubtotal, 'f', 2, 64),
		TaxAmount:      strconv.FormatFloat(returnTax, 'f', 2, 64),
		DiscountAmount: strconv.FormatFloat(returnDiscount, 'f', 2, 64),
		TotalAmount:    strconv.FormatFloat(returnTotal, 'f', 2, 64),
		PaidAmount:     strconv.FormatFloat(returnTotal, 'f', 2, 64),
		ChangeAmount:   "0.00",
		PaidStatus:     int32(proto.PaidStatus_PAID_STATUS_REFUNDED),
		Notes:          req.Reason,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := tx.Create(&returnDoc).Error; err != nil {
		tx.Rollback()
		return &proto.ReturnOrderResponse{
			Success: false,
			Message: strPtr("Failed to create return document: " + err.Error()),
		}, err
	}

	for _, item := range itemsToReturn {
		returnItem := OrderItem{
			DocumentId:          returnDoc.ID,
			ProductId:           item.ProductId,
			ServingEmployeeId:   item.ServingEmployeeId,
			Quantity:            -item.Quantity,
			UnitPrice:           item.UnitPrice,
			PriceBeforeDiscount: item.PriceBeforeDiscount,
			DiscountId:          item.DiscountId,
			DiscountAmount:      item.DiscountAmount,
			LineTotal:           item.LineTotal,
			CommissionAmount:    item.CommissionAmount,
			CreatedAt:           now,
		}

		if err := tx.Create(&returnItem).Error; err != nil {
			tx.Rollback()
			return &proto.ReturnOrderResponse{
				Success: false,
				Message: strPtr("Failed to create return items: " + err.Error()),
			}, err
		}
	}

	if len(itemsToReturn) == len(originalOrder.OrderItems) {
		if err := tx.Model(&OrderDocument{}).
			Where("id = ?", req.GetOriginalOrderId()).
			Update("paid_status", int32(proto.PaidStatus_PAID_STATUS_REFUNDED)).
			Error; err != nil {
			tx.Rollback()
			return &proto.ReturnOrderResponse{
				Success: false,
				Message: strPtr("Failed to update original order: " + err.Error()),
			}, err
		}
	}

	if err := tx.Commit().Error; err != nil {
		return &proto.ReturnOrderResponse{
			Success: false,
			Message: strPtr("Failed to commit transaction: " + err.Error()),
		}, err
	}

	if err := s.db.Where("id = ?", returnDoc.ID).
		Preload("OrderItems.Product.ProductGroup").
		Preload("OrderItems.Discount").
		Preload("PaymentType").
		First(&returnDoc).Error; err != nil {
		return &proto.ReturnOrderResponse{
			Success: false,
			Message: strPtr("Failed to reload return document"),
		}, err
	}

	s.publishOrderEvent(ctx, OrderEvent{
		EventType:      EventOrderReturned,
		OrderID:        returnDoc.ID,
		DocumentNumber: returnDoc.DocumentNumber,
		CashierID:      req.GetProcessedBy(),
		TotalAmount:    returnDoc.TotalAmount,
		PaidStatus:     returnDoc.PaidStatus,
		DocumentType:   returnDoc.DocumentType,
		Timestamp:      time.Now(),
		OrderData:      &returnDoc,
	})

	return &proto.ReturnOrderResponse{
		Success:        true,
		Message:        strPtr("Return processed successfully"),
		ReturnDocument: s.orderDocumentToProto(returnDoc),
	}, nil
}

// -- Pub/Sub Related --
type OrderEvent struct {
	EventType      string         `json:"event_type"`
	OrderID        int64          `json:"order_id"`
	DocumentNumber string         `json:"document_number"`
	CashierID      int64          `json:"cashier_id"`
	TotalAmount    string         `json:"total_amount"`
	PaidStatus     int32          `json:"paid_status"`
	DocumentType   int32          `json:"document_type"`
	Timestamp      time.Time      `json:"timestamp"`
	OrderData      *OrderDocument `json:"order_data,omitempty"`
}

func (s *POSHandler) publishOrderEvent(ctx context.Context, event OrderEvent) error {
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	channel := fmt.Sprintf("pos:events:%s", event.EventType)
	if err := s.redis.Publish(ctx, channel, eventJSON).Err(); err != nil {
		return fmt.Errorf("failed to publish event: %w", err)
	}

	if err := s.redis.Publish(ctx, "pos:events:all", eventJSON).Err(); err != nil {
		return fmt.Errorf("failed to publish to all channel: %w", err)
	}

	return nil
}
