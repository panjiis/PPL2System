package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"syntra-system/internal/gateway/clients"
	"syntra-system/internal/gateway/handlers"
	"syntra-system/internal/gateway/middleware"

	"github.com/gin-gonic/gin"
)

func main() {
	grpcClients, err := clients.NewGRPCClientsWithFallback()
	if err != nil {
		log.Printf("Warning: Some gRPC services may be unavailable: %v", err)
	}
	defer grpcClients.Close()

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

	// var posHandler *handlers.POSHTTPHandler
	// if grpcClients.POS != nil {
	// 	posHandler = handlers.NewPOSHTTPHandler(grpcClients.POS)
	// }

	// var commissionsHandler *handlers.CommissionsHTTPHandler
	// if grpcClients.Commissions != nil {
	// 	commissionsHandler = handlers.NewCommissionsHTTPHandler(grpcClients.Commissions)
	// }

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
			} else {
				employees.POST("", serviceUnavailableHandler("User service"))
				employees.GET("", serviceUnavailableHandler("User service"))
				employees.GET("/:id", serviceUnavailableHandler("User service"))
				employees.PUT("/:id", serviceUnavailableHandler("User service"))
			}
		}

		roles := protected.Group("/roles")
		{
			if userHandler != nil {
				roles.POST("", userHandler.CreateRole)
				roles.GET("", userHandler.ListRoles)
			} else {
				roles.POST("", serviceUnavailableHandler("User service"))
				roles.GET("", serviceUnavailableHandler("User service"))
			}
		}

		inventoryGroup := protected.Group("/inventory")
		{
			if inventoryHandler != nil {
				// Product routes
				inventoryGroup.POST("/products", inventoryHandler.CreateProduct)
				inventoryGroup.GET("/products", inventoryHandler.ListProducts)
				inventoryGroup.GET("/products/:id", inventoryHandler.GetProduct)
				inventoryGroup.GET("/products/code/:code", inventoryHandler.GetProductByCode)
				inventoryGroup.PUT("/products/:id", inventoryHandler.UpdateProduct)

				// Stock routes
				inventoryGroup.POST("/stocks/check", inventoryHandler.CheckStock)
				inventoryGroup.POST("/stocks/reserve", inventoryHandler.ReserveStock)
				inventoryGroup.POST("/stocks/release", inventoryHandler.ReleaseStock)
				inventoryGroup.POST("/stocks/update", inventoryHandler.UpdateStock)
				inventoryGroup.POST("/stocks/transfer", inventoryHandler.TransferStock)
				inventoryGroup.GET("/stocks", inventoryHandler.GetStock)
				inventoryGroup.GET("/stocks/low", inventoryHandler.ListLowStock)

				// Stock movement routes
				inventoryGroup.GET("/movements", inventoryHandler.ListStockMovements)

				// Warehouse routes
				inventoryGroup.POST("/warehouses", inventoryHandler.CreateWarehouse)
				inventoryGroup.GET("/warehouses", inventoryHandler.ListWarehouses)
				inventoryGroup.GET("/warehouses/:code", inventoryHandler.GetWarehouse)

				// Supplier routes
				inventoryGroup.POST("/suppliers", inventoryHandler.CreateSupplier)
				inventoryGroup.GET("/suppliers", inventoryHandler.ListSuppliers)
				inventoryGroup.GET("/suppliers/:id", inventoryHandler.GetSupplier)

				// Product Type routes
				inventoryGroup.POST("/product-types", inventoryHandler.CreateProductType)
				inventoryGroup.GET("/product-types", inventoryHandler.ListProductTypes)

			} else {
				// Product routes
				inventoryGroup.POST("/products", serviceUnavailableHandler("Inventory service"))
				inventoryGroup.GET("/products", serviceUnavailableHandler("Inventory service"))
				inventoryGroup.GET("/products/:id", serviceUnavailableHandler("Inventory service"))
				inventoryGroup.GET("/products/code/:code", serviceUnavailableHandler("Inventory service"))
				inventoryGroup.PUT("/products/:id", serviceUnavailableHandler("Inventory service"))
				inventoryGroup.DELETE("/products/:id", serviceUnavailableHandler("Inventory service"))

				// Stock routes
				inventoryGroup.POST("/stocks/check", serviceUnavailableHandler("Inventory service"))
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

		// posGroup := protected.Group("/pos")
		// {
		// 	if posHandler != nil {
		// 		posGroup.POST("/sales", posHandler.CreateSale)
		// 		posGroup.GET("/sales", posHandler.ListSales)
		// 		posGroup.GET("/sales/:id", posHandler.GetSale)
		// 	} else {
		// 		posGroup.POST("/sales", serviceUnavailableHandler("POS service"))
		// 		posGroup.GET("/sales", serviceUnavailableHandler("POS service"))
		// 		posGroup.GET("/sales/:id", serviceUnavailableHandler("POS service"))
		// 	}
		// }

		// commissionsGroup := protected.Group("/commissions")
		// {
		// 	if commissionsHandler != nil {
		// 		commissionsGroup.POST("", commissionsHandler.CalculateCommission)
		// 		commissionsGroup.GET("", commissionsHandler.ListCommissions)
		// 	} else {
		// 		commissionsGroup.POST("", serviceUnavailableHandler("Commissions service"))
		// 		commissionsGroup.GET("", serviceUnavailableHandler("Commissions service"))
		// 	}
		// }
	}

	r.GET("/health", healthCheckHandler(grpcClients))
	r.GET("/health/detailed", detailedHealthCheckHandler(grpcClients))

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
