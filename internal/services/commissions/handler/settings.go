package handler

import (
	"context"
	"encoding/json"
	proto "syntra-system/proto/protogen/commissions"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (c *CommissionHandler) GetCommissionSettings(ctx context.Context, req *proto.GetCommissionSettingsRequest) (*proto.GetCommissionSettingsResponse, error) {
	if req.GetEmployeeId() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Employee ID is required")
	}

	employeeID, err := json.Marshal(map[string]interface{}{
		"employee_id": req.GetEmployeeId(),
	})

	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to marshal request data: %v", err)
	}

	msg, err := c.nats.RequestWithContext(ctx, "employee.get", employeeID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to request employee data via NATS: %v", err)
	}

	var employeeData struct {
		Found          bool   `json:"found"`
		Error          string `json:"error,omitempty"`
		EmployeeName   string `json:"employee_name,omitempty"`
		Email          string `json:"email,omitempty"`
		Position       string `json:"position,omitempty"`
		CommissionRate string `json:"commission_rate,omitempty"`
		CommissionType string `json:"commission_type,omitempty"`
	}

	if err := json.Unmarshal(msg.Data, &employeeData); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to parse employee response: %v", err)
	}

	if !employeeData.Found {
		return nil, status.Errorf(codes.NotFound, "Employee with ID %d not found", employeeID)
	}

	var tierSettingsGorm []CommissionTierInfo
	if employeeData.CommissionType == "tiered" {
		if err := c.db.WithContext(ctx).Where("employee_id = ?", employeeID).Order("min_sales_amount asc").Find(&tierSettingsGorm).Error; err != nil {
			return nil, status.Errorf(codes.Internal, "Failed to get commission tiers: %v", err)
		}
	}

	var commissionTypeProto proto.CommissionType
	switch employeeData.CommissionType {
	case "percentage":
		commissionTypeProto = proto.CommissionType_COMMISSION_TYPE_PERCENTAGE
	case "tiered":
		commissionTypeProto = proto.CommissionType_COMMISSION_TYPE_TIERED
	case "fixed_amount":
		commissionTypeProto = proto.CommissionType_COMMISSION_TYPE_FIXED_AMOUNT
	default:
		commissionTypeProto = proto.CommissionType_COMMISSION_TYPE_UNSPECIFIED
	}

	employeeSummaryProto := &proto.EmployeeSummary{
		Id:             req.GetEmployeeId(),
		EmployeeName:   employeeData.EmployeeName,
		Position:       &employeeData.Position,
		CommissionRate: employeeData.CommissionRate,
		CommissionType: commissionTypeProto,
	}

	var tierSettingsProto []*proto.CommissionTierSetting
	for _, tier := range tierSettingsGorm {
		tierSettingsProto = append(tierSettingsProto, &proto.CommissionTierSetting{
			MinSalesAmount: tier.MinSalesAmount,
			MaxSalesAmount: &tier.MaxSalesAmount,
			CommissionRate: tier.CommissionRate,
		})
	}

	return &proto.GetCommissionSettingsResponse{
		Success:      true,
		Employee:     employeeSummaryProto,
		TierSettings: tierSettingsProto,
	}, nil
}
