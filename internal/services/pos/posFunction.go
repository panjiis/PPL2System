package utils

import (
	"errors"
	"fmt"
	POSModels "syntra-system/internal/database/models"
	proto "syntra-system/proto/protogen/pos"
	"time"

	"gorm.io/gorm"
)

func InterpretDiscountTypeValue(t proto.DiscountType) string {
	switch t {
	case proto.DiscountType_DISCOUNT_TYPE_PERCENTAGE:
		return "Percentage"
	case proto.DiscountType_DISCOUNT_TYPE_FIXED_AMOUNT:
		return "Fixed Amount"
	case proto.DiscountType_DISCOUNT_TYPE_BUY_X_GET_Y:
		return "Buy X Get Y"
	default:
		return "Unknown"
	}
}

func GenerateServiceCode(db *gorm.DB, productGroupId int32) (string, error) {
	tx := db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var productGroup POSModels.ProductGroup

	if err := tx.Set("gorm:query_option", "FOR UPDATE").
		Where("id = ?", productGroupId).
		First(&productGroup).Error; err != nil {

		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Sprintf("GEN-%s-0001", time.Now().Format("20060102")), nil
		}
		return "", err
	}

	currentID := int32(0)
	if productGroup.LastServiceId != nil {
		currentID = *productGroup.LastServiceId
	}
	newServiceId := currentID + 1
	serviceCode := fmt.Sprintf("SRV-%04d", newServiceId)

	if err := tx.Model(&productGroup).
		Update("last_service_id", newServiceId).Error; err != nil {
		tx.Rollback()
		return "", err
	}

	tx.Commit()
	return serviceCode, nil
}
