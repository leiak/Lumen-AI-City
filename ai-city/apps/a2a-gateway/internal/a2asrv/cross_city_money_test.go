package a2asrv

import (
	"context"
	"testing"
	"time"

	"github.com/aicity/a2a-gateway/internal/crosscity"
	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
)

type fakeMoneyGateway struct {
	called      bool
	creditCalls int
	transfer    *crosscity.EconomyTransfer
	err         error
}

func (f *fakeMoneyGateway) Reserve(context.Context, *crosscity.EconomyTransfer) (*crosscity.EconomyTransfer, error) {
	return f.transfer, f.err
}

func (f *fakeMoneyGateway) CreditInbound(context.Context, *crosscity.EconomyTransfer) (*crosscity.EconomyTransfer, error) {
	f.called = true
	f.creditCalls++
	return f.transfer, f.err
}

func (f *fakeMoneyGateway) SettleOutbound(context.Context, string) (*crosscity.EconomyTransfer, error) {
	return f.transfer, f.err
}

func (f *fakeMoneyGateway) RefundOutbound(context.Context, string) (*crosscity.EconomyTransfer, error) {
	return f.transfer, f.err
}

func (f *fakeMoneyGateway) GetOutbound(context.Context, string) (*crosscity.EconomyTransfer, error) {
	return f.transfer, f.err
}

func (f *fakeMoneyGateway) GetInbound(context.Context, string) (*crosscity.EconomyTransfer, error) {
	f.called = true
	return f.transfer, f.err
}

func (f *fakeMoneyGateway) ListExpiredOutbound(context.Context) ([]crosscity.EconomyTransfer, error) {
	return nil, nil
}

func transferRequest() *a2av1.TransferCrossCityRequest {
	return &a2av1.TransferCrossCityRequest{
		TransferId:        "936aea10-0000-4000-8000-000000000001",
		SourceCityId:      "city_a",
		SourceUserId:      "alice",
		DestinationCityId: "city_b",
		DestinationUserId: "bob",
		Currency:          "gold",
		Amount:            100,
		IdempotencyKey:    "cross-key-1",
		TraceId:           "trace-1",
		ReservedAtMs:      time.Now().UnixMilli(),
		ExpiresAtMs:       time.Now().Add(time.Minute).UnixMilli(),
	}
}

func newMoneyService(gateway *fakeMoneyGateway) *Service {
	service := NewService(nil, nil, nil, nil, nil)
	service.SetMoneyClient(gateway, "city_b")
	return service
}

func TestTransferCrossCityCallsEconomyCredit(t *testing.T) {
	gateway := &fakeMoneyGateway{transfer: &crosscity.EconomyTransfer{
		GlobalID: "936aea10-0000-4000-8000-000000000001",
		Status:   "credited",
		TraceID:  "trace-1",
	}}
	response, err := newMoneyService(gateway).TransferCrossCity(context.Background(), transferRequest())
	if err != nil {
		t.Fatalf("TransferCrossCity() error = %v", err)
	}
	if !gateway.called {
		t.Fatal("economy credit endpoint was not called")
	}
	if response.GetStatus() != a2av1.CrossCityTransferStatus_CROSS_CITY_TRANSFER_STATUS_CREDITED {
		t.Fatalf("unexpected status: %s", response.GetStatus())
	}
}

func TestTransferCrossCityRejectsExpiredWithoutEconomyCall(t *testing.T) {
	gateway := &fakeMoneyGateway{}
	request := transferRequest()
	request.ExpiresAtMs = time.Now().Add(-time.Minute).UnixMilli()
	response, err := newMoneyService(gateway).TransferCrossCity(context.Background(), request)
	if err != nil {
		t.Fatalf("TransferCrossCity() error = %v", err)
	}
	if gateway.called {
		t.Fatal("economy called for expired transfer")
	}
	if response.GetErrorCode() != "CROSS_CITY_EXPIRED" {
		t.Fatalf("unexpected error code: %s", response.GetErrorCode())
	}
}

func TestTransferCrossCityRejectsPeerMismatchWithoutEconomyCall(t *testing.T) {
	gateway := &fakeMoneyGateway{}
	request := transferRequest()
	request.DestinationCityId = "city_c"
	response, err := newMoneyService(gateway).TransferCrossCity(context.Background(), request)
	if err != nil {
		t.Fatalf("TransferCrossCity() error = %v", err)
	}
	if gateway.called {
		t.Fatal("economy called for mismatched peer")
	}
	if response.GetErrorCode() != "CROSS_CITY_PEER_MISMATCH" {
		t.Fatalf("unexpected error code: %s", response.GetErrorCode())
	}
}

func TestTransferCrossCityDuplicateReturnsOriginalCredit(t *testing.T) {
	credited := &crosscity.EconomyTransfer{
		GlobalID: "936aea10-0000-4000-8000-000000000001",
		Status:   "credited",
	}
	gateway := &fakeMoneyGateway{transfer: credited}
	service := newMoneyService(gateway)
	for range 2 {
		response, err := service.TransferCrossCity(context.Background(), transferRequest())
		if err != nil {
			t.Fatalf("TransferCrossCity() error = %v", err)
		}
		if response.GetStatus() != a2av1.CrossCityTransferStatus_CROSS_CITY_TRANSFER_STATUS_CREDITED {
			t.Fatalf("unexpected status: %s", response.GetStatus())
		}
	}
	if gateway.creditCalls != 2 {
		t.Fatal("duplicate replay did not reach idempotent economy endpoint")
	}
}

func TestGetCrossCityTransferIsReadOnly(t *testing.T) {
	gateway := &fakeMoneyGateway{transfer: &crosscity.EconomyTransfer{
		GlobalID: "936aea10-0000-4000-8000-000000000001",
		Status:   "credited",
	}}
	response, err := newMoneyService(gateway).GetCrossCityTransfer(context.Background(), &a2av1.GetCrossCityTransferRequest{
		TransferId: "936aea10-0000-4000-8000-000000000001",
	})
	if err != nil {
		t.Fatalf("GetCrossCityTransfer() error = %v", err)
	}
	if !gateway.called {
		t.Fatal("economy get endpoint was not called")
	}
	if response.GetStatus() != a2av1.CrossCityTransferStatus_CROSS_CITY_TRANSFER_STATUS_CREDITED {
		t.Fatalf("unexpected status: %s", response.GetStatus())
	}
}
