package handler

import (
	"context"
	"log"

	"github.com/go-redis/redis/v8"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"

	lib "syntra-system/internal/utils"
	proto "syntra-system/proto/protogen/commissions"
)

type CommissionHandler struct {
	proto.UnimplementedCommissionServiceServer
	db    *gorm.DB
	redis *redis.Client
	nats  *nats.Conn
}

func NewCommissionHandler(db *gorm.DB, redisClient *redis.Client) *CommissionHandler {
	nc, err := nats.Connect("nats://10.147.17.76:4222",
		nats.ConnectHandler(func(nc *nats.Conn) {
			log.Printf("Commission service connected to NATS at %v", nc.ConnectedUrl())
		}),
		nats.DisconnectHandler(func(nc *nats.Conn) {
			log.Printf("Commission service disconnected from NATS")
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Printf("Commission service reconnected to NATS at %v", nc.ConnectedUrl())
		}),
	)
	if err != nil {
		log.Fatal("Failed to connect to NATS:", err)
	}

	handler := &CommissionHandler{
		db:    db,
		redis: redisClient,
		nats:  nc,
	}

	go func() {
		log.Println("Starting to listen for POS events...")
		if err := handler.SubscribeToPosEvents(context.Background()); err != nil {
			log.Printf("Error subscribing to POS events: %v", err)
		}
	}()

	go func() {
		log.Println("Starting to listen for employee events...")
		if err := handler.SubscribeToEmployeeEvents(context.Background()); err != nil {
			log.Printf("Error subscribing to employee events: %v", err)
		}
	}()

	if err := handler.SubscribeToOrderEvents(); err != nil {
		log.Fatalf("Failed to setup NATS subscriptions: %v", err)
	}

	return handler
}

func (c *CommissionHandler) commissionCalculationToProto(commissionCalculation CommissionCalculation) *proto.CommissionCalculation {
	var detailsProto []*proto.CommissionDetail
	// Konversi setiap item dalam slice CommissionDetails
	for _, detail := range commissionCalculation.CommissionDetails {
		detailsProto = append(detailsProto, c.commissionDetailToProto(detail))
	}

	var paymentProto *proto.CommissionPayment
	// Konversi relasi CommissionPayment jika ada (tidak nil)
	if commissionCalculation.CommissionPayment.ID != 0 {
		// 2. Berikan struct-nya langsung, JANGAN gunakan '*'
		paymentProto = c.commissionPaymentToProto(commissionCalculation.CommissionPayment)
	}

	return &proto.CommissionCalculation{
		Id:                     commissionCalculation.ID,
		EmployeeId:             commissionCalculation.EmployeeID,
		CalculationPeriodStart: commissionCalculation.CalculationPeriodStart,
		CalculationPeriodEnd:   commissionCalculation.CalculationPeriodEnd,
		TotalSales:             commissionCalculation.TotalSales,
		BaseCommission:         commissionCalculation.BaseCommission,
		BonusCommission:        commissionCalculation.BonusCommission,
		TotalCommission:        commissionCalculation.TotalCommission,
		Status:                 proto.CommissionStatus(commissionCalculation.Status), // Konversi int32 ke enum proto
		CalculatedBy:           commissionCalculation.CalculatedBy,
		CalculatedByName:       commissionCalculation.CalculatedByName,
		ApprovedBy:             commissionCalculation.ApprovedBy,
		ApprovedByName:         commissionCalculation.ApprovedByName,
		Notes:                  commissionCalculation.Notes,
		CreatedAt:              timestamppb.New(lib.TimeNowOrZero(commissionCalculation.CreatedAt)),
		UpdatedAt:              timestamppb.New(lib.TimeNowOrZero(commissionCalculation.UpdatedAt)),
		CommissionDetails:      detailsProto,
		CommissionPayment:      paymentProto,
		// Note: Employee (summary) tidak diisi di sini, mirip seperti PaymentType.
	}
}

func (h *CommissionHandler) commissionDetailToProto(commissionDetail CommissionDetail) *proto.CommissionDetail {
	return &proto.CommissionDetail{
		Id:                      commissionDetail.ID,
		CommissionCalculationId: commissionDetail.CommissionCalculationID,
		OrderItemId:             commissionDetail.OrderItemID,
		// ProductId:               commissionDetail.ProductID,
		SalesAmount:      commissionDetail.SalesAmount,
		CommissionRate:   commissionDetail.CommissionRate,
		CommissionAmount: commissionDetail.CommissionAmount,
		CreatedAt:        timestamppb.New(lib.TimeNowOrZero(commissionDetail.CreatedAt)),
	}
}

func (h *CommissionHandler) commissionPaymentToProto(commissionPayment CommissionPayment) *proto.CommissionPayment {
	return &proto.CommissionPayment{
		Id:                      commissionPayment.ID,
		CommissionCalculationId: commissionPayment.CommissionCalculationID,
		EmployeeId:              commissionPayment.EmployeeID,
		PaymentAmount:           commissionPayment.PaymentAmount,
		PaymentDate:             commissionPayment.PaymentDate,
		// PaymentTypeId:           commissionPayment.PaymentTypeID,
		ReferenceNumber: commissionPayment.ReferenceNumber, // Langsung assign karena GORM model & proto sama-sama pointer
		PaidBy:          commissionPayment.PaidBy,
		Notes:           commissionPayment.Notes,
		CreatedAt:       timestamppb.New(lib.TimeNowOrZero(commissionPayment.CreatedAt)),
		// Note: PaymentType (summary) tidak diisi di sini karena datanya dari service lain.
		// Data ini bisa di-populate di level atas jika diperlukan (misal, dengan gRPC call lain).
	}
}
