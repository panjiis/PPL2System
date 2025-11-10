package utils

import (
	"fmt"
	"time"

	InventoryModels "syntra-system/internal/database/models"

	"gorm.io/gorm"
)

func GenerateProductCode(db *gorm.DB, productTypeId int32) (string, error) {
	tx := db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var productType InventoryModels.ProductType

	if err := tx.Set("gorm:query_option", "FOR UPDATE").
		Where("id = ?", productTypeId).
		First(&productType).Error; err != nil {

		tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			return fmt.Sprintf("GEN-%s-0001", time.Now().Format("20060102")), nil
		}
		return "", err
	}

	newProductId := productType.LastProductId + 1
	productCode := fmt.Sprintf("%s-%04d", productType.ProductTypeCode, newProductId)

	if err := tx.Model(&productType).
		Update("last_product_id", newProductId).Error; err != nil {
		tx.Rollback()
		return "", err
	}

	tx.Commit()
	return productCode, nil
}
