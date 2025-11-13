package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	proto "syntra-system/proto/protogen/inventory"
)

type InventoryHTTPHandler struct {
	inventoryClient proto.InventoryServiceClient
}

func NewInventoryHTTPHandler(inventoryClient proto.InventoryServiceClient) *InventoryHTTPHandler {
	return &InventoryHTTPHandler{
		inventoryClient: inventoryClient,
	}
}

// Helper functions
func (s *InventoryHTTPHandler) success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
	})
}

func (s *InventoryHTTPHandler) error(c *gin.Context, code int, message string) {
	c.JSON(code, gin.H{
		"success": false,
		"error":   message,
	})
}

func parseIntParam(c *gin.Context, param string) (int32, error) {
	str := c.Param(param)
	val, err := strconv.ParseInt(str, 10, 32)
	return int32(val), err
}

func parseIntQuery(c *gin.Context, param string) *int32 {
	str := c.Query(param)
	if str == "" {
		return nil
	}
	val, err := strconv.ParseInt(str, 10, 32)
	if err != nil {
		return nil
	}
	result := int32(val)
	return &result
}

func parseInt64Query(c *gin.Context, param string) *int64 {
	str := c.Query(param)
	if str == "" {
		return nil
	}
	val, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return nil
	}
	return &val
}

func parseBoolQuery(c *gin.Context, param string) *bool {
	str := c.Query(param)
	if str == "" {
		return nil
	}
	val, err := strconv.ParseBool(str)
	if err != nil {
		return nil
	}
	return &val
}

func parseStringQuery(c *gin.Context, param string) *string {
	str := c.Query(param)
	if str == "" {
		return nil
	}
	return &str
}

func buildPaginationRequest(c *gin.Context) *proto.PaginationRequest {
	pageSize := c.DefaultQuery("page_size", "20")
	pageToken := c.Query("page_token")

	size, _ := strconv.ParseInt(pageSize, 10, 32)
	return &proto.PaginationRequest{
		PageSize:  int32(size),
		PageToken: pageToken,
	}
}

// Product endpoints
func (s *InventoryHTTPHandler) CreateProduct(c *gin.Context) {
	var req proto.CreateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.error(c, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	resp, err := s.inventoryClient.CreateProduct(c.Request.Context(), &req)
	if err != nil {
		s.error(c, http.StatusInternalServerError, "Failed to create product: "+err.Error())
		return
	}

	if !resp.Success {
		s.error(c, http.StatusBadRequest, *resp.Message)
		return
	}

	s.success(c, resp.Product)
}

func (s *InventoryHTTPHandler) UpdateProduct(c *gin.Context) {
	id, err := parseIntParam(c, "id")
	if err != nil {
		s.error(c, http.StatusBadRequest, "Invalid product ID")
		return
	}

	var req proto.UpdateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.error(c, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	req.Id = id

	resp, err := s.inventoryClient.UpdateProduct(c.Request.Context(), &req)
	if err != nil {
		s.error(c, http.StatusInternalServerError, "Failed to update product: "+err.Error())
		return
	}

	if !resp.Success {
		s.error(c, http.StatusBadRequest, *resp.Message)
		return
	}

	s.success(c, resp.Product)
}

func (s *InventoryHTTPHandler) ListProducts(c *gin.Context) {
	req := &proto.ListProductsRequest{
		Pagination:    buildPaginationRequest(c),
		IsActive:      parseBoolQuery(c, "is_active"),
		ProductTypeId: parseIntQuery(c, "product_type_id"),
		SupplierId:    parseIntQuery(c, "supplier_id"),
		SearchTerm:    parseStringQuery(c, "search"),
	}

	resp, err := s.inventoryClient.ListProducts(c.Request.Context(), req)
	if err != nil {
		s.error(c, http.StatusInternalServerError, "Failed to list products: "+err.Error())
		return
	}

	if !resp.Success {
		s.error(c, http.StatusInternalServerError, *resp.Message)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"data":       resp.Products,
		"pagination": resp.Pagination,
	})
}

func (s *InventoryHTTPHandler) GetProduct(c *gin.Context) {
	id, err := parseIntParam(c, "id")
	if err != nil {
		s.error(c, http.StatusBadRequest, "Invalid product ID")
		return
	}

	req := &proto.GetProductRequest{Id: id}
	resp, err := s.inventoryClient.GetProduct(c.Request.Context(), req)
	if err != nil {
		s.error(c, http.StatusInternalServerError, "Failed to get product: "+err.Error())
		return
	}

	if !resp.Success {
		s.error(c, http.StatusNotFound, *resp.Message)
		return
	}

	s.success(c, resp.Product)
}

func (s *InventoryHTTPHandler) GetProductByCode(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		s.error(c, http.StatusBadRequest, "Product code is required")
		return
	}

	req := &proto.GetProductByCodeRequest{ProductCode: code}
	resp, err := s.inventoryClient.GetProductByCode(c.Request.Context(), req)
	if err != nil {
		s.error(c, http.StatusInternalServerError, "Failed to get product: "+err.Error())
		return
	}

	if !resp.Success {
		s.error(c, http.StatusNotFound, *resp.Message)
		return
	}

	s.success(c, resp.Product)
}

// Stock endpoints
func (s *InventoryHTTPHandler) CheckStock(c *gin.Context) {
	var req proto.CheckStockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.error(c, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	resp, err := s.inventoryClient.CheckStock(c.Request.Context(), &req)
	if err != nil {
		s.error(c, http.StatusInternalServerError, "Failed to check stock: "+err.Error())
		return
	}

	if !resp.Success {
		s.error(c, http.StatusBadRequest, *resp.Message)
		return
	}

	s.success(c, gin.H{
		"is_available":             resp.IsAvailable,
		"total_available_quantity": resp.TotalAvailableQuantity,
		"stock_details":            resp.StockDetails,
	})
}

func (s *InventoryHTTPHandler) ReserveStock(c *gin.Context) {
	var req proto.ReserveStockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.error(c, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	resp, err := s.inventoryClient.ReserveStock(c.Request.Context(), &req)
	if err != nil {
		s.error(c, http.StatusInternalServerError, "Failed to reserve stock: "+err.Error())
		return
	}

	if !resp.Success {
		s.error(c, http.StatusBadRequest, *resp.Message)
		return
	}

	s.success(c, resp.UpdatedStock)
}

func (s *InventoryHTTPHandler) ReleaseStock(c *gin.Context) {
	var req proto.ReleaseStockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.error(c, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	resp, err := s.inventoryClient.ReleaseStock(c.Request.Context(), &req)
	if err != nil {
		s.error(c, http.StatusInternalServerError, "Failed to release stock: "+err.Error())
		return
	}

	if !resp.Success {
		s.error(c, http.StatusBadRequest, *resp.Message)
		return
	}

	s.success(c, resp.UpdatedStock)
}

func (s *InventoryHTTPHandler) UpdateStock(c *gin.Context) {
	var req proto.UpdateStockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.error(c, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	resp, err := s.inventoryClient.UpdateStock(c.Request.Context(), &req)
	if err != nil {
		s.error(c, http.StatusInternalServerError, "Failed to update stock: "+err.Error())
		return
	}

	if !resp.Success {
		s.error(c, http.StatusBadRequest, *resp.Message)
		return
	}

	s.success(c, gin.H{
		"stock_movement": resp.StockMovement,
		"updated_stock":  resp.UpdatedStock,
	})
}

func (s *InventoryHTTPHandler) TransferStock(c *gin.Context) {
	var req proto.TransferStockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.error(c, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	resp, err := s.inventoryClient.TransferStock(c.Request.Context(), &req)
	if err != nil {
		s.error(c, http.StatusInternalServerError, "Failed to transfer stock: "+err.Error())
		return
	}

	if !resp.Success {
		s.error(c, http.StatusBadRequest, *resp.Message)
		return
	}

	s.success(c, gin.H{
		"stock_movements":   resp.StockMovements,
		"source_stock":      resp.SourceStock,
		"destination_stock": resp.DestinationStock,
	})
}

func (s *InventoryHTTPHandler) GetStock(c *gin.Context) {
	productId := parseIntQuery(c, "product_id")
	if productId == nil {
		s.error(c, http.StatusBadRequest, "product_id is required")
		return
	}

	req := &proto.GetStockRequest{
		ProductId:   *productId,
		WarehouseId: parseIntQuery(c, "warehouse_id"),
	}

	resp, err := s.inventoryClient.GetStock(c.Request.Context(), req)
	if err != nil {
		s.error(c, http.StatusInternalServerError, "Failed to get stock: "+err.Error())
		return
	}

	if !resp.Success {
		s.error(c, http.StatusBadRequest, *resp.Message)
		return
	}

	s.success(c, resp.Stocks)
}

func (s *InventoryHTTPHandler) ListLowStock(c *gin.Context) {
	req := &proto.ListLowStockRequest{
		WarehouseId: parseIntQuery(c, "warehouse_id"),
		Pagination:  buildPaginationRequest(c),
	}

	resp, err := s.inventoryClient.ListLowStock(c.Request.Context(), req)
	if err != nil {
		s.error(c, http.StatusInternalServerError, "Failed to list low stock: "+err.Error())
		return
	}

	if !resp.Success {
		s.error(c, http.StatusInternalServerError, *resp.Message)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"data":       resp.LowStocks,
		"pagination": resp.Pagination,
	})
}

// Stock movement endpoints
func (s *InventoryHTTPHandler) ListStockMovements(c *gin.Context) {
	var dateRange *proto.DateRange
	if startDate := c.Query("start_date"); startDate != "" {
		dateRange = &proto.DateRange{StartDate: startDate}
		if endDate := c.Query("end_date"); endDate != "" {
			dateRange.EndDate = endDate
		}
	}

	var movementType *proto.MovementType
	if mtStr := c.Query("movement_type"); mtStr != "" {
		if mt, err := strconv.ParseInt(mtStr, 10, 32); err == nil {
			movType := proto.MovementType(mt)
			movementType = &movType
		}
	}

	req := &proto.ListStockMovementsRequest{
		Pagination:   buildPaginationRequest(c),
		ProductId:    parseIntQuery(c, "product_id"),
		WarehouseId:  parseIntQuery(c, "warehouse_id"),
		MovementType: movementType,
		DateRange:    dateRange,
	}

	resp, err := s.inventoryClient.ListStockMovements(c.Request.Context(), req)
	if err != nil {
		s.error(c, http.StatusInternalServerError, "Failed to list stock movements: "+err.Error())
		return
	}

	if !resp.Success {
		s.error(c, http.StatusInternalServerError, *resp.Message)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"data":       resp.StockMovements,
		"pagination": resp.Pagination,
	})
}

// Warehouse endpoints
func (s *InventoryHTTPHandler) CreateWarehouse(c *gin.Context) {
	var req proto.CreateWarehouseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.error(c, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	resp, err := s.inventoryClient.CreateWarehouse(c.Request.Context(), &req)
	if err != nil {
		s.error(c, http.StatusInternalServerError, "Failed to create warehouse: "+err.Error())
		return
	}

	if !resp.Success {
		s.error(c, http.StatusBadRequest, *resp.Message)
		return
	}

	s.success(c, resp.Warehouse)
}

func (s *InventoryHTTPHandler) ListWarehouses(c *gin.Context) {
	req := &proto.ListWarehousesRequest{
		Pagination: buildPaginationRequest(c),
		IsActive:   parseBoolQuery(c, "is_active"),
	}

	resp, err := s.inventoryClient.ListWarehouses(c.Request.Context(), req)
	if err != nil {
		s.error(c, http.StatusInternalServerError, "Failed to list warehouses: "+err.Error())
		return
	}

	if !resp.Success {
		s.error(c, http.StatusInternalServerError, *resp.Message)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"data":       resp.Warehouses,
		"pagination": resp.Pagination,
	})
}

func (s *InventoryHTTPHandler) GetWarehouse(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		s.error(c, http.StatusBadRequest, "Warehouse code is required")
		return
	}

	req := &proto.GetWarehouseRequest{WarehouseCode: code}
	resp, err := s.inventoryClient.GetWarehouse(c.Request.Context(), req)
	if err != nil {
		s.error(c, http.StatusInternalServerError, "Failed to get warehouse: "+err.Error())
		return
	}

	if !resp.Success {
		s.error(c, http.StatusNotFound, *resp.Message)
		return
	}

	s.success(c, resp.Warehouse)
}

// Supplier endpoints
func (s *InventoryHTTPHandler) CreateSupplier(c *gin.Context) {
	var req proto.CreateSupplierRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.error(c, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	resp, err := s.inventoryClient.CreateSupplier(c.Request.Context(), &req)
	if err != nil {
		s.error(c, http.StatusInternalServerError, "Failed to create supplier: "+err.Error())
		return
	}

	if !resp.Success {
		s.error(c, http.StatusBadRequest, *resp.Message)
		return
	}

	s.success(c, resp.Supplier)
}

func (s *InventoryHTTPHandler) ListSuppliers(c *gin.Context) {
	req := &proto.ListSuppliersRequest{
		Pagination: buildPaginationRequest(c),
		IsActive:   parseBoolQuery(c, "is_active"),
	}

	resp, err := s.inventoryClient.ListSuppliers(c.Request.Context(), req)
	if err != nil {
		s.error(c, http.StatusInternalServerError, "Failed to list suppliers: "+err.Error())
		return
	}

	if !resp.Success {
		s.error(c, http.StatusInternalServerError, *resp.Message)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"data":       resp.Suppliers,
		"pagination": resp.Pagination,
	})
}

func (s *InventoryHTTPHandler) GetSupplier(c *gin.Context) {
	id, err := parseIntParam(c, "id")
	if err != nil {
		s.error(c, http.StatusBadRequest, "Invalid supplier ID")
		return
	}

	req := &proto.GetSupplierRequest{Id: id}
	resp, err := s.inventoryClient.GetSupplier(c.Request.Context(), req)
	if err != nil {
		s.error(c, http.StatusInternalServerError, "Failed to get supplier: "+err.Error())
		return
	}

	if !resp.Success {
		s.error(c, http.StatusNotFound, *resp.Message)
		return
	}

	s.success(c, resp.Supplier)
}

// Product Type endpoints
func (s *InventoryHTTPHandler) CreateProductType(c *gin.Context) {
	var req proto.CreateProductTypeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.error(c, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	resp, err := s.inventoryClient.CreateProductType(c.Request.Context(), &req)
	if err != nil {
		s.error(c, http.StatusInternalServerError, "Failed to create product type: "+err.Error())
		return
	}

	if !resp.Success {
		s.error(c, http.StatusBadRequest, *resp.Message)
		return
	}

	s.success(c, resp.ProductType)
}

func (s *InventoryHTTPHandler) ListProductTypes(c *gin.Context) {
	req := &proto.ListProductTypesRequest{
		Pagination: buildPaginationRequest(c),
	}

	resp, err := s.inventoryClient.ListProductTypes(c.Request.Context(), req)
	if err != nil {
		s.error(c, http.StatusInternalServerError, "Failed to list product types: "+err.Error())
		return
	}

	if !resp.Success {
		s.error(c, http.StatusInternalServerError, *resp.Message)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"data":       resp.ProductTypes,
		"pagination": resp.Pagination,
	})
}
