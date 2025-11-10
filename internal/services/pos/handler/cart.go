package handler

import (
	"context"
	"strconv"
	lib "syntra-system/internal/utils"
	proto "syntra-system/proto/protogen/pos"
	"time"

	"gorm.io/gorm"
)

func (s *POSHandler) CreateCart(ctx context.Context, req *proto.CreateCartRequest) (*proto.CreateCartResponse, error) {
	if req.GetCashierId() == 0 {
		return &proto.CreateCartResponse{
			Success: false,
			Message: lib.StrPtr("cashier_id required"),
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
			Message: lib.StrPtr("Failed to create cart: " + err.Error()),
		}, err
	}

	return &proto.CreateCartResponse{
		Success: true,
		Message: lib.StrPtr("Cart created successfully"),
		Cart:    s.cartToProto(cart),
	}, nil
}

func (s *POSHandler) GetCart(ctx context.Context, req *proto.GetCartRequest) (*proto.GetCartResponse, error) {
	if req.GetCartId() == "" {
		return &proto.GetCartResponse{
			Success: false,
			Message: lib.StrPtr("cart_id required"),
		}, nil
	}

	cartId, err := strconv.ParseInt(req.GetCartId(), 10, 64)
	if err != nil {
		return &proto.GetCartResponse{
			Success: false,
			Message: lib.StrPtr("Invalid cart_id format"),
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
				Message: lib.StrPtr("Cart not found"),
			}, nil
		}
		return &proto.GetCartResponse{
			Success: false,
			Message: lib.StrPtr("Database error"),
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
			Message: lib.StrPtr("cart_id required"),
		}, nil
	}

	if req.GetProductCode() == "" {
		return &proto.AddItemToCartResponse{
			Success: false,
			Message: lib.StrPtr("product_code required"),
		}, nil
	}

	if req.GetQuantity() <= 0 {
		return &proto.AddItemToCartResponse{
			Success: false,
			Message: lib.StrPtr("quantity must be greater than 0"),
		}, nil
	}

	cartId, err := strconv.ParseInt(req.GetCartId(), 10, 64)
	if err != nil {
		return &proto.AddItemToCartResponse{
			Success: false,
			Message: lib.StrPtr("Invalid cart_id format"),
		}, nil
	}

	var cart Cart
	if err := s.db.Where("id = ? AND status = ?", cartId, 0).First(&cart).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.AddItemToCartResponse{
				Success: false,
				Message: lib.StrPtr("Cart not found or inactive"),
			}, nil
		}
		return &proto.AddItemToCartResponse{
			Success: false,
			Message: lib.StrPtr("Database error"),
		}, err
	}

	var product Product
	if err := s.db.Where("product_code = ? AND is_active = ?", req.GetProductCode(), true).
		Preload("ProductGroup").
		First(&product).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.AddItemToCartResponse{
				Success: false,
				Message: lib.StrPtr("Product not found or inactive"),
			}, nil
		}
		return &proto.AddItemToCartResponse{
			Success: false,
			Message: lib.StrPtr("Database error"),
		}, err
	}

	if product.RequiresServiceEmployee && req.ServingEmployeeId == nil {
		return &proto.AddItemToCartResponse{
			Success: false,
			Message: lib.StrPtr("This product requires a service employee"),
		}, nil
	}

	var existingItem CartItem
	err = s.db.Where("cart_id = ? AND product_code = ?", cartId, req.GetProductCode()).
		First(&existingItem).Error

	if err == nil {
		existingItem.Quantity += req.GetQuantity()

		unitPrice, _ := strconv.ParseFloat(existingItem.UnitPrice, 64)
		lineTotal := unitPrice * float64(existingItem.Quantity)
		existingItem.LineTotal = strconv.FormatFloat(lineTotal, 'f', 2, 64)

		if err := s.db.Save(&existingItem).Error; err != nil {
			return &proto.AddItemToCartResponse{
				Success: false,
				Message: lib.StrPtr("Failed to update cart item: " + err.Error()),
			}, err
		}
	} else if err == gorm.ErrRecordNotFound {
		unitPrice, _ := strconv.ParseFloat(product.ProductPrice, 64)
		lineTotal := unitPrice * float64(req.GetQuantity())

		cartItem := CartItem{
			CartId:            cartId,
			ProductCode:       req.GetProductCode(),
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
				Message: lib.StrPtr("Failed to add item to cart: " + err.Error()),
			}, err
		}
	} else {
		return &proto.AddItemToCartResponse{
			Success: false,
			Message: lib.StrPtr("Database error"),
		}, err
	}

	if err := s.recalculateCartTotals(ctx, cartId); err != nil {
		return &proto.AddItemToCartResponse{
			Success: false,
			Message: lib.StrPtr("Failed to recalculate totals: " + err.Error()),
		}, err
	}

	if err := s.db.Where("id = ?", cartId).
		Preload("CartItems.Product.ProductGroup").
		Preload("CartItems.Discount").
		First(&cart).Error; err != nil {
		return &proto.AddItemToCartResponse{
			Success: false,
			Message: lib.StrPtr("Failed to reload cart"),
		}, err
	}

	return &proto.AddItemToCartResponse{
		Success: true,
		Message: lib.StrPtr("Item added to cart successfully"),
		Cart:    s.cartToProto(cart),
	}, nil
}

func (s *POSHandler) RemoveItemFromCart(ctx context.Context, req *proto.RemoveItemFromCartRequest) (*proto.RemoveItemFromCartResponse, error) {
	if req.GetCartId() == "" {
		return &proto.RemoveItemFromCartResponse{
			Success: false,
			Message: lib.StrPtr("cart_id required"),
		}, nil
	}

	if req.GetItemId() == "" {
		return &proto.RemoveItemFromCartResponse{
			Success: false,
			Message: lib.StrPtr("item_id required"),
		}, nil
	}

	cartId, err := strconv.ParseInt(req.GetCartId(), 10, 64)
	if err != nil {
		return &proto.RemoveItemFromCartResponse{
			Success: false,
			Message: lib.StrPtr("Invalid cart_id format"),
		}, nil
	}

	itemId, err := strconv.ParseInt(req.GetItemId(), 10, 64)
	if err != nil {
		return &proto.RemoveItemFromCartResponse{
			Success: false,
			Message: lib.StrPtr("Invalid item_id format"),
		}, nil
	}

	var cart Cart
	if err := s.db.Where("id = ? AND status = ?", cartId, 0).First(&cart).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.RemoveItemFromCartResponse{
				Success: false,
				Message: lib.StrPtr("Cart not found or inactive"),
			}, nil
		}
		return &proto.RemoveItemFromCartResponse{
			Success: false,
			Message: lib.StrPtr("Database error"),
		}, err
	}

	result := s.db.Where("id = ? AND cart_id = ?", itemId, cartId).Delete(&CartItem{})
	if result.Error != nil {
		return &proto.RemoveItemFromCartResponse{
			Success: false,
			Message: lib.StrPtr("Failed to remove item: " + result.Error.Error()),
		}, result.Error
	}

	if result.RowsAffected == 0 {
		return &proto.RemoveItemFromCartResponse{
			Success: false,
			Message: lib.StrPtr("Cart item not found"),
		}, nil
	}

	if err := s.recalculateCartTotals(ctx, cartId); err != nil {
		return &proto.RemoveItemFromCartResponse{
			Success: false,
			Message: lib.StrPtr("Failed to recalculate totals: " + err.Error()),
		}, err
	}

	if err := s.db.Where("id = ?", cartId).
		Preload("CartItems.Product.ProductGroup").
		Preload("CartItems.Discount").
		First(&cart).Error; err != nil {
		return &proto.RemoveItemFromCartResponse{
			Success: false,
			Message: lib.StrPtr("Failed to reload cart"),
		}, err
	}

	return &proto.RemoveItemFromCartResponse{
		Success: true,
		Message: lib.StrPtr("Item removed from cart successfully"),
		Cart:    s.cartToProto(cart),
	}, nil
}

func (s *POSHandler) ApplyDiscount(ctx context.Context, req *proto.ApplyDiscountRequest) (*proto.ApplyDiscountResponse, error) {
	if req.GetCartId() == "" {
		return &proto.ApplyDiscountResponse{
			Success: false,
			Message: lib.StrPtr("cart_id required"),
		}, nil
	}

	if req.GetDiscountId() == 0 {
		return &proto.ApplyDiscountResponse{
			Success: false,
			Message: lib.StrPtr("discount_id required"),
		}, nil
	}

	cartId, err := strconv.ParseInt(req.GetCartId(), 10, 64)
	if err != nil {
		return &proto.ApplyDiscountResponse{
			Success: false,
			Message: lib.StrPtr("Invalid cart_id format"),
		}, nil
	}

	var cart Cart
	if err := s.db.Where("id = ? AND status = ?", cartId, 0).First(&cart).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.ApplyDiscountResponse{
				Success: false,
				Message: lib.StrPtr("Cart not found or inactive"),
			}, nil
		}
		return &proto.ApplyDiscountResponse{
			Success: false,
			Message: lib.StrPtr("Database error"),
		}, err
	}

	var discount Discount
	if err := s.db.Where("id = ? AND is_active = ?", req.GetDiscountId(), true).
		First(&discount).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.ApplyDiscountResponse{
				Success: false,
				Message: lib.StrPtr("Discount not found or inactive"),
			}, nil
		}
		return &proto.ApplyDiscountResponse{
			Success: false,
			Message: lib.StrPtr("Database error"),
		}, err
	}

	now := time.Now()
	if discount.ValidFrom != nil && now.Before(*discount.ValidFrom) {
		return &proto.ApplyDiscountResponse{
			Success: false,
			Message: lib.StrPtr("Discount is not yet valid"),
		}, nil
	}
	if discount.ValidUntil != nil && now.After(*discount.ValidUntil) {
		return &proto.ApplyDiscountResponse{
			Success: false,
			Message: lib.StrPtr("Discount has expired"),
		}, nil
	}

	var itemIds []int64
	if len(req.ItemIds) > 0 {

		var items []CartItem
		query := s.db.Where("cart_id = ?", cartId)

		productCodes := req.GetItemIds()
		query = query.Where("product_code IN ?", productCodes)

		if discount.ProductCode != nil {
			found := false
			for _, pc := range productCodes {
				if pc == *discount.ProductCode {
					found = true
					break
				}
			}
			if !found {
				return &proto.ApplyDiscountResponse{
					Success: false,
					Message: lib.StrPtr("Requested items do not match discount's product"),
				}, nil
			}
			query = query.Where("product_code = ?", discount.ProductCode)
		}

		if err := query.Find(&items).Error; err != nil {
			return &proto.ApplyDiscountResponse{
				Success: false,
				Message: lib.StrPtr("Failed to find eligible items"),
			}, err
		}

		for _, item := range items {
			itemIds = append(itemIds, item.ID)
		}
	} else {
		var items []CartItem
		query := s.db.Where("cart_id = ?", cartId)

		if discount.ProductCode != nil {
			query = query.Where("product_code = ?", discount.ProductCode)
		} else if discount.ProductGroupId != nil {
			query = query.Joins("JOIN products ON product_code = cart_items.product_code").
				Where("products.product_group_id = ?", discount.ProductGroupId)
		}

		if err := query.Find(&items).Error; err != nil {
			return &proto.ApplyDiscountResponse{
				Success: false,
				Message: lib.StrPtr("Failed to find eligible items"),
			}, err
		}

		for _, item := range items {
			itemIds = append(itemIds, item.ID)
		}
	}

	if len(itemIds) == 0 {
		return &proto.ApplyDiscountResponse{
			Success: false,
			Message: lib.StrPtr("No eligible items found for this discount"),
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

		item.DiscountId = &discount.Id
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
			Message: lib.StrPtr("Failed to recalculate totals: " + err.Error()),
		}, err
	}

	if err := s.db.Where("id = ?", cartId).
		Preload("CartItems.Product.ProductGroup").
		Preload("CartItems.Discount").
		First(&cart).Error; err != nil {
		return &proto.ApplyDiscountResponse{
			Success: false,
			Message: lib.StrPtr("Failed to reload cart"),
		}, err
	}

	return &proto.ApplyDiscountResponse{
		Success: true,
		Message: lib.StrPtr("Discount applied successfully"),
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
