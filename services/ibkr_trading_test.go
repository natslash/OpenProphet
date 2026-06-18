package services

import (
	"context"
	"testing"
	"prophet-trader/interfaces"
)

func TestIBKRTradingService_StubMethods(t *testing.T) {
	service := &IBKRTradingService{}
	ctx := context.Background()

	_, err := service.PlaceOrder(ctx, &interfaces.Order{})
	if err == nil {
		t.Errorf("PlaceOrder should return error")
	}

	err = service.CancelOrder(ctx, "test-id")
	if err == nil {
		t.Errorf("CancelOrder should return error")
	}

	_, err = service.ListOrders(ctx, "all")
	if err == nil {
		t.Errorf("ListOrders should return error")
	}

	_, err = service.GetOrder(ctx, "test-id")
	if err == nil {
		t.Errorf("GetOrder should return error")
	}
}
