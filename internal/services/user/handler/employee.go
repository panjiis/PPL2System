package handler

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	proto "syntra-system/proto/protogen/user"
	"time"

	"gorm.io/gorm"
)

func (s *UserHandler) CreateEmployee(ctx context.Context, req *proto.CreateEmployeeRequest) (*proto.CreateEmployeeResponse, error) {
	if req.GetEmployeeName() == "" || req.GetBaseSalary() == "" || req.GetCommissionRate() == "" {
		return &proto.CreateEmployeeResponse{
			Success: false,
			Message: "employee name, base salary, and commission rate are required",
		}, nil
	}

	newEmployee := Employee{
		EmployeeName:   req.GetEmployeeName(),
		BaseSalary:     req.GetBaseSalary(),
		CommissionRate: req.GetCommissionRate(),
		CommissionType: int32(req.GetCommissionType()),
		IsActive:       true,
	}

	if req.Position != nil {
		newEmployee.Position = req.GetPosition()
	}
	if req.Phone != nil {
		newEmployee.Phone = req.GetPhone()
	}
	if req.Email != nil {
		newEmployee.Email = req.GetEmail()
	}
	if req.Address != nil {
		newEmployee.Address = req.GetAddress()
	}
	if req.HireDate != nil {
		newEmployee.HireDate = req.GetHireDate()
	}

	if err := s.db.Create(&newEmployee).Error; err != nil {
		return &proto.CreateEmployeeResponse{
			Success: false,
			Message: "error creating employee",
		}, err
	}

	eventData, _ := json.Marshal(map[string]interface{}{
		"id":            newEmployee.ID,
		"employee_name": newEmployee.EmployeeName,
		"email":         newEmployee.Email,
		"position":      newEmployee.Position,
		"action":        "created",
		"timestamp":     time.Now(),
	})

	s.nats.Publish("employee.created", eventData)
	log.Printf("Published employee.created event for ID %d", newEmployee.ID)

	s.InvalidateUserCaches(ctx)

	return &proto.CreateEmployeeResponse{
		Success:  true,
		Message:  "employee created successfully",
		Employee: s.employeeToProto(newEmployee),
	}, nil
}

func (s *UserHandler) GetEmployee(ctx context.Context, req *proto.GetEmployeeRequest) (*proto.GetEmployeeResponse, error) {
	var employee Employee
	if err := s.db.Preload("CommissionTiers").First(&employee, req.GetId()).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.GetEmployeeResponse{
				Success: false,
				Message: "employee not found",
			}, nil
		}
		return &proto.GetEmployeeResponse{
			Success: false,
			Message: "database error",
		}, err
	}

	return &proto.GetEmployeeResponse{
		Success:  true,
		Message:  "employee retrieved successfully",
		Employee: s.employeeToProto(employee),
	}, nil
}

func (s *UserHandler) UpdateEmployee(ctx context.Context, req *proto.UpdateEmployeeRequest) (*proto.UpdateEmployeeResponse, error) {
	var employee Employee
	if err := s.db.Preload("CommissionTiers").First(&employee, req.GetId()).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.UpdateEmployeeResponse{
				Success: false,
				Message: "employee not found",
			}, nil
		}
		return &proto.UpdateEmployeeResponse{
			Success: false,
			Message: "database error",
		}, err
	}

	if req.EmployeeName != nil {
		employee.EmployeeName = req.GetEmployeeName()
	}
	if req.Position != nil {
		employee.Position = req.GetPosition()
	}
	if req.Phone != nil {
		employee.Phone = req.GetPhone()
	}
	if req.Email != nil {
		employee.Email = req.GetEmail()
	}
	if req.Address != nil {
		employee.Address = req.GetAddress()
	}
	if req.BaseSalary != nil {
		employee.BaseSalary = req.GetBaseSalary()
	}
	if req.CommissionRate != nil {
		employee.CommissionRate = req.GetCommissionRate()
	}
	if req.CommissionType != nil {
		employee.CommissionType = int32(req.GetCommissionType())
	}
	if req.IsActive != nil {
		employee.IsActive = req.GetIsActive()
	}

	if err := s.db.Save(&employee).Error; err != nil {
		return &proto.UpdateEmployeeResponse{
			Success: false,
			Message: "error updating employee",
		}, err
	}

	eventData, _ := json.Marshal(map[string]interface{}{
		"id":            employee.ID,
		"employee_name": employee.EmployeeName,
		"email":         employee.Email,
		"position":      employee.Position,
		"action":        "updated",
		"timestamp":     time.Now(),
	})
	s.nats.Publish("employee.updated", eventData)
	log.Printf("Published employee.updated event for ID %d", employee.ID)

	s.InvalidateUserCaches(ctx)

	return &proto.UpdateEmployeeResponse{
		Success:  true,
		Message:  "employee updated successfully",
		Employee: s.employeeToProto(employee),
	}, nil
}

func (s *UserHandler) ListEmployees(ctx context.Context, req *proto.ListEmployeesRequest) (*proto.ListEmployeesResponse, error) {
	var employees []Employee
	var total int64

	query := s.db.Model(&Employee{}).Preload("CommissionTiers")

	if req.IsActive != nil {
		query = query.Where("is_active = ?", req.GetIsActive())
	}
	if req.Position != nil && req.GetPosition() != "" {
		query = query.Where("position ILIKE ?", "%"+req.GetPosition()+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return &proto.ListEmployeesResponse{
			Success: false,
			Message: "database error",
		}, err
	}

	pageSize := int(req.GetPagination().GetPageSize())
	if pageSize <= 0 {
		pageSize = 10
	}

	pageNumber := 1
	if token := req.GetPagination().GetPageToken(); token != "" {
		if n, err := strconv.Atoi(token); err == nil && n > 0 {
			pageNumber = n
		}
	}

	offset := (pageNumber - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Find(&employees).Error; err != nil {
		return &proto.ListEmployeesResponse{
			Success: false,
			Message: "database error",
		}, err
	}

	protoEmployees := make([]*proto.Employee, len(employees))
	for i, emp := range employees {
		protoEmployees[i] = s.employeeToProto(emp)
	}

	nextPageToken := ""
	if int64(pageNumber*pageSize) < total {
		nextPageToken = strconv.Itoa(pageNumber + 1)
	}

	return &proto.ListEmployeesResponse{
		Success:   true,
		Message:   "employees retrieved successfully",
		Employees: protoEmployees,
		Pagination: &proto.PaginationResponse{
			NextPageToken: nextPageToken,
			TotalCount:    int32(total),
		},
	}, nil
}
