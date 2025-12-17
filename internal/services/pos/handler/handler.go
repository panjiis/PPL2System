package handler

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	posFunc "syntra-system/internal/services/pos"
	lib "syntra-system/internal/utils"
	proto "syntra-system/proto/protogen/pos"

	"github.com/go-redis/redis/v8"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
	"sync/atomic"
)

type POSHandler struct {
	proto.UnimplementedPOSServiceServer
	db          *gorm.DB
	redis       *redis.Client
	nats        *nats.Conn
	cancelFunc  context.CancelFunc
	schedulerWg sync.WaitGroup
	schedulerMu sync.Mutex
	parentCtx   context.Context
	shuttingDown atomic.Bool
}

func NewPOSHandler(db *gorm.DB, redisClient *redis.Client, parentCtx context.Context) *POSHandler {
	nc, err := nats.Connect("nats://10.147.17.76:4222",
		nats.ConnectHandler(func(nc *nats.Conn) {
			log.Printf("POS service connected to NATS at %v", nc.ConnectedUrl())
		}),
		nats.DisconnectHandler(func(nc *nats.Conn) {
			log.Printf("POS service disconnected from NATS")
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Printf("POS service reconnected to NATS at %v", nc.ConnectedUrl())
		}),
	)
	if err != nil {
		log.Fatal("Failed to connect to NATS:", err)
	}

	return &POSHandler{
		db:    db,
		redis: redisClient,
		nats:  nc,
		parentCtx: parentCtx,
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
		OrdersDate:     timestamppb.New(lib.TimeNowOrZero(doc.OrdersDate)),
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
		discount = s.discountToProto(item.Discount)
	}

	return &proto.OrderItem{
		Id:                  item.ID,
		DocumentId:          item.DocumentId,
		ProductCode:         item.ProductCode,
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

func (s *POSHandler) discountToProto(discount *Discount) *proto.Discount {
	pb := &proto.Discount{
		Id:            int64(discount.Id),
		DiscountName:  discount.DiscountName,
		DiscountType:  s.stringToDiscountType(discount.DiscountType),
		DiscountValue: discount.DiscountValue,
		MinQuantity:   discount.MinQuantity,
		IsActive:      discount.IsActive,
		CreatedAt:     timestamppb.New(discount.CreatedAt),
		UpdatedAt:     timestamppb.New(discount.UpdatedAt),
	}

	discountTypeLabel := posFunc.InterpretDiscountTypeValue(pb.DiscountType)
	pb.DiscountTypeLabel = &discountTypeLabel

	if discount.ProductCode != nil {
		pb.ProductCode = discount.ProductCode
	}
	if discount.ProductGroupId != nil {
		pb.ProductGroupId = discount.ProductGroupId
	}
	if discount.MaxUsagePerTransaction != nil {
		pb.MaxUsagePerTransaction = discount.MaxUsagePerTransaction
	}
	if discount.ValidFrom != nil {
		pb.ValidFrom = timestamppb.New(*discount.ValidFrom)
	}
	if discount.ValidUntil != nil {
		pb.ValidUntil = timestamppb.New(*discount.ValidUntil)
	}
	if discount.BuyQuantity != nil {
		pb.BuyQuantity = discount.BuyQuantity
	}
	if discount.GetQuantity != nil {
		pb.GetQuantity = discount.GetQuantity
	}
	return pb
}

func (s *POSHandler) productToProto(p Product) *proto.Product {
	var productGroup *proto.ProductGroup
	if p.ProductGroup != nil {
		productGroup = s.productGroupToProto(*p.ProductGroup)
	}

	return &proto.Product{
		ProductCode:             p.ProductCode,
		ProductName:             p.ProductName,
		ProductPrice:            p.ProductPrice,
		CostPrice:               p.CostPrice,
		ImageUrl:                p.ImageUrl,
		Color:                   p.Color,
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
	for _, p := range pg.Products {
		products = append(products, &proto.Product{
			ProductCode:    p.ProductCode,
			ProductName:    p.ProductName,
			ProductGroupId: p.ProductGroupId,
			ProductPrice:   p.ProductPrice,
			IsActive:       p.IsActive,
			CreatedAt:      timestamppb.New(p.CreatedAt),
			UpdatedAt:      timestamppb.New(p.UpdatedAt),
		})
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
		discount = s.discountToProto(item.Discount)
	}

	return &proto.CartItem{
		ItemId:            strconv.FormatInt(item.ID, 10),
		ProductCode:       item.ProductCode,
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
