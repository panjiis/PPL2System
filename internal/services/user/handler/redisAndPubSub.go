package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
	"gorm.io/gorm"
)

const (
	USER_CACHE_PREFIX       = "user:"
	USER_EMPLOYEE_CACHE_KEY = "user:employee"
	ROLE_CACHE_KEY          = "roles:list"
	CACHE_TTL_SHORT         = 5 * time.Minute
	CACHE_TTL_MEDIUM        = 30 * time.Minute
	CACHE_TTL_LONG          = 2 * time.Hour
)

// --- GORM Models ---
type User struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	Username  string `gorm:"uniqueIndex;not null"`
	Email     string `gorm:"uniqueIndex;not null"`
	Password  string `gorm:"not null"`
	Firstname string `gorm:"not null"`
	Lastname  string `gorm:"not null"`
	RoleID    int32  `gorm:"not null"`
	Role      Role   `gorm:"foreignKey:RoleID"`
	IsActive  bool   `gorm:"default:true"`
	LastLogin *time.Time
	CreatedAt *time.Time `gorm:"autoCreateTime"`
	UpdatedAt *time.Time `gorm:"autoUpdateTime"`
}

type Role struct {
	ID          int32      `gorm:"primaryKey;autoIncrement"`
	RoleName    string     `gorm:"uniqueIndex;not null"`
	AccessLevel int32      `gorm:"not null"`
	Permissions string     `gorm:"type:text"`
	CreatedAt   *time.Time `gorm:"autoCreateTime"`
	UpdatedAt   *time.Time `gorm:"autoUpdateTime"`
}

type Employee struct {
	ID             int64  `gorm:"primaryKey;autoIncrement"`
	EmployeeName   string `gorm:"not null"`
	Position       string `gorm:"column:position"`
	Phone          string
	Email          string
	Address        string `gorm:"type:text"`
	HireDate       string
	BaseSalary     string     `gorm:"not null"`
	CommissionRate string     `gorm:"not null"`
	CommissionType int32      `gorm:"not null"`
	IsActive       bool       `gorm:"default:true"`
	CreatedAt      *time.Time `gorm:"autoCreateTime"`
	UpdatedAt      *time.Time `gorm:"autoUpdateTime"`

	CommissionTiers []CommissionTier `gorm:"foreignKey:EmployeeID"`
}

type CommissionTier struct {
	ID             int32  `gorm:"primaryKey;autoIncrement"`
	EmployeeID     int64  `gorm:"not null"`
	MinSalesAmount string `gorm:"not null"`
	MaxSalesAmount string
	CommissionRate string     `gorm:"not null"`
	CreatedAt      *time.Time `gorm:"autoCreateTime"`
	UpdatedAt      *time.Time `gorm:"autoUpdateTime"`
}

type CommissionTierInfo struct {
	MinSalesAmount string `gorm:"column:min_sales_amount"`
	MaxSalesAmount string `gorm:"column:max_sales_amount"`
	CommissionRate string `gorm:"column:commission_rate"`
}

type Store struct {
	ID                    int64  `gorm:"primaryKey;autoIncrement:true" json:"id"`
	Name                  string `gorm:"not null" json:"name"`
	ImageURL              *string
	StorePreferences      *string
	ManagementPreferences *string
	Address               *string
	Phone                 *string
	City                  *string
	Country               *string
	PostalCode            *string
	IsActive              bool       `gorm:"default:true" json:"is_active"`
	CreatedAt             *time.Time `gorm:"autoCreateTime"`
	UpdatedAt             *time.Time `gorm:"autoUpdateTime"`
}

func (s *UserHandler) InvalidateUserCaches(ctx context.Context, userIDs ...int64) {
	_ = s.redis.Del(ctx, USER_EMPLOYEE_CACHE_KEY, ROLE_CACHE_KEY)

	for _, id := range userIDs {
		cacheKey := fmt.Sprintf("%s%d", USER_CACHE_PREFIX, id)
		_ = s.redis.Del(ctx, cacheKey)
	}
}

func (s *UserHandler) subscribeToNATSRequests() {
	s.nats.Subscribe("employee.get", func(msg *nats.Msg) {
		var req struct {
			EmployeeID int64 `json:"employee_id"`
		}

		if err := json.Unmarshal(msg.Data, &req); err != nil {
			log.Printf("Failed to parse employee.get request: %v", err)
			errorResp := map[string]interface{}{
				"found": false,
				"error": "Invalid request format",
			}
			respData, _ := json.Marshal(errorResp)
			msg.Respond(respData)
			return
		}

		var employee Employee
		result := s.db.First(&employee, req.EmployeeID)

		response := map[string]interface{}{
			"found": result.Error == nil,
		}

		if result.Error == nil {
			response["employee_name"] = employee.EmployeeName
			response["email"] = employee.Email
			response["position"] = employee.Position
		} else {
			response["error"] = "Employee not found"
		}

		respData, _ := json.Marshal(response)
		msg.Respond(respData)

		log.Printf("Handled employee.get request for ID %d", req.EmployeeID)
	})

	managerSub, err := s.nats.Subscribe("employee.manager.get", func(msg *nats.Msg) {
		var req struct {
			ManagerID int64 `json:"manager_id"`
		}

		if err := json.Unmarshal(msg.Data, &req); err != nil {
			log.Printf("Failed to parse employee.manager.get request: %v", err)
			errorResp := map[string]interface{}{
				"found": false,
				"error": "Invalid request format",
			}
			respData, _ := json.Marshal(errorResp)
			msg.Respond(respData)
			return
		}

		var manager User
		result := s.db.First(&manager, req.ManagerID)

		response := map[string]interface{}{
			"found": result.Error == nil,
		}

		if result.Error == nil {
			response["manager_name"] = manager.Firstname + " " + manager.Lastname
			response["email"] = manager.Email
		} else {
			response["error"] = "Employee not found"
		}

		respData, _ := json.Marshal(response)
		msg.Respond(respData)

		log.Printf("Handled employee.manager.get request for ID %d", req.ManagerID)
	})

	if err != nil {
		log.Printf("❌ Failed to subscribe to employee.manager.get: %v ", err)
		log.Print(managerSub)
	} else {
		log.Println("✅ Subscribed to employee.manager.get")
	}

	s.nats.Subscribe("employee.get-commission-tier", func(msg *nats.Msg) {
		var req struct {
			EmployeeID int64 `json:"employee_id"`
		}

		if err := json.Unmarshal(msg.Data, &req); err != nil {
			log.Printf("Failed to parse employee.get-commission-tier request: %v", err)
			respData, _ := json.Marshal(map[string]interface{}{
				"found": false,
				"error": "Invalid request format",
			})
			_ = msg.Respond(respData)
			return
		}

		var employee Employee
		err := s.db.WithContext(context.Background()).First(&employee, req.EmployeeID).Error

		resp := map[string]interface{}{
			"found": err == nil,
		}

		if err == nil {
			resp["employee_name"] = employee.EmployeeName
			resp["email"] = employee.Email
			resp["position"] = employee.Position
			resp["commission_type"] = employee.CommissionType
			resp["commission_rate"] = employee.CommissionRate

			if employee.CommissionType == 3 {
				var tiers []CommissionTierInfo
				if err := s.db.WithContext(context.Background()).
					Table("user.commission_tiers").
					Where("employee_id = ?", req.EmployeeID).
					Order("min_sales_amount asc").
					Find(&tiers).Error; err != nil {
					resp["error"] = "Failed to fetch commission tiers"
				} else {
					resp["tiers"] = tiers
				}
			}
		} else if err == gorm.ErrRecordNotFound {
			resp["error"] = "Employee not found"
		} else {
			resp["error"] = "Database error"
			log.Printf("Database error fetching employee %d: %v", req.EmployeeID, err)
		}

		respData, _ := json.Marshal(resp)
		_ = msg.Respond(respData)
		log.Printf("Handled employee.get-commission-tier request for ID %d", req.EmployeeID)
	})

	log.Println("✅ User service subscribed to employee.get")
}
