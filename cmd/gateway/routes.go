package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"syntra-system/internal/gateway/clients"
	"syntra-system/internal/gateway/handlers"
	"syntra-system/internal/gateway/middleware"
	proto "syntra-system/proto/protogen/analytics"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func startScheduler(analyticsClient proto.AnalyticsServiceClient) {
	go func() {
		loc, err := time.LoadLocation("Asia/Jakarta")
		if err != nil {
			log.Printf("⚠️ Gagal load lokasi Asia/Jakarta, fallback ke Local: %v", err)
			loc = time.Local
		}

		for {
			// nextRun := time.Now().In(loc).Add(10 * time.Second) // Mode Test
			
			now := time.Now().In(loc)
			nextRun := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 1, 0, 0, loc) // Mode Production
			// ---------------------------------------------------------

			duration := nextRun.Sub(now)
			log.Printf("⏳ Scheduler sleeping for %v until next run at %v...", duration, nextRun)

			time.Sleep(duration)

			// --- STARTING BATCH PROCESS ---
			log.Println("🚀 Starting Nightly Batch Jobs...")

			waktuBangun := time.Now().In(loc)
			targetDate := waktuBangun.AddDate(0, 0, -1).Format("2006-01-02")
			
			log.Printf("📅 Processing data for date: %s", targetDate)

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			
			// ==========================================
			// JOB 1: Generate Daily Summary 
			// ==========================================
			log.Println("▶️ [1/3] Running GenerateDailySummary...")
			resp1, err := analyticsClient.GenerateDailySummary(ctx, &proto.GenerateDailySummaryRequest{
					Date: targetDate,
			})
			if err != nil {
					log.Printf("❌ [1/3] Failed: %v", err)
			} else {
					log.Printf("✅ [1/3] Success: %s", resp1.GetMessage())
			}

			// ==========================================
			// JOB 2: Generate Product Sales Summary 
			// ==========================================
			log.Println("▶️ [2/3] Running GenerateProductSalesSummary...")
			// Kita kosongkan ProductCode & GroupId agar Python memproses SEMUA produk
			resp2, err := analyticsClient.GenerateProductSalesSummary(ctx, &proto.GenerateProductSalesSummaryRequest{
					Date: targetDate, 
			})
			if err != nil {
					log.Printf("❌ [2/3] Failed: %v", err)
			} else {
					log.Printf("✅ [2/3] Success: %s", resp2.GetMessage())
			}

			// ==========================================
			// JOB 3: Generate Customer Analytics 
			// ==========================================
			log.Println("▶️ [3/3] Running GenerateCustomerAnalytics...")
			resp3, err := analyticsClient.GenerateCustomerAnalytics(ctx, &proto.GenerateCustomerAnalyticsRequest{
					Date: targetDate,
			})
			if err != nil {
					log.Printf("❌ [3/3] Failed: %v", err)
			} else {
					log.Printf("✅ [3/3] Success: %s", resp3.GetMessage())
			}

			log.Println("🏁 Nightly Batch Jobs Finished.")
			cancel()
		}
	}()
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Printf("Warning: Error loading .env file: %v", err)
		// Continue with defaults or environment variables from system
	}

	grpcClients, err := clients.NewGRPCClientsWithFallback()
	if err != nil {
		log.Printf("Warning: Some gRPC services may be unavailable: %v", err)
		if grpcClients == nil {
			log.Fatal("All gRPC services are unavailable. Cannot start server.")
		}
	}
	defer func() {
		if grpcClients != nil {
			grpcClients.Close()
		}
	}()

	r := gin.Default()

	r.Use(middleware.CORS())
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.Use(middleware.RateLimit())
	r.Use(serviceHealthMiddleware(grpcClients))

	var userHandler *handlers.UserHTTPHandler
	if grpcClients.User != nil {
		userHandler = handlers.NewUserHTTPHandler(grpcClients.User)
	}

	var inventoryHandler *handlers.InventoryHTTPHandler
	if grpcClients.Inventory != nil {
		inventoryHandler = handlers.NewInventoryHTTPHandler(grpcClients.Inventory)
	}

	var posHandler *handlers.POSHTTPHandler
	if grpcClients.POS != nil {
		posHandler = handlers.NewPOSHTTPHandler(grpcClients.POS)
	}

	var commissionsHandler *handlers.CommissionsHTTPHandler
	if grpcClients.Commissions != nil {
		commissionsHandler = handlers.NewCommissionsHTTPHandler(grpcClients.Commissions)
	}

	var analyticsHandler *handlers.AnalyticsHTTPHandler
	if grpcClients.Analytics != nil {
		analyticsHandler = handlers.NewAnalyticsHTTPHandler(grpcClients.Analytics)
	}

	// --- Public API Group ---
	public := r.Group("/api/v1")
	{
		auth := public.Group("/auth")
		{
			if userHandler != nil {
				auth.POST("/login", userHandler.Login)
				auth.POST("/register", userHandler.Register)
			} else {
				auth.POST("/login", serviceUnavailableHandler("User service"))
				auth.POST("/register", serviceUnavailableHandler("User service"))
			}
		}
	}

	// --- Protected API Group ---
	protected := r.Group("/api/v1")
	protected.Use(middleware.JWTAuth())
	{
		users := protected.Group("/users")
		{
			if userHandler != nil {
				users.GET("", userHandler.ListUsers)
				users.GET("/:id", userHandler.GetUser)
				users.PUT("/:id", userHandler.UpdateUser)
			} else {
				users.GET("", serviceUnavailableHandler("User service"))
				users.GET("/:id", serviceUnavailableHandler("User service"))
				users.PUT("/:id", serviceUnavailableHandler("User service"))
			}
		}

		employees := protected.Group("/employees")
		{
			if userHandler != nil {
				employees.POST("", userHandler.CreateEmployee)
				employees.GET("", userHandler.ListEmployees)
				employees.GET("/:id", userHandler.GetEmployee)
				employees.PUT("/:id", userHandler.UpdateEmployee)

				employees.GET("/:id/commission-summary", commissionsHandler.GetCommissionSummary)
				employees.GET("/:id/commission-settings", commissionsHandler.GetCommissionSettings)
			} else {
				employees.POST("", serviceUnavailableHandler("User service"))
				employees.GET("", serviceUnavailableHandler("User service"))
				employees.GET("/:id", serviceUnavailableHandler("User service"))
				employees.PUT("/:id", serviceUnavailableHandler("User service"))

				employees.GET("/:id/commission-summary", serviceUnavailableHandler("Commission service"))
				employees.PUT("/:id/commission-settings", serviceUnavailableHandler("Commission service"))
			}
		}

		roles := protected.Group("/roles")
		{
			if userHandler != nil {
				roles.POST("", userHandler.CreateRole)
				roles.GET("", userHandler.ListRoles)
				roles.PUT("/:id", userHandler.UpdateRole)
				roles.GET("/:id", userHandler.GetRole)
			} else {
				roles.POST("", serviceUnavailableHandler("User service"))
				roles.GET("", serviceUnavailableHandler("User service"))
			}
		}

		stores := protected.Group("/store")
		{
			if userHandler != nil {
				stores.POST("", userHandler.CreateStore)
				stores.GET("", userHandler.ListStores)
				stores.PUT("/:id", userHandler.UpdateStore)
				stores.GET("/:id", userHandler.GetStore)
			} else {
				stores.POST("", serviceUnavailableHandler("User service"))
				stores.GET("", serviceUnavailableHandler("User service"))
			}
		}

		inventoryGroup := protected.Group("/inventory")
		{
			if inventoryHandler != nil {
				// Product routes
				inventoryGroup.POST("/products", inventoryHandler.CreateProduct)
				inventoryGroup.GET("/products", inventoryHandler.ListProducts)
				inventoryGroup.GET("/products/:code", inventoryHandler.GetProduct)
				inventoryGroup.PUT("/products/:code", inventoryHandler.UpdateProduct)

				// Stock routes
				inventoryGroup.GET("/stocks", inventoryHandler.ListStocks)
				inventoryGroup.POST("/stocks/:productCode", inventoryHandler.ListStocks)
				inventoryGroup.POST("/stocks/:productCode/:warehouseId", inventoryHandler.ListStocks)
				inventoryGroup.POST("/stocks/reserve", inventoryHandler.ReserveStock)
				inventoryGroup.POST("/stocks/release", inventoryHandler.ReleaseStock)
				inventoryGroup.POST("/stocks/update", inventoryHandler.UpdateStock)
				inventoryGroup.POST("/stocks/transfer", inventoryHandler.TransferStock)
				inventoryGroup.GET("/stocks/low", inventoryHandler.ListLowStock)

				// Stock movement routes
				inventoryGroup.GET("/movements", inventoryHandler.ListStockMovements)

				// Warehouse routes
				inventoryGroup.POST("/warehouses", inventoryHandler.CreateWarehouse)
				inventoryGroup.PUT("/warehouses/:code", inventoryHandler.UpdateWarehouse)
				inventoryGroup.PUT("/warehouses/status/:code", inventoryHandler.UpdateWarehouseStatus)
				inventoryGroup.GET("/warehouses", inventoryHandler.ListWarehouses)
				inventoryGroup.GET("/warehouses/:code", inventoryHandler.GetWarehouse)

				// Supplier routes
				inventoryGroup.POST("/suppliers", inventoryHandler.CreateSupplier)
				inventoryGroup.PUT("/suppliers/:code", inventoryHandler.UpdateSupplier)
				inventoryGroup.PUT("/suppliers/status/:code", inventoryHandler.UpdateSupplierStatus)
				inventoryGroup.GET("/suppliers", inventoryHandler.ListSuppliers)
				inventoryGroup.GET("/suppliers/:id", inventoryHandler.GetSupplier)

				// Product Type routes
				inventoryGroup.POST("/product-types", inventoryHandler.CreateProductType)
				inventoryGroup.PUT("/product-types/:id", inventoryHandler.UpdateProductType)
				inventoryGroup.GET("/product-types/:id", inventoryHandler.GetProductType)
				inventoryGroup.GET("/product-types", inventoryHandler.ListProductTypes)
				inventoryGroup.GET("/product-types/:id/products", inventoryHandler.ListProductByProductType)

			} else {
				// Product routes
				inventoryGroup.POST("/products", serviceUnavailableHandler("Inventory service"))
				inventoryGroup.GET("/products", serviceUnavailableHandler("Inventory service"))
				inventoryGroup.GET("/products/:code", serviceUnavailableHandler("Inventory service"))
				inventoryGroup.PUT("/products/:code", serviceUnavailableHandler("Inventory service"))
				inventoryGroup.DELETE("/products/:id", serviceUnavailableHandler("Inventory service"))

				// Stock routes
				inventoryGroup.GET("/stocks", serviceUnavailableHandler("Inventory service"))
				inventoryGroup.POST("/stocks/reserve", serviceUnavailableHandler("Inventory service"))
				inventoryGroup.POST("/stocks/release", serviceUnavailableHandler("Inventory service"))
				inventoryGroup.POST("/stocks/update", serviceUnavailableHandler("Inventory service"))
				inventoryGroup.POST("/stocks/transfer", serviceUnavailableHandler("Inventory service"))
				inventoryGroup.GET("/stocks", serviceUnavailableHandler("Inventory service"))
				inventoryGroup.GET("/stocks/low", serviceUnavailableHandler("Inventory service"))

				// Stock movement routes
				inventoryGroup.GET("/movements", serviceUnavailableHandler("Inventory service"))

				// Warehouse routes
				inventoryGroup.POST("/warehouses", serviceUnavailableHandler("Inventory service"))
				inventoryGroup.GET("/warehouses", serviceUnavailableHandler("Inventory service"))
				inventoryGroup.GET("/warehouses/:code", serviceUnavailableHandler("Inventory service"))
				inventoryGroup.DELETE("/warehouses/:code", serviceUnavailableHandler("Inventory service"))

				// Supplier routes
				inventoryGroup.POST("/suppliers", serviceUnavailableHandler("Inventory service"))
				inventoryGroup.GET("/suppliers", serviceUnavailableHandler("Inventory service"))
				inventoryGroup.GET("/suppliers/:id", serviceUnavailableHandler("Inventory service"))
				inventoryGroup.DELETE("/suppliers/:id", serviceUnavailableHandler("Inventory service"))

				// Product Type routes
				inventoryGroup.POST("/product-types", serviceUnavailableHandler("Inventory service"))
				inventoryGroup.GET("/product-types", serviceUnavailableHandler("Inventory service"))
				inventoryGroup.DELETE("/product-types/:id", serviceUnavailableHandler("Inventory service"))
			}
		}

		posGroup := protected.Group("/pos")
		{
			if posHandler != nil {
				// Products
				posGroup.POST("/products", posHandler.CreateProduct)
				posGroup.PUT("/products/:code", posHandler.UpdateProduct)
				posGroup.GET("/products", posHandler.ListProducts)
				posGroup.GET("/products/:code", posHandler.GetProduct)
				// Product Groups
				posGroup.POST("/product-groups", posHandler.CreateProductGroupHandler)
				posGroup.PUT("/product-groups/:id", posHandler.UpdateProductGroupHandler)
				posGroup.GET("/product-groups/:id", posHandler.GetProductGroupHandler)
				posGroup.GET("/product-groups", posHandler.ListProductGroups)

				// Payment Types
				posGroup.POST("/payment-types", posHandler.CreatePaymentTypes)
				posGroup.PUT("/payment-types/:id", posHandler.UpdatePaymentType)
				posGroup.GET("/payment-types", posHandler.ListPaymentTypes)

				// Payment Processing
				posGroup.POST("/payments/process", posHandler.ProcessPayment)

				// Discounts
				posGroup.POST("/discounts", posHandler.CreateDiscount)
				posGroup.GET("/discounts", posHandler.ListDiscounts)
				posGroup.GET("/discounts/:id", posHandler.GetDiscount)
				posGroup.PUT("/discounts/:id", posHandler.UpdateDiscount)
				posGroup.DELETE("/discounts/:id", posHandler.DeleteDiscount)
				posGroup.POST("/discounts/validate", posHandler.ValidateDiscount)

				// Carts
				posGroup.POST("/carts", posHandler.CreateCart)
				posGroup.GET("/carts/:id", posHandler.GetCart)
				posGroup.POST("/carts/items", posHandler.AddItemToCart)
				posGroup.DELETE("/carts/:cart_id/items/:item_id", posHandler.RemoveItemFromCart)
				posGroup.POST("/carts/discounts", posHandler.ApplyDiscount)

				// Orders
				posGroup.POST("/orders", posHandler.CreateOrder)
				posGroup.POST("/orders/from-cart", posHandler.CreateOrderFromCart)
				posGroup.GET("/orders", posHandler.ListOrders)
				posGroup.GET("/orders/:id", posHandler.GetOrder)
				posGroup.POST("/orders/void", posHandler.VoidOrder)
				posGroup.POST("/orders/return", posHandler.ReturnOrder)
			} else {
				posGroup.GET("/*any", serviceUnavailableHandler("POS service"))
				posGroup.POST("/*any", serviceUnavailableHandler("POS service"))
				posGroup.PUT("/*any", serviceUnavailableHandler("POS service"))
				posGroup.DELETE("/*any", serviceUnavailableHandler("POS service"))
			}
		}

		commissionsGroup := protected.Group("/commissions")
		{
			if commissionsHandler != nil {
				// --- Calculation ---
				commissionsGroup.POST("", commissionsHandler.CalculateCommission)
				commissionsGroup.POST("/:id/recalculate", commissionsHandler.RecalculateCommission)
				commissionsGroup.POST("/bulk-calculate", commissionsHandler.BulkCalculateCommissions)

				// --- Management ---
				commissionsGroup.GET("", commissionsHandler.ListCommissionCalculations)
				commissionsGroup.GET("/:id", commissionsHandler.GetCommissionCalculation)
				commissionsGroup.POST("/:id/approve", commissionsHandler.ApproveCommission)
				commissionsGroup.POST("/:id/reject", commissionsHandler.RejectCommission)
				commissionsGroup.POST("/bulk-approve", commissionsHandler.BulkApproveCommissions)

				// --- Payment ---
				commissionsGroup.POST("/:id/pay", commissionsHandler.PayCommission)
				commissionsGroup.GET("/:id/payment", commissionsHandler.GetCommissionPayment)

				// --- Reporting ---
				commissionsGroup.GET("/report", commissionsHandler.GetCommissionReport)
			} else {
				commissionsGroup.POST("", serviceUnavailableHandler("Commissions service"))
				commissionsGroup.GET("", serviceUnavailableHandler("Commissions service"))
			}
		}

		analyticsGroup := protected.Group("/analytics")
		{
			if analyticsHandler != nil {
				// sales
				analyticsGroup.GET("/reports/sales", analyticsHandler.GetSalesReport)
				analyticsGroup.GET("/reports/daily-summary", analyticsHandler.GetDailySummary)
				analyticsGroup.POST("/reports/daily-summary/generate", analyticsHandler.GenerateDailySummary)

				// products
				analyticsGroup.GET("/products/sales", analyticsHandler.GetProductSales)
				analyticsGroup.GET("/products/top-selling", analyticsHandler.GetTopSellingProducts)
				analyticsGroup.POST("/products/sales/generate", analyticsHandler.GenerateProductSalesSummary)

				// employees
				analyticsGroup.GET("/employees/performance", analyticsHandler.GetEmployeePerformance)
				analyticsGroup.GET("/performance/report", analyticsHandler.GetPerformanceReport)
				analyticsGroup.POST("/employees/performance/generate", analyticsHandler.GenerateEmployeePerformance)
				
				// customers
				analyticsGroup.GET("/customers/analytics", analyticsHandler.GetCustomerAnalytics)
				analyticsGroup.GET("/customers/peak-hours", analyticsHandler.GetPeakHours)
				analyticsGroup.POST("/customers/analytics/generate", analyticsHandler.GenerateCustomerAnalytics)
				
				// dashboard
				analyticsGroup.GET("/dashboard", analyticsHandler.GetDashboardData)
				analyticsGroup.GET("/real-time-metrics", analyticsHandler.GetRealTimeMetrics)
			} else {
				// Fallback jika service analytics tidak tersedia
				analyticsGroup.GET("/*any", serviceUnavailableHandler("Analytics service"))
				analyticsGroup.POST("/*any", serviceUnavailableHandler("Analytics service"))
			}
		}
	}

	r.GET("/health", healthCheckHandler(grpcClients))
	r.GET("/health/detailed", detailedHealthCheckHandler(grpcClients))

	if grpcClients.Analytics != nil {
		log.Println("🕒 Starting Daily Summary Scheduler...")
		startScheduler(grpcClients.Analytics)
	} else {
		log.Println("⚠️ Analytics Service unavailable. Scheduler will not start.")
	}

	port := ":8080"
	log.Printf("Starting server on port %s", port)
	if err := r.Run(port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func serviceUnavailableHandler(serviceName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": serviceName + " is currently unavailable",
			"error":   "SERVICE_UNAVAILABLE",
		})
	}
}

func serviceHealthMiddleware(clients *clients.GRPCClients) gin.HandlerFunc {
	return func(c *gin.Context) {
		if clients.User != nil {
			c.Header("X-User-Service", "available")
		} else {
			c.Header("X-User-Service", "unavailable")
		}
		if clients.Inventory != nil {
			c.Header("X-Inventory-Service", "available")
		} else {
			c.Header("X-Inventory-Service", "unavailable")
		}
		if clients.POS != nil {
			c.Header("X-POS-Service", "available")
		} else {
			c.Header("X-POS-Service", "unavailable")
		}
		if clients.Commissions != nil {
			c.Header("X-Commissions-Service", "available")
		} else {
			c.Header("X-Commissions-Service", "unavailable")
		}
		if clients.Analytics != nil {
			c.Header("X-Analytics-Service", "available")
		} else {
			c.Header("X-Analytics-Service", "unavailable")
		}
		c.Next()
	}
}

func healthCheckHandler(clients *clients.GRPCClients) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := "healthy"
		httpStatus := http.StatusOK

		unavailableServices := []string{}
		if clients.User == nil {
			unavailableServices = append(unavailableServices, "user")
		}
		if clients.Inventory == nil {
			unavailableServices = append(unavailableServices, "inventory")
		}
		if clients.POS == nil {
			unavailableServices = append(unavailableServices, "pos")
		}
		if clients.Commissions == nil {
			unavailableServices = append(unavailableServices, "commissions")
		}
		if clients.Analytics == nil {
			unavailableServices = append(unavailableServices, "analytics")
		}

		if len(unavailableServices) > 0 {
			status = "degraded"
			httpStatus = http.StatusPartialContent
		}

		c.JSON(httpStatus, gin.H{
			"status":               status,
			"message":              "Server is running",
			"unavailable_services": unavailableServices,
			"timestamp":            time.Now(),
		})
	}
}

func detailedHealthCheckHandler(clients *clients.GRPCClients) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		services := map[string]interface{}{
			"user":        checkServiceHealth(ctx, clients.IsUserServiceHealthy()),
			"inventory":   checkServiceHealth(ctx, clients.IsInventoryServiceHealthy()),
			"pos":         checkServiceHealth(ctx, clients.IsPOSServiceHealthy()),
			"commissions": checkServiceHealth(ctx, clients.IsCommissionsServiceHealthy()),
			"analytics":   checkServiceHealth(ctx, clients.IsAnalyticsServiceHealthy()),
		}

		overallStatus := "healthy"
		for _, service := range services {
			if serviceMap, ok := service.(map[string]interface{}); ok {
				if serviceMap["status"] != "healthy" {
					overallStatus = "degraded"
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"overall_status": overallStatus,
			"services":       services,
			"timestamp":      time.Now(),
		})
	}
}

func checkServiceHealth(ctx context.Context, isHealthy bool) map[string]interface{} {
	if !isHealthy {
		return map[string]interface{}{
			"status":  "unavailable",
			"message": "Service client not initialized or connection lost",
		}
	}
	return map[string]interface{}{
		"status":  "healthy",
		"message": "Service is responding",
	}
}
