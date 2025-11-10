package handler

import (
	"context"
	"testing"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

func TestPOSHandler_publishOrderEvent(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for receiver constructor.
		db          *gorm.DB
		redisClient *redis.Client
		// Named input parameters for target function.
		event   OrderEvent
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewPOSHandler(tt.db, tt.redisClient)
			gotErr := s.publishOrderEvent(context.Background(), tt.event)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("publishOrderEvent() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("publishOrderEvent() succeeded unexpectedly")
			}
		})
	}
}
