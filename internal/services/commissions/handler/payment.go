package handler

import (
	"context"
	proto "syntra-system/proto/protogen/commissions"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (c *CommissionHandler) PayCommission(ctx context.Context, req *proto.PayCommissionRequest) (*proto.PayCommissionResponse, error) {
	if req.GetCommissionCalculationId() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Commission Calculation ID is required")
	}
	if req.GetPaymentTypeId() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Payment Type ID is required")
	}
	if req.GetPaidBy() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Paid By (user ID) is required")
	}

	paymentDate := time.Now().Format("2006-01-02")
	if req.GetPaymentDate() != "" {
		paymentDate = req.GetPaymentDate()
	}

	var calculation CommissionCalculation
	var payment CommissionPayment

	err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&calculation, req.GetCommissionCalculationId()).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return status.Errorf(codes.NotFound, "Commission calculation with ID %d Not Found", req.GetCommissionCalculationId())
			}
			return status.Errorf(codes.Internal, "Failed to retrieve calculation: %v", err)
		}

		if calculation.Status != int32(proto.CommissionStatus_COMMISSION_STATUS_APPROVED) {
			return status.Errorf(codes.FailedPrecondition, "Commission can only be paid from APPROVED status. Current status: %s", proto.CommissionStatus_name[calculation.Status])
		}

		payment = CommissionPayment{
			CommissionCalculationID: calculation.ID,
			EmployeeID:              calculation.EmployeeID,
			PaymentAmount:           calculation.TotalCommission, // Jumlah pembayaran = total komisi
			PaymentDate:             paymentDate,
			// PaymentTypeID:           req.GetPaymentTypeId(),
			ReferenceNumber: req.ReferenceNumber,
			PaidBy:          req.GetPaidBy(),
			Notes:           req.Notes,
		}
		if err := tx.Create(&payment).Error; err != nil {
			return status.Errorf(codes.Internal, "Failed to create payment record: %v", err)
		}

		calculation.Status = int32(proto.CommissionStatus_COMMISSION_STATUS_PAID)
		if err := tx.Save(&calculation).Error; err != nil {
			return status.Errorf(codes.Internal, "Failed to update calculation status: %v", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	c.InvalidateCommissionCaches(ctx, req.GetCommissionCalculationId())

	c.db.WithContext(ctx).Preload("CommissionDetails").First(&calculation, calculation.ID)

	return &proto.PayCommissionResponse{
		CommissionPayment:  c.commissionPaymentToProto(payment),
		UpdatedCalculation: c.commissionCalculationToProto(calculation),
	}, nil
}

func (c *CommissionHandler) GetCommissionPayment(ctx context.Context, req *proto.GetCommissionPaymentRequest) (*proto.GetCommissionPaymentResponse, error) {
	if req.GetCommissionCalculationId() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Commission Calculation ID is required")
	}

	var payment CommissionPayment
	err := c.db.WithContext(ctx).Where("commission_calculation_id = ?", req.GetCommissionCalculationId()).First(&payment).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, status.Errorf(codes.NotFound, "Commission calculation with ID %d Not Found", req.GetCommissionCalculationId())
		}
		return nil, status.Errorf(codes.Internal, "Failed to retrieve commission payment: %v", err)
	}

	return &proto.GetCommissionPaymentResponse{
		CommissionPayment: c.commissionPaymentToProto(payment),
	}, nil
}
