package handler

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	lib "syntra-system/internal/utils"
	proto "syntra-system/proto/protogen/pos"

	"gorm.io/gorm"
)

var wibLoc *time.Location

func (s *POSHandler) CreateDiscount(ctx context.Context, req *proto.CreateDiscountRequest) (*proto.CreateDiscountResponse, error) {
	loc, _ := time.LoadLocation("Asia/Jakarta")

	if err := s.validateDiscountValue(req.GetDiscountType(), req.GetDiscountValue()); err != nil {
		return &proto.CreateDiscountResponse{
			Success: false,
			Message: lib.StrPtr("Invalid discount value: " + err.Error()),
		}, nil
	}

	if req.ValidFrom != nil && req.ValidUntil != nil {
		if req.GetValidFrom().AsTime().In(loc).After(req.GetValidUntil().AsTime().In(loc)) {
			return &proto.CreateDiscountResponse{
				Success: false,
				Message: lib.StrPtr("Valid from date cannot be after valid until date"),
			}, nil
		}
	}

	if req.ProductCode != nil {
		var product Product
		if err := s.db.First(&product, "product_code = ?", req.GetProductCode()).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return &proto.CreateDiscountResponse{
					Success: false,
					Message: lib.StrPtr("Product not found"),
				}, nil
			}
			return &proto.CreateDiscountResponse{
				Success: false,
				Message: lib.StrPtr("Database error"),
			}, err
		}
	}

	if req.ProductGroupId != nil && req.GetProductGroupId() != 0 {
		var productGroup ProductGroup
		if err := s.db.First(&productGroup, req.GetProductGroupId()).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return &proto.CreateDiscountResponse{
					Success: false,
					Message: lib.StrPtr("Product group not found"),
				}, nil
			}
			return &proto.CreateDiscountResponse{
				Success: false,
				Message: lib.StrPtr("Database error"),
			}, err
		}
	}

	if err := s.checkForConflictingDiscounts(req); err != nil {
		return &proto.CreateDiscountResponse{
			Success: false,
			Message: lib.StrPtr("Conflict: " + err.Error()),
		}, nil
	}

	var buyQuantity, getQuantity *int32
	if req.GetDiscountType() == proto.DiscountType_DISCOUNT_TYPE_BUY_X_GET_Y {
		parts := strings.Split(req.GetDiscountValue(), ":")
		if len(parts) == 2 {
			buyQty, err1 := strconv.ParseInt(parts[0], 10, 32)
			getQty, err2 := strconv.ParseInt(parts[1], 10, 32)

			if err1 == nil && err2 == nil {
				buyQty32 := int32(buyQty)
				getQty32 := int32(getQty)
				buyQuantity = &buyQty32
				getQuantity = &getQty32
			}
		}
	}

	discount := &Discount{
		DiscountName:  req.GetDiscountName(),
		DiscountType:  int32(req.GetDiscountType().Number()),
		DiscountValue: req.GetDiscountValue(),
		MinQuantity:   req.GetMinQuantity(),
		IsActive:      req.GetIsActive(),
	}

	if req.ProductCode != nil {
		discount.ProductCode = req.ProductCode
	}
	if req.ProductGroupId != nil {
		discount.ProductGroupId = req.ProductGroupId
	}
	if req.MaxUsagePerTransaction != nil {
		discount.MaxUsagePerTransaction = req.MaxUsagePerTransaction
	}
	if req.ValidFrom != nil {
		validFrom := req.GetValidFrom().AsTime().In(loc)
		discount.ValidFrom = &validFrom
	}
	if req.ValidUntil != nil {
		validUntil := req.GetValidUntil().AsTime().In(loc)
		discount.ValidUntil = &validUntil
	}
	if buyQuantity != nil {
		discount.BuyQuantity = buyQuantity
	}
	if getQuantity != nil {
		discount.GetQuantity = getQuantity
	}

	if err := s.db.Create(discount).Error; err != nil {
		return &proto.CreateDiscountResponse{
			Success: false,
			Message: lib.StrPtr("Error creating discount"),
		}, err
	}

	s.db.Preload("Product").Preload("ProductGroup").First(discount, discount.Id)
	s.TriggerSchedulerRecalculation()

	return &proto.CreateDiscountResponse{
		Success:  true,
		Message:  lib.StrPtr("Discount created successfully"),
		Discount: s.discountToProto(discount),
	}, nil
}

func (s *POSHandler) GetDiscount(ctx context.Context, req *proto.GetDiscountRequest) (*proto.GetDiscountResponse, error) {
	var discount Discount
	if err := s.db.Preload("Product").Preload("ProductGroup").First(&discount, req.GetId()).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.GetDiscountResponse{
				Success: false,
				Message: lib.StrPtr("Discount not found"),
			}, nil
		}
		return &proto.GetDiscountResponse{
			Success: false,
			Message: lib.StrPtr("Database error"),
		}, err
	}

	return &proto.GetDiscountResponse{
		Success:  true,
		Discount: s.discountToProto(&discount),
	}, nil
}

func (s *POSHandler) ListDiscounts(ctx context.Context, req *proto.ListDiscountsRequest) (*proto.ListDiscountsResponse, error) {
	var discounts []Discount
	var total int64

	query := s.db.Model(&Discount{}).Preload("Product").Preload("ProductGroup")

	if req.Search != nil && req.GetSearch() != 0 {
		search := req.GetSearch()
		query = query.Where("discount_name LIKE ?", search)
	}

	if req.IsActive != nil {
		query = query.Where("is_active = ?", req.GetIsActive())
	}

	if req.DiscountType != nil && *req.DiscountType != proto.DiscountType_DISCOUNT_TYPE_UNSPECIFIED {
		query = query.Where("discount_type = ?", req.GetDiscountType().String())
	}

	if req.ProductCode != nil {
		query = query.Where("product_code = ?", req.GetProductCode())
	}

	if req.ProductGroupId != nil {
		query = query.Where("product_group_id = ?", req.GetProductGroupId())
	}

	if err := query.Count(&total).Error; err != nil {
		return &proto.ListDiscountsResponse{
			Success: false,
			Message: lib.StrPtr("Database error"),
		}, err
	}

	page := int(req.GetPage())
	pageSize := int(req.GetPageSize())
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Find(&discounts).Error; err != nil {
		return &proto.ListDiscountsResponse{
			Success: false,
			Message: lib.StrPtr("Database error"),
		}, err
	}

	pbDiscounts := make([]*proto.Discount, len(discounts))
	for i, discount := range discounts {
		pbDiscounts[i] = s.discountToProto(&discount)
	}

	return &proto.ListDiscountsResponse{
		Success:   true,
		Discounts: pbDiscounts,
		Total:     int32(total),
		Page:      req.GetPage(),
		PageSize:  req.GetPageSize(),
	}, nil
}

func (s *POSHandler) UpdateDiscount(ctx context.Context, req *proto.UpdateDiscountRequest) (*proto.UpdateDiscountResponse, error) {
	var discount Discount
	loc, _ := time.LoadLocation("Asia/Jakarta")

	if err := s.db.Preload("Product").Preload("ProductGroup").First(&discount, req.GetId()).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.UpdateDiscountResponse{
				Success: false,
				Message: lib.StrPtr("Discount not found"),
			}, nil
		}
		return &proto.UpdateDiscountResponse{
			Success: false,
			Message: lib.StrPtr("Database error"),
		}, err
	}

	updates := make(map[string]interface{})

	if req.DiscountName != nil {
		updates["discount_name"] = req.GetDiscountName()
	}
	if req.DiscountType != nil && req.GetDiscountType() != proto.DiscountType_DISCOUNT_TYPE_UNSPECIFIED {
		updates["discount_type"] = req.GetDiscountType()

		if req.DiscountValue != nil {
			if err := s.validateDiscountValue(req.GetDiscountType(), req.GetDiscountValue()); err != nil {
				return &proto.UpdateDiscountResponse{
					Success: false,
					Message: lib.StrPtr("Invalid discount value: " + err.Error()),
				}, nil
			}
		}
	}
	if req.DiscountValue != nil {
		var discountType proto.DiscountType

		if req.DiscountType != nil {
			discountType = req.GetDiscountType()
		} else if discount.DiscountType != 0 {
			discountType = proto.DiscountType(discount.DiscountType)
		} else {
			return &proto.UpdateDiscountResponse{
				Success: false,
				Message: lib.StrPtr("Discount type is required"),
			}, nil
		}

		if err := s.validateDiscountValue(discountType, req.GetDiscountValue()); err != nil {
			return &proto.UpdateDiscountResponse{
				Success: false,
				Message: lib.StrPtr("Invalid discount value: " + err.Error()),
			}, nil
		}

		updates["discount_value"] = req.GetDiscountValue()
	}
	if req.ProductCode != nil {
		updates["product_code"] = req.GetProductCode()
	}
	if req.ProductGroupId != nil {
		updates["product_group_id"] = req.GetProductGroupId()
	}
	if req.MinQuantity != nil {
		updates["min_quantity"] = req.GetMinQuantity()
	}
	if req.MaxUsagePerTransaction != nil {
		updates["max_usage_per_transaction"] = req.GetMaxUsagePerTransaction()
	}
	if req.ValidFrom != nil {
		validFrom := req.GetValidFrom().AsTime().In(loc)
		updates["valid_from"] = &validFrom
	}
	if req.ValidUntil != nil {
		validUntil := req.GetValidUntil().AsTime().In(loc)
		updates["valid_until"] = &validUntil
	}
	if req.IsActive != nil {
		updates["is_active"] = req.GetIsActive()
	}
	if req.BuyQuantity != nil {
		updates["buy_quantity"] = req.GetBuyQuantity()
	}
	if req.GetQuantity != nil {
		updates["get_quantity"] = req.GetGetQuantity()
	}

	if len(updates) > 0 {
		if err := s.db.Model(&discount).Updates(updates).Error; err != nil {
			return &proto.UpdateDiscountResponse{
				Success: false,
				Message: lib.StrPtr("Error updating discount"),
			}, err
		}
	}

	s.db.Preload("Product").Preload("ProductGroup").First(&discount, discount.Id)
	s.TriggerSchedulerRecalculation()

	return &proto.UpdateDiscountResponse{
		Success:  true,
		Message:  lib.StrPtr("Discount updated successfully"),
		Discount: s.discountToProto(&discount),
	}, nil
}

func (s *POSHandler) DeleteDiscount(ctx context.Context, req *proto.DeleteDiscountRequest) (*proto.DeleteDiscountResponse, error) {
	var discount Discount
	if err := s.db.First(&discount, req.GetId()).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.DeleteDiscountResponse{
				Success: false,
				Message: lib.StrPtr("Discount not found"),
			}, nil
		}
		return &proto.DeleteDiscountResponse{
			Success: false,
			Message: lib.StrPtr("Database error"),
		}, err
	}

	if err := s.db.Delete(&discount).Error; err != nil {
		return &proto.DeleteDiscountResponse{
			Success: false,
			Message: lib.StrPtr("Error deleting discount"),
		}, err
	}

	return &proto.DeleteDiscountResponse{
		Success: true,
		Message: lib.StrPtr("Discount deleted successfully"),
	}, nil
}

func (s *POSHandler) ValidateDiscountForCartItem(discount *Discount, cartItem *CartItem) error {
	if !discount.IsActive {
		return fmt.Errorf("discount is not active")
	}

	now := time.Now()
	if discount.ValidFrom != nil && now.Before(*discount.ValidFrom) {
		return fmt.Errorf("discount is not yet valid")
	}
	if discount.ValidUntil != nil && now.After(*discount.ValidUntil) {
		return fmt.Errorf("discount has expired")
	}

	if cartItem.Quantity < discount.MinQuantity {
		return fmt.Errorf("minimum quantity required is %d, but cart has %d", discount.MinQuantity, cartItem.Quantity)
	}

	if discount.ProductCode != nil && cartItem.ProductCode != *discount.ProductCode {
		return fmt.Errorf("discount does not apply to this product")
	}

	if discount.ProductGroupId != nil {
		var product Product
		if err := s.db.First(&product, "product_code = ?", cartItem.ProductCode).Error; err != nil {
			return fmt.Errorf("product not found")
		}
		if product.ProductGroupId != discount.ProductGroupId {
			return fmt.Errorf("discount does not apply to this product group")
		}
	}

	switch discount.DiscountType {
	case 1:
		_, err := strconv.ParseFloat(discount.DiscountValue, 64)
		if err != nil {
			return fmt.Errorf("invalid percentage discount value: %s", discount.DiscountValue)
		}
	case 2:
		_, err := strconv.ParseFloat(discount.DiscountValue, 64)
		if err != nil {
			return fmt.Errorf("invalid fixed amount discount value: %s", discount.DiscountValue)
		}
	case 3:
		parts := strings.Split(discount.DiscountValue, ":")
		if len(parts) != 2 {
			return fmt.Errorf("BUY_X_GET_Y discount value must be in format 'X:Y'")
		}
		buyQty, err1 := strconv.ParseInt(parts[0], 10, 32)
		getQty, err2 := strconv.ParseInt(parts[1], 10, 32)
		if err1 != nil || err2 != nil || buyQty <= 0 || getQty <= 0 {
			return fmt.Errorf("invalid BUY_X_GET_Y format")
		}

		totalRequired := buyQty + getQty
		if int64(cartItem.Quantity) < totalRequired {
			return fmt.Errorf("need at least %d items for this buy-x-get-y discount", totalRequired)
		}
	}

	return nil
}

func (s *POSHandler) CalculateDiscountAmount(discount *Discount, cartItem *CartItem) (float64, error) {
	if err := s.ValidateDiscountForCartItem(discount, cartItem); err != nil {
		return 0, err
	}

	unitPrice, err := strconv.ParseFloat(cartItem.UnitPrice, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid unit price: %v", err)
	}

	var discountAmount float64

	switch discount.DiscountType {
	case 1:
		percentage, err := strconv.ParseFloat(discount.DiscountValue, 64)
		if err != nil {
			return 0, err
		}
		discountAmount = (unitPrice * float64(cartItem.Quantity) * percentage) / 100

		if discount.MaxUsagePerTransaction != nil {
			maxEligibleValue := float64(*discount.MaxUsagePerTransaction) * unitPrice
			maxPossibleDiscount := (maxEligibleValue * percentage) / 100
			if discountAmount > maxPossibleDiscount {
				discountAmount = maxPossibleDiscount
			}
		}

	case 2:
		fixedAmount, err := strconv.ParseFloat(discount.DiscountValue, 64)
		if err != nil {
			return 0, err
		}
		discountAmount = fixedAmount

	case 3:
		parts := strings.Split(discount.DiscountValue, ":")
		if len(parts) != 2 {
			return 0, fmt.Errorf("invalid BUY_X_GET_Y format")
		}
		buyQty, err1 := strconv.ParseInt(parts[0], 10, 32)
		getQty, err2 := strconv.ParseInt(parts[1], 10, 32)
		if err1 != nil || err2 != nil {
			return 0, fmt.Errorf("invalid BUY_X_GET_Y format")
		}

		sets := cartItem.Quantity / int32(buyQty+getQty)
		freeItems := sets * int32(getQty)
		discountAmount = float64(freeItems) * unitPrice
	}

	maxPossibleLineDiscount := unitPrice * float64(cartItem.Quantity)
	if discountAmount > maxPossibleLineDiscount {
		discountAmount = maxPossibleLineDiscount
	}

	return discountAmount, nil
}

func (s *POSHandler) ValidateAndApplyDiscount(discountId int32, cartItem *CartItem) (*proto.Discount, float64, error) {
	var discount Discount
	if err := s.db.First(&discount, discountId).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, 0, fmt.Errorf("discount not found")
		}
		return nil, 0, err
	}

	if err := s.ValidateDiscountForCartItem(&discount, cartItem); err != nil {
		return nil, 0, err
	}

	discountAmount, err := s.CalculateDiscountAmount(&discount, cartItem)
	if err != nil {
		return nil, 0, err
	}

	pbDiscount := s.discountToProto(&discount)

	return pbDiscount, discountAmount, nil
}

func (s *POSHandler) ValidateDiscountForOrder(discount *Discount, cartItems []*CartItem) error {
	if !discount.IsActive {
		return fmt.Errorf("discount is not active")
	}

	now := time.Now()
	if discount.ValidFrom != nil && now.Before(*discount.ValidFrom) {
		return fmt.Errorf("discount is not yet valid")
	}
	if discount.ValidUntil != nil && now.After(*discount.ValidUntil) {
		return fmt.Errorf("discount has expired")
	}

	if discount.ProductCode != nil {
		matches := false
		for _, item := range cartItems {
			if item.ProductCode == *discount.ProductCode {
				matches = true
				break
			}
		}
		if !matches {
			return fmt.Errorf("no matching products in cart for this discount")
		}
	}

	if discount.ProductGroupId != nil {
		matches := false
		for _, item := range cartItems {
			var product Product
			if err := s.db.First(&product, "product_code = ?", item.ProductCode).Error; err != nil {
				continue
			}
			if product.ProductGroupId == discount.ProductGroupId {
				matches = true
				break
			}
		}
		if !matches {
			return fmt.Errorf("no matching products in cart for this discount")
		}
	}

	totalQuantity := int32(0)
	for _, item := range cartItems {
		totalQuantity += item.Quantity
	}

	if totalQuantity < discount.MinQuantity {
		return fmt.Errorf("minimum quantity required is %d, but order has %d", discount.MinQuantity, totalQuantity)
	}

	return nil
}

func (s *POSHandler) validateDiscountValue(discountType proto.DiscountType, value string) error {
	switch discountType {
	case proto.DiscountType_DISCOUNT_TYPE_PERCENTAGE:
		val, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("percentage discount value must be a valid number")
		}
		if val <= 0 || val > 100 {
			return fmt.Errorf("percentage discount must be between 0 and 100")
		}
	case proto.DiscountType_DISCOUNT_TYPE_FIXED_AMOUNT:
		val, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("fixed amount discount value must be a valid number")
		}
		if val <= 0 {
			return fmt.Errorf("fixed amount discount must be greater than 0")
		}
	case proto.DiscountType_DISCOUNT_TYPE_BUY_X_GET_Y:
		parts := strings.Split(value, ":")
		if len(parts) != 2 {
			return fmt.Errorf("BUY_X_GET_Y discount value must be in format 'X:Y'")
		}

		buyQty, err := strconv.ParseInt(parts[0], 10, 32)
		if err != nil || buyQty <= 0 {
			return fmt.Errorf("buy quantity must be a positive integer")
		}

		getQty, err := strconv.ParseInt(parts[1], 10, 32)
		if err != nil || getQty <= 0 {
			return fmt.Errorf("get quantity must be a positive integer")
		}
	case proto.DiscountType_DISCOUNT_TYPE_UNSPECIFIED:
		return fmt.Errorf("discount type cannot be unspecified")
	default:
		return fmt.Errorf("unknown discount type")
	}
	return nil
}

func (s *POSHandler) checkForConflictingDiscounts(req *proto.CreateDiscountRequest) error {
	currentTime := time.Date(2025, 11, 5, 9, 44, 19, 0, time.UTC)

	if req.ValidFrom != nil && req.ValidUntil != nil {
		validFrom := req.GetValidFrom().AsTime()
		validUntil := req.GetValidUntil().AsTime()

		if validFrom.Before(currentTime) {
			return fmt.Errorf("valid_from cannot be in the past")
		}
		if validUntil.Before(validFrom) {
			return fmt.Errorf("valid_until must be after valid_from")
		}
	}

	query := s.db.Model(&Discount{}).Where("is_active = ?", true)

	if req.ProductCode != nil {
		productQuery := query.Where("product_code = ?", req.GetProductCode())

		if req.ValidFrom != nil && req.ValidUntil != nil {
			validFrom := req.GetValidFrom().AsTime()
			validUntil := req.GetValidUntil().AsTime()

			productQuery = productQuery.Where(
				"(valid_from < ? AND valid_until > ?) OR "+
					"(valid_from BETWEEN ? AND ?) OR "+
					"(valid_until BETWEEN ? AND ?)",
				validFrom, validUntil,
				validFrom, validUntil,
				validFrom, validUntil,
			)
		}

		var productCount int64
		if err := productQuery.Count(&productCount).Error; err != nil {
			return err
		}

		if productCount > 0 {
			return fmt.Errorf("conflicting active discount already exists for this product code")
		}
	}

	if req.ProductGroupId != nil && req.GetProductGroupId() != 0 {
		groupQuery := query.Where("product_group_id = ?", req.GetProductGroupId())

		if req.ValidFrom != nil && req.ValidUntil != nil {
			validFrom := req.GetValidFrom().AsTime()
			validUntil := req.GetValidUntil().AsTime()

			groupQuery = groupQuery.Where(
				"(valid_from < ? AND valid_until > ?) OR "+
					"(valid_from BETWEEN ? AND ?) OR "+
					"(valid_until BETWEEN ? AND ?)",
				validFrom, validUntil,
				validFrom, validUntil,
				validFrom, validUntil,
			)
		}

		var groupCount int64
		if err := groupQuery.Count(&groupCount).Error; err != nil {
			return err
		}

		if groupCount > 0 {
			return fmt.Errorf("conflicting active discount already exists for this product group")
		}
	}

	return nil
}

func (s *POSHandler) stringToDiscountType(sType int32) proto.DiscountType {
	switch sType {
	case 1:
		return proto.DiscountType_DISCOUNT_TYPE_PERCENTAGE
	case 2:
		return proto.DiscountType_DISCOUNT_TYPE_FIXED_AMOUNT
	case 3:
		return proto.DiscountType_DISCOUNT_TYPE_BUY_X_GET_Y
	default:
		return proto.DiscountType_DISCOUNT_TYPE_UNSPECIFIED
	}
}

func (s *POSHandler) StartDiscountScheduler(parentCtx context.Context) {
	var wibLoc *time.Location
	wibLoc, _ = time.LoadLocation("Asia/Jakarta")
	s.schedulerMu.Lock()
	defer s.schedulerMu.Unlock()

	if s.cancelFunc != nil {
		s.cancelFunc()
		s.schedulerWg.Wait()
	}

	ctx, cancel := context.WithCancel(parentCtx)
	s.parentCtx = parentCtx
	s.cancelFunc = cancel
	s.schedulerWg.Add(1)

	go func() {
		defer s.schedulerWg.Done()
		s.updateDiscountsAndScheduleNext(ctx, wibLoc)
	}()
}

func (s *POSHandler) updateDiscountsAndScheduleNext(ctx context.Context, wib *time.Location) {
	s.updateDiscountActiveStatus(ctx, wib)

	nextUpdateTime, ok := s.calculateNextUpdateTime(ctx, wib)
	if !ok {
		log.Println("No future discounts found, stopping scheduler goroutine.")
		return
	}

	log.Printf("Next discount status change scheduled for %s", nextUpdateTime.Format("2006-01-02 15:04:05 MST"))

	durationUntilNext := time.Until(nextUpdateTime)

	timer := time.NewTimer(durationUntilNext)
	defer timer.Stop()

	select {
	case <-timer.C:
		s.updateDiscountsAndScheduleNext(ctx, wibLoc)
	case <-ctx.Done():
		log.Println("Stopping discount scheduler due to context cancellation")
		return
	}
}

func (s *POSHandler) TriggerSchedulerRecalculation() {
	log.Println("New discount added, triggering scheduler recalculation...")
	s.StartDiscountScheduler(s.parentCtx)
}

func (s *POSHandler) updateDiscountActiveStatus(ctx context.Context, wib *time.Location) {
	nowWIB := time.Now().In(wib)
	nowStr := nowWIB.Format("2006-01-02 15:04:05")

	query := `
		UPDATE discounts
		SET is_active = CASE
			WHEN (is_active = FALSE AND valid_from IS NOT NULL AND ? >= valid_from) THEN TRUE
			WHEN (is_active = TRUE AND valid_until IS NOT NULL AND ? > valid_until) THEN FALSE
			ELSE is_active
		END
		WHERE
			(is_active = FALSE AND valid_from IS NOT NULL AND ? >= valid_from) OR
			(is_active = TRUE AND valid_until IS NOT NULL AND ? > valid_until);
	`

	result := s.db.WithContext(ctx).Exec(query, nowStr, nowStr, nowStr, nowStr)

	if result.Error != nil {
		log.Printf("Failed to update discount active status: %v", result.Error)
		return
	}

	rowsAffected := result.RowsAffected
	log.Printf("Updated %d discount records at %s", rowsAffected, nowWIB.Format("2006-01-02 15:04:05 MST"))
}

func (s *POSHandler) calculateNextUpdateTime(ctx context.Context, wib *time.Location) (time.Time, bool) {
	nowWIB := time.Now().In(wib)

	var nextActivation *time.Time
	err := s.db.WithContext(ctx).
		Model(&Discount{}).
		Select("MIN(valid_from)").
		Where("is_active = ? AND valid_from IS NOT NULL AND valid_from > ?", false, nowWIB).
		Scan(&nextActivation).Error

	if err != nil {
		log.Printf("Error querying next activation time: %v", err)
		return time.Time{}, false
	}

	var nextDeactivation *time.Time
	err = s.db.WithContext(ctx).
		Model(&Discount{}).
		Select("MIN(valid_until)").
		Where("is_active = ? AND valid_until IS NOT NULL AND valid_until > ?", true, nowWIB).
		Scan(&nextDeactivation).Error

	if err != nil {
		log.Printf("Error querying next deactivation time: %v", err)
		return time.Time{}, false
	}

	var earliestTime *time.Time = nil
	if nextActivation != nil && nextDeactivation != nil {
		if nextActivation.Before(*nextDeactivation) {
			earliestTime = nextActivation
		} else {
			earliestTime = nextDeactivation
		}
	} else if nextActivation != nil {
		earliestTime = nextActivation
	} else if nextDeactivation != nil {
		earliestTime = nextDeactivation
	}

	if earliestTime == nil {
		return time.Time{}, false
	}

	return *earliestTime, true
}
