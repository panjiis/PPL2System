package handler

import (
	"log"

	"github.com/go-redis/redis/v8"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"

	lib "syntra-system/internal/utils"
	proto "syntra-system/proto/protogen/user"
)

type UserHandler struct {
	proto.UnimplementedUserServiceServer
	db    *gorm.DB
	redis *redis.Client
	nats  *nats.Conn
}

func NewUserHandler(db *gorm.DB, redisClient *redis.Client) *UserHandler {
	nc, err := nats.Connect("nats://10.147.17.76:4222",
		nats.ConnectHandler(func(nc *nats.Conn) {
			log.Printf("User service connected to NATS at %v", nc.ConnectedUrl())
		}),
		nats.DisconnectHandler(func(nc *nats.Conn) {
			log.Printf("User service disconnected from NATS")
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Printf("User service reconnected to NATS at %v", nc.ConnectedUrl())
		}),
	)
	if err != nil {
		log.Fatal("Failed to connect to NATS:", err)
	}

	handler := &UserHandler{
		db:    db,
		redis: redisClient,
		nats:  nc,
	}

	handler.subscribeToNATSRequests()

	return handler
}

// --- Conversion Helpers ---
func (s *UserHandler) roleToProto(role Role) *proto.Role {
	return &proto.Role{
		Id:          role.ID,
		RoleName:    role.RoleName,
		AccessLevel: role.AccessLevel,
		Permissions: lib.StrPtr(role.Permissions),
		CreatedAt:   timestamppb.New(lib.TimeNowOrZero(role.CreatedAt)),
		UpdatedAt:   timestamppb.New(lib.TimeNowOrZero(role.UpdatedAt)),
	}
}

func (s *UserHandler) userToProto(user User) *proto.User {
	var roleProto *proto.Role
	if user.Role.ID != 0 {
		roleProto = s.roleToProto(user.Role)
	}

	var lastLoginProto *timestamppb.Timestamp
	if user.LastLogin != nil {
		lastLoginProto = timestamppb.New(*user.LastLogin)
	}

	return &proto.User{
		Id:        user.ID,
		Username:  user.Username,
		Email:     user.Email,
		Password:  "",
		Firstname: user.Firstname,
		Lastname:  user.Lastname,
		RoleId:    user.RoleID,
		IsActive:  user.IsActive,
		LastLogin: lastLoginProto,
		CreatedAt: timestamppb.New(lib.TimeNowOrZero(user.CreatedAt)),
		UpdatedAt: timestamppb.New(lib.TimeNowOrZero(user.UpdatedAt)),
		Role:      roleProto,
	}
}

func (s *UserHandler) employeeToProto(employee Employee) *proto.Employee {
	var commissionTiers []*proto.CommissionTier
	for _, tier := range employee.CommissionTiers {
		commissionTiers = append(commissionTiers, &proto.CommissionTier{
			Id:             tier.ID,
			EmployeeId:     tier.EmployeeID,
			MinSalesAmount: tier.MinSalesAmount,
			MaxSalesAmount: lib.StrPtr(tier.MaxSalesAmount),
			CommissionRate: tier.CommissionRate,
			CreatedAt:      timestamppb.New(lib.TimeNowOrZero(tier.CreatedAt)),
			UpdatedAt:      timestamppb.New(lib.TimeNowOrZero(tier.UpdatedAt)),
		})
	}

	return &proto.Employee{
		Id:              employee.ID,
		EmployeeName:    employee.EmployeeName,
		Position:        lib.StrPtr(employee.Position),
		Phone:           lib.StrPtr(employee.Phone),
		Email:           lib.StrPtr(employee.Email),
		Address:         lib.StrPtr(employee.Address),
		HireDate:        lib.StrPtr(employee.HireDate),
		BaseSalary:      employee.BaseSalary,
		CommissionRate:  employee.CommissionRate,
		CommissionType:  proto.CommissionType(employee.CommissionType),
		IsActive:        employee.IsActive,
		CreatedAt:       timestamppb.New(lib.TimeNowOrZero(employee.CreatedAt)),
		UpdatedAt:       timestamppb.New(lib.TimeNowOrZero(employee.UpdatedAt)),
		CommissionTiers: commissionTiers,
	}
}
func (s *UserHandler) storeToProto(store Store) *proto.Store {
	return &proto.Store{
		Id:                    store.ID,
		Name:                  store.Name,
		ImageUrl:              store.ImageURL,
		StorePreferences:      store.StorePreferences,
		ManagementPreferences: store.ManagementPreferences,
		Address:               store.Address,
		Phone:                 store.Phone,
		City:                  store.City,
		Country:               store.Country,
		PostalCode:            store.PostalCode,
		IsActive:              store.IsActive,
		CreatedAt:             timestamppb.New(lib.TimeNowOrZero(store.CreatedAt)),
		UpdatedAt:             timestamppb.New(lib.TimeNowOrZero(store.UpdatedAt)),
	}
}
