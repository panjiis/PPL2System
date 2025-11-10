package utils

import "time"

type ListDiscountsQuery struct {
	Page        int     `form:"page,default=1"`
	PageSize    int     `form:"page_size,default=10"`
	IsActive    *bool   `form:"is_active"`
	ProductCode *string `form:"product_code"`
	SearchTerm  *int32  `form:"search"`
}

type ValidateDiscountRequest struct {
	DiscountID  int32  `json:"discount_id" binding:"required"`
	ProductCode string `json:"product_code" binding:"required"`
	Quantity    int32  `json:"quantity" binding:"required"`
}

type CreateDiscountRequest struct {
	DiscountName           string     `json:"discount_name" binding:"required"`
	DiscountType           int32      `json:"discount_type" binding:"required,oneof=1 2 3"`
	DiscountValue          *string    `json:"discount_value,omitempty"` // used for percentage/fixed
	BuyQuantity            *int32     `json:"buy_quantity,omitempty"`   // for BUY_X_GET_Y
	GetQuantity            *int32     `json:"get_quantity,omitempty"`   // for BUY_X_GET_Y
	ProductCode            *string    `json:"product_code,omitempty"`
	ProductGroupId         *int32     `json:"product_group_id,omitempty"`
	MinQuantity            int32      `json:"min_quantity" binding:"min=1"`
	MaxUsagePerTransaction *string     `json:"max_usage_per_transaction,omitempty"`
	ValidFrom              *time.Time `json:"valid_from,omitempty"`
	ValidUntil             *time.Time `json:"valid_until,omitempty"`
	IsActive               bool       `json:"is_active"`
}

type UpdateDiscountRequest struct {
	DiscountName           *string    `json:"discount_name,omitempty"`
	DiscountType           *int32     `json:"discount_type,omitempty"`
	DiscountValue          *string    `json:"discount_value,omitempty"`
	BuyQuantity            *int32     `json:"buy_quantity,omitempty"`
	GetQuantity            *int32     `json:"get_quantity,omitempty"`
	ProductCode            *string    `json:"product_code,omitempty"`
	ProductGroupId         *int32     `json:"product_group_id,omitempty"`
	MinQuantity            *int32     `json:"min_quantity,omitempty"`
	MaxUsagePerTransaction *string     `json:"max_usage_per_transaction,omitempty"`
	ValidFrom              *time.Time `json:"valid_from,omitempty"`
	ValidUntil             *time.Time `json:"valid_until,omitempty"`
	IsActive               *bool      `json:"is_active,omitempty"`
}
