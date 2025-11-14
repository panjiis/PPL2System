package handler

import (
	"context"
	"fmt"
	"log"
	"strconv"
	lib "syntra-system/internal/utils"
	proto "syntra-system/proto/protogen/pos"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (s *POSHandler) CreateOrder(ctx context.Context, req *proto.CreateOrderRequest) (*proto.CreateOrderResponse, error) {
	if req.GetDocumentNumber() == "" {
		return &proto.CreateOrderResponse{
			Success: false,
			Message: lib.StrPtr("document_number required"),
		}, nil
	}

	if req.GetCashierId() == 0 {
		return &proto.CreateOrderResponse{
			Success: false,
			Message: lib.StrPtr("cashier_id required"),
		}, nil
	}

	if len(req.GetOrderItems()) == 0 {
		return &proto.CreateOrderResponse{
			Success: false,
			Message: lib.StrPtr("order must have at least one item"),
		}, nil
	}

	var existingOrder OrderDocument
	err := s.db.Where("document_number = ?", req.GetDocumentNumber()).First(&existingOrder).Error
	if err == nil {
		return &proto.CreateOrderResponse{
			Success: false,
			Message: lib.StrPtr("Document number already exists"),
		}, nil
	} else if err != gorm.ErrRecordNotFound {
		return &proto.CreateOrderResponse{
			Success: false,
			Message: lib.StrPtr("Database error"),
		}, err
	}

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	loc, _ := time.LoadLocation("Asia/Jakarta")
	now := time.Now().In(loc)
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
			Message: lib.StrPtr("Failed to create order: " + err.Error()),
		}, err
	}

	for _, itemReq := range req.GetOrderItems() {
		var product Product
		if err := tx.Where("id = ? AND is_active = ?", itemReq.GetProductCode(), true).
			Preload("ProductGroup").
			First(&product).Error; err != nil {
			tx.Rollback()
			if err == gorm.ErrRecordNotFound {
				return &proto.CreateOrderResponse{
					Success: false,
					Message: lib.StrPtr(fmt.Sprintf("Product %s not found or inactive", itemReq.GetProductCode())),
				}, nil
			}
			return &proto.CreateOrderResponse{
				Success: false,
				Message: lib.StrPtr("Database error"),
			}, err
		}

		if product.RequiresServiceEmployee && itemReq.ServingEmployeeId == nil {
			tx.Rollback()
			return &proto.CreateOrderResponse{
				Success: false,
				Message: lib.StrPtr(fmt.Sprintf("Product '%s' requires a service employee", product.ProductName)),
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

				if discount.ProductCode != nil && *discount.ProductCode != itemReq.GetProductCode() {
					tx.Rollback()
					return &proto.CreateOrderResponse{
						Success: false,
						Message: lib.StrPtr(fmt.Sprintf("Discount %d does not apply to product %s", *itemReq.DiscountId, itemReq.GetProductCode())),
					}, nil
				}

				if itemReq.GetQuantity() < discount.MinQuantity {
					tx.Rollback()
					return &proto.CreateOrderResponse{
						Success: false,
						Message: lib.StrPtr(fmt.Sprintf("Discount requires minimum quantity of %d", discount.MinQuantity)),
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
			ProductCode:         itemReq.GetProductCode(),
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
				Message: lib.StrPtr("Failed to create order item: " + err.Error()),
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
			Message: lib.StrPtr("Failed to update order totals: " + err.Error()),
		}, err
	}

	if err := tx.Commit().Error; err != nil {
		return &proto.CreateOrderResponse{
			Success: false,
			Message: lib.StrPtr("Failed to commit transaction: " + err.Error()),
		}, err
	}

	// --- TAMBAHKAN BLOK INI ---
	// Muat ulang order dengan OrderItems yang sudah terisi
	// Kita butuh preloads ini untuk createOrderEvent dan orderDocumentToProto
	var reloadedOrder OrderDocument
	if err := s.db.Where("id = ?", order.ID).
		Preload("OrderItems.Product.ProductGroup").
		Preload("OrderItems.Discount").
		Preload("PaymentType").
		First(&reloadedOrder).Error; err != nil {

		log.Printf("Gagal memuat ulang order %d untuk event: %v", order.ID, err)
		// Jika gagal, kirim data seadanya (event & proto akan tidak lengkap)
		reloadedOrder = order
	}
	// --- AKHIR BLOK TAMBAHAN ---

	// Gunakan 'reloadedOrder' yang sudah lengkap, BUKAN 'order'
	event := s.createOrderEvent(&reloadedOrder, EventOrderCreated)
	s.publishOrderEvent(ctx, event)

	return &proto.CreateOrderResponse{
		Success:       true,
		Message:       lib.StrPtr("Order created successfully"),
		OrderDocument: s.orderDocumentToProto(reloadedOrder),
	}, nil
}

func (s *POSHandler) CreateOrderFromCart(ctx context.Context, req *proto.CreateOrderFromCartRequest) (*proto.CreateOrderFromCartResponse, error) {
	if req.GetCartId() == "" {
		return &proto.CreateOrderFromCartResponse{
			Success: false,
			Message: lib.StrPtr("cart_id required"),
		}, nil
	}

	if req.GetDocumentNumber() == "" {
		return &proto.CreateOrderFromCartResponse{
			Success: false,
			Message: lib.StrPtr("document_number required"),
		}, nil
	}

	cartId, err := strconv.ParseInt(req.GetCartId(), 10, 64)
	if err != nil {
		return &proto.CreateOrderFromCartResponse{
			Success: false,
			Message: lib.StrPtr("Invalid cart_id format"),
		}, nil
	}

	var existingOrder OrderDocument
	err = s.db.Where("document_number = ?", req.GetDocumentNumber()).First(&existingOrder).Error
	if err == nil {
		return &proto.CreateOrderFromCartResponse{
			Success: false,
			Message: lib.StrPtr("Document number already exists"),
		}, nil
	} else if err != gorm.ErrRecordNotFound {
		return &proto.CreateOrderFromCartResponse{
			Success: false,
			Message: lib.StrPtr("Database error"),
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
				Message: lib.StrPtr("Cart not found or already processed"),
			}, nil
		}
		return &proto.CreateOrderFromCartResponse{
			Success: false,
			Message: lib.StrPtr("Database error"),
		}, err
	}

	if len(cart.CartItems) == 0 {
		return &proto.CreateOrderFromCartResponse{
			Success: false,
			Message: lib.StrPtr("Cart is empty"),
		}, nil
	}

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	loc, _ := time.LoadLocation("Asia/Jakarta")
	now := time.Now().In(loc)
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
			Message: lib.StrPtr("Failed to create order: " + err.Error()),
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
			ProductCode:         cartItem.ProductCode,
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
				Message: lib.StrPtr("Failed to create order items: " + err.Error()),
			}, err
		}
	}

	if err := tx.Model(&Cart{}).Where("id = ?", cartId).Update("status", 1).Error; err != nil {
		tx.Rollback()
		return &proto.CreateOrderFromCartResponse{
			Success: false,
			Message: lib.StrPtr("Failed to update cart status: " + err.Error()),
		}, err
	}

	if err := tx.Commit().Error; err != nil {
		return &proto.CreateOrderFromCartResponse{
			Success: false,
			Message: lib.StrPtr("Failed to commit transaction: " + err.Error()),
		}, err
	}

	// --- TAMBAHKAN BLOK INI ---
	// Muat ulang order dengan OrderItems yang sudah terisi
	// Kita butuh preloads ini untuk createOrderEvent dan orderDocumentToProto
	var reloadedOrder OrderDocument
	if err := s.db.Where("id = ?", order.ID).
		Preload("OrderItems.Product.ProductGroup").
		Preload("OrderItems.Discount").
		Preload("PaymentType").
		First(&reloadedOrder).Error; err != nil {

		log.Printf("Gagal memuat ulang order %d untuk event: %v", order.ID, err)
		// Jika gagal, kirim data seadanya (event & proto akan tidak lengkap)
		reloadedOrder = order
	}
	// --- AKHIR BLOK TAMBAHAN ---

	// Gunakan 'reloadedOrder' yang sudah lengkap, BUKAN 'order'
	event := s.createOrderEvent(&reloadedOrder, EventOrderCreated)
	s.publishOrderEvent(ctx, event)

	return &proto.CreateOrderFromCartResponse{
		Success:       true,
		Message:       lib.StrPtr("Order created successfully from cart"),
		OrderDocument: s.orderDocumentToProto(reloadedOrder),
	}, nil
}

func (s *POSHandler) GetOrder(ctx context.Context, req *proto.GetOrderRequest) (*proto.GetOrderResponse, error) {
	if req.GetId() == 0 {
		return &proto.GetOrderResponse{
			Success: false,
			Message: lib.StrPtr("order id required"),
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
				Message: lib.StrPtr("Order not found"),
			}, nil
		}
		return &proto.GetOrderResponse{
			Success: false,
			Message: lib.StrPtr("Database error"),
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
		log.Printf("Error counting orders: %v", err)
		return &proto.ListOrdersResponse{
			Success: false,
			Message: lib.StrPtr("Database error counting orders"),
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
		log.Printf("Error fetching orders: %v", err)
		return &proto.ListOrdersResponse{
			Success: false,
			Message: lib.StrPtr("Database error fetching orders"),
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
			Message: lib.StrPtr("order id required"),
		}, nil
	}

	if req.GetVoidedBy() == 0 {
		return &proto.VoidOrderResponse{
			Success: false,
			Message: lib.StrPtr("voided_by (cashier_id) required"),
		}, nil
	}

	if req.GetReason() == "" {
		return &proto.VoidOrderResponse{
			Success: false,
			Message: lib.StrPtr("void reason required"),
		}, nil
	}

	var order OrderDocument
	if err := s.db.Where("id = ?", req.GetId()).
		Preload("OrderItems").
		First(&order).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.VoidOrderResponse{
				Success: false,
				Message: lib.StrPtr("Order not found"),
			}, nil
		}
		return &proto.VoidOrderResponse{
			Success: false,
			Message: lib.StrPtr("Database error"),
		}, err
	}

	if order.DocumentType == int32(proto.DocumentType_DOCUMENT_TYPE_VOID) {
		return &proto.VoidOrderResponse{
			Success: false,
			Message: lib.StrPtr("Order is already voided"),
		}, nil
	}

	if order.PaidStatus == int32(proto.PaidStatus_PAID_STATUS_PAID) {
		return &proto.VoidOrderResponse{
			Success: false,
			Message: lib.StrPtr("Cannot void a paid order. Use return instead."),
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
			Message: lib.StrPtr("Failed to void order: " + err.Error()),
		}, err
	}

	if err := tx.Commit().Error; err != nil {
		return &proto.VoidOrderResponse{
			Success: false,
			Message: lib.StrPtr("Failed to commit transaction: " + err.Error()),
		}, err
	}

	event := s.createOrderEvent(&order, EventOrderVoided)
	s.publishOrderEvent(ctx, event)

	return &proto.VoidOrderResponse{
		Success:       true,
		Message:       lib.StrPtr("Order voided successfully"),
		OrderDocument: s.orderDocumentToProto(order),
	}, nil
}

func (s *POSHandler) ReturnOrder(ctx context.Context, req *proto.ReturnOrderRequest) (*proto.ReturnOrderResponse, error) {
	if req.GetOriginalOrderId() == 0 {
		return &proto.ReturnOrderResponse{
			Success: false,
			Message: lib.StrPtr("original_order_id required"),
		}, nil
	}
	if req.GetProcessedBy() == 0 {
		return &proto.ReturnOrderResponse{
			Success: false,
			Message: lib.StrPtr("processed_by (cashier_id) required"),
		}, nil
	}
	if len(req.GetItemIds()) == 0 {
		return &proto.ReturnOrderResponse{
			Success: false,
			Message: lib.StrPtr("at least one item_id required for return"),
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
				Message: lib.StrPtr("Original order not found"),
			}, nil
		}
		return &proto.ReturnOrderResponse{
			Success: false,
			Message: lib.StrPtr("Database error"),
		}, err
	}

	if originalOrder.PaidStatus != int32(proto.PaidStatus_PAID_STATUS_PAID) {
		return &proto.ReturnOrderResponse{
			Success: false,
			Message: lib.StrPtr("Can only return paid orders"),
		}, nil
	}

	if originalOrder.DocumentType == int32(proto.DocumentType_DOCUMENT_TYPE_VOID) {
		return &proto.ReturnOrderResponse{
			Success: false,
			Message: lib.StrPtr("Cannot return a voided order"),
		}, nil
	}

	var itemsToReturn []OrderItem
	if err := s.db.Where("id IN ? AND document_id = ?", req.GetItemIds(), req.GetOriginalOrderId()).
		Preload("Product.ProductGroup").
		Preload("Discount").
		Find(&itemsToReturn).Error; err != nil {
		return &proto.ReturnOrderResponse{
			Success: false,
			Message: lib.StrPtr("Failed to fetch items: " + err.Error()),
		}, err
	}

	if len(itemsToReturn) == 0 {
		return &proto.ReturnOrderResponse{
			Success: false,
			Message: lib.StrPtr("No valid items found for return"),
		}, nil
	}

	if len(itemsToReturn) != len(req.GetItemIds()) {
		return &proto.ReturnOrderResponse{
			Success: false,
			Message: lib.StrPtr("Some item IDs are invalid or don't belong to this order"),
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
			Message: lib.StrPtr("Failed to create return document: " + err.Error()),
		}, err
	}

	for _, item := range itemsToReturn {
		returnItem := OrderItem{
			DocumentId:          returnDoc.ID,
			ProductCode:         item.ProductCode,
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
				Message: lib.StrPtr("Failed to create return items: " + err.Error()),
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
				Message: lib.StrPtr("Failed to update original order: " + err.Error()),
			}, err
		}
	}

	if err := tx.Commit().Error; err != nil {
		return &proto.ReturnOrderResponse{
			Success: false,
			Message: lib.StrPtr("Failed to commit transaction: " + err.Error()),
		}, err
	}

	event := s.createOrderEvent(&returnDoc, EventOrderReturned)
	s.publishOrderEvent(ctx, event)

	saleItems := s.orderItemsToSaleItems(originalOrder.OrderItems)
	saleRefundedEvent := SaleRefundedEvent{
		EventID:       uuid.New().String(),
		Timestamp:     time.Now(),
		TransactionID: originalOrder.DocumentNumber,
		DocumentID:    originalOrder.ID,
		WarehouseID:   1,
		Items:         saleItems,
	}

	err := s.publishStockChanges(ctx, "sale.refunded", saleRefundedEvent)
	if err != nil {
		log.Printf("Failed to publish sale.refunded event: %v", err)
	}

	return &proto.ReturnOrderResponse{
		Success:        true,
		Message:        lib.StrPtr("Return processed successfully"),
		ReturnDocument: s.orderDocumentToProto(returnDoc),
	}, nil
}
