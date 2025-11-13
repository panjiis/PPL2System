package handler

import (
	"context"
	"errors"
	"log"
	"strconv"
	lib "syntra-system/internal/utils"
	proto "syntra-system/proto/protogen/pos"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (s *POSHandler) CreatePaymentTypes(ctx context.Context, req *proto.PaymentType) (*proto.CreatePaymentTypesResponse, error) {
	var paymentType PaymentType

	if req.PaymentName == "" {
		return &proto.CreatePaymentTypesResponse{
			Success: false,
			Message: lib.StrPtr("payment name is required"),
		}, nil
	}

	paymentType = PaymentType{
		PaymentName:       req.GetPaymentName(),
		ProcessingFeeRate: req.GetProcessingFeeRate(),
		IsActive:          req.GetIsActive(),
	}

	if err := s.db.Create(&paymentType).Error; err != nil {
		return &proto.CreatePaymentTypesResponse{
			Success: false,
			Message: lib.StrPtr("database error"),
		}, err
	}

	return &proto.CreatePaymentTypesResponse{
		Success: true,
		PaymentTypes: &proto.PaymentType{
			Id:                paymentType.ID,
			PaymentName:       paymentType.PaymentName,
			ProcessingFeeRate: paymentType.ProcessingFeeRate,
			IsActive:          paymentType.IsActive,
		},
	}, nil
}

func (s *POSHandler) UpdatePaymentTypes(ctx context.Context, req *proto.UpdatePaymentTypeRequest) (*proto.CreatePaymentTypesResponse, error) {
	var paymentType PaymentType

	if req.Id == 0 {
		return &proto.CreatePaymentTypesResponse{
			Success: false,
			Message: lib.StrPtr("payment type id is required"),
		}, nil
	}

	if err := s.db.WithContext(ctx).First(&paymentType, req.Id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &proto.CreatePaymentTypesResponse{
				Success: false,
				Message: lib.StrPtr("payment type not found"),
			}, nil
		}
		return nil, err
	}

	if req.PaymentName != lib.StrPtr("") {
		paymentType.PaymentName = req.GetPaymentName()
	}

	if *req.ProcessingFeeRate != "" {
		paymentType.ProcessingFeeRate = req.GetProcessingFeeRate()
	}

	if req.IsActive != nil {
		paymentType.IsActive = *req.IsActive
	}

	if err := s.db.WithContext(ctx).Save(&paymentType).Error; err != nil {
		return nil, err
	}

	return &proto.CreatePaymentTypesResponse{
		Success: true,
		Message: lib.StrPtr("payment type updated successfully"),
		PaymentTypes: &proto.PaymentType{
			Id:                paymentType.ID,
			PaymentName:       paymentType.PaymentName,
			ProcessingFeeRate: paymentType.ProcessingFeeRate,
			IsActive:          paymentType.IsActive,
		},
	}, nil
}

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
				Message: lib.StrPtr("Payment Type not found"),
			}, err
		}
		return &proto.ListPaymentTypesResponse{
			Success: false,
			Message: lib.StrPtr("database error"),
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
			Message: lib.StrPtr("order_id required"),
		}, nil
	}

	if err := s.db.Where("id = ?", req.GetOrderId()).First(&order).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.ProcessPaymentResponse{
				Success: false,
				Message: lib.StrPtr("Order Not Found"),
			}, nil
		}
		return &proto.ProcessPaymentResponse{
			Success: false,
			Message: lib.StrPtr("database error"),
		}, err
	}

	if order.PaidStatus == 1 {
		return &proto.ProcessPaymentResponse{
			Success: false,
			Message: lib.StrPtr("Order already paid"),
		}, nil
	}

	if req.GetPaymentTypeId() == 1 {
		paidAmount, err := strconv.ParseFloat(req.GetPaidAmount(), 64)
		if err != nil {
			return &proto.ProcessPaymentResponse{
				Success: false,
				Message: lib.StrPtr("Invalid paid amount format"),
			}, nil
		}

		totalAmount, err := strconv.ParseFloat(order.TotalAmount, 64)
		if err != nil {
			return &proto.ProcessPaymentResponse{
				Success: false,
				Message: lib.StrPtr("Invalid total amount"),
			}, err
		}

		if paidAmount < totalAmount {
			return &proto.ProcessPaymentResponse{
				Success: false,
				Message: lib.StrPtr("Insufficient payment amount"),
			}, nil
		}

		paymentChange := paidAmount - totalAmount
		changeAmount = strconv.FormatFloat(paymentChange, 'f', 2, 64)
	}

	order.PaidStatus = 1
	order.PaymentTypeId = lib.Int32Ptr(req.PaymentTypeId)

	if err := s.db.Save(&order).Error; err != nil {
		return &proto.ProcessPaymentResponse{
			Success: false,
			Message: lib.StrPtr("Failed to update order: " + err.Error()),
		}, err
	}

	if err := s.db.Where("id = ?", order.ID).
		Preload("OrderItems.Product.ProductGroup").
		Preload("OrderItems.Discount").
		Preload("PaymentType").
		First(&order).Error; err != nil {
		return &proto.ProcessPaymentResponse{
			Success: false,
			Message: lib.StrPtr("Failed to reload order"),
		}, err
	}

	// === TAMBAHKAN LOG INI ===
	log.Println("DEBUG POS: Menerbitkan event 'payment.processed'...")
	// ==========================

	// --- BUAT PAYLOAD EVENT YANG RATA (FLATTENED) ---
	// Kita bisa melakukan ini karena Anda sudah me-reload order dengan Preload
	items := make([]OrderItemEvent, len(order.OrderItems))
	for i, item := range order.OrderItems {
		productName := ""
		costPrice := "0.00"
		if item.Product != nil {
			productName = item.Product.ProductName
			costPrice = item.Product.CostPrice
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
			CostPrice:           costPrice,
		}
	}

	orderDataPayload := &OrderDataEvent{
		ID:             order.ID,
		DocumentNumber: order.DocumentNumber,
		CashierId:      order.CashierId,
		OrdersDate:     order.OrdersDate,
		TaxAmount:      order.TaxAmount,
		TotalAmount:    order.TotalAmount,
		OrderItems:     items,
	}
	// --- AKHIR BLOK PAYLOAD ---

	s.publishOrderEvent(ctx, OrderEvent{
		EventType:      EventPaymentProcessed,
		OrderID:        order.ID,
		DocumentNumber: order.DocumentNumber,
		CashierID:      order.CashierId,
		TotalAmount:    order.TotalAmount,
		PaidStatus:     order.PaidStatus,
		DocumentType:   order.DocumentType,
		Timestamp:      time.Now(),
		OrderData:      orderDataPayload,
	})

	saleItems := s.orderItemsToSaleItems(order.OrderItems)
	saleCompletedEvent := SaleCompletedEvent{
		EventID:       uuid.New().String(),
		Timestamp:     time.Now(),
		TransactionID: order.DocumentNumber,
		DocumentID:    order.ID,
		WarehouseID:   1,
		Items:         saleItems,
	}

	err := s.publishStockChanges(ctx, "sale.completed", saleCompletedEvent)
	if err != nil {
		log.Printf("Failed to publish sale.completed event: %v", err)
	}

	return &proto.ProcessPaymentResponse{
		Success:       true,
		Message:       lib.StrPtr("Payment processed successfully"),
		OrderDocument: s.orderDocumentToProto(order),
		ChangeAmount:  changeAmount,
	}, nil
}
