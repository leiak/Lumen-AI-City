package crosscity

import (
	"context"
	"errors"
	"testing"
	"time"

	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
)

type fakeLocalMoney struct {
	reserved *EconomyTransfer
	settled  int
	refunded int
	expired  []EconomyTransfer
}

func (f *fakeLocalMoney) Reserve(_ context.Context, request *EconomyTransfer) (*EconomyTransfer, error) {
	f.reserved = &EconomyTransfer{
		GlobalID:       "936aea10-0000-4000-8000-000000000001",
		IdempotencyKey: request.IdempotencyKey,
		Status:         "reserved",
	}
	return f.reserved, nil
}

func (f *fakeLocalMoney) CreditInbound(context.Context, *EconomyTransfer) (*EconomyTransfer, error) {
	return nil, nil
}

func (f *fakeLocalMoney) SettleOutbound(context.Context, string) (*EconomyTransfer, error) {
	f.settled++
	f.reserved.Status = "settled"
	return f.reserved, nil
}

func (f *fakeLocalMoney) RefundOutbound(context.Context, string) (*EconomyTransfer, error) {
	f.refunded++
	f.reserved.Status = "refunded"
	return f.reserved, nil
}

func (f *fakeLocalMoney) GetOutbound(context.Context, string) (*EconomyTransfer, error) {
	return f.reserved, nil
}

func (f *fakeLocalMoney) GetInbound(context.Context, string) (*EconomyTransfer, error) {
	return f.reserved, nil
}

func (f *fakeLocalMoney) ListExpiredOutbound(context.Context) ([]EconomyTransfer, error) {
	return f.expired, nil
}

type fakeRemoteMoney struct {
	status    a2av1.CrossCityTransferStatus
	errorCode string
	err       error
	called    bool
	request   *a2av1.TransferCrossCityRequest
}

func (f *fakeRemoteMoney) TransferCrossCity(_ context.Context, req *a2av1.TransferCrossCityRequest) (*RemoteTransfer, error) {
	f.called = true
	f.request = req
	return &RemoteTransfer{TransferID: "936aea10-0000-4000-8000-000000000001", Status: f.status, ErrorCode: f.errorCode}, f.err
}

func (f *fakeRemoteMoney) GetCrossCityTransfer(context.Context, *a2av1.GetCrossCityTransferRequest) (*RemoteTransfer, error) {
	f.called = true
	return &RemoteTransfer{TransferID: "936aea10-0000-4000-8000-000000000001", Status: f.status, ErrorCode: f.errorCode}, f.err
}

func newReconciler(local *fakeLocalMoney, remote *fakeRemoteMoney) *Reconciler {
	return &Reconciler{Local: local, Remote: remote, PeerCityID: "city_b"}
}

func TestOrchestrateReservesRemoteCreditsThenSettles(t *testing.T) {
	local := &fakeLocalMoney{}
	remote := &fakeRemoteMoney{status: a2av1.CrossCityTransferStatus_CROSS_CITY_TRANSFER_STATUS_CREDITED}
	request := &EconomyTransfer{
		SourceCityID:      "city_a",
		DestinationCityID: "city_b",
		Currency:          "gold",
		Amount:            100,
		IdempotencyKey:    "cross-city-gold-key",
		ExpiresAt:         time.Now().Add(time.Minute).UTC(),
	}
	transfer, err := newReconciler(local, remote).Orchestrate(context.Background(), request)
	if err != nil {
		t.Fatalf("Orchestrate() error = %v", err)
	}
	if local.settled != 1 || !remote.called || transfer.Status != "settled" {
		t.Fatalf("unexpected saga state local=%+v remote=%v transfer=%+v", local, remote.called, transfer)
	}
	if remote.request.GetIdempotencyKey() != request.IdempotencyKey {
		t.Fatalf("remote idempotency key = %q, want %q", remote.request.GetIdempotencyKey(), request.IdempotencyKey)
	}
}

func TestOrchestrateRemoteTimeoutLeavesReservationPending(t *testing.T) {
	local := &fakeLocalMoney{}
	remote := &fakeRemoteMoney{err: errors.New("context deadline exceeded")}
	transfer, err := newReconciler(local, remote).Orchestrate(context.Background(), &EconomyTransfer{ExpiresAt: time.Now().Add(time.Minute)})
	if err == nil {
		t.Fatal("expected remote error")
	}
	if transfer == nil || transfer.Status != "reserved" || local.settled != 0 {
		t.Fatalf("unexpected pending state transfer=%+v settled=%d", transfer, local.settled)
	}
}

func TestRunOnceSettlesCreditedReservation(t *testing.T) {
	local := &fakeLocalMoney{reserved: &EconomyTransfer{GlobalID: "936aea10-0000-4000-8000-000000000001", Status: "reserved"}}
	local.expired = []EconomyTransfer{*local.reserved}
	remote := &fakeRemoteMoney{status: a2av1.CrossCityTransferStatus_CROSS_CITY_TRANSFER_STATUS_CREDITED}
	settled, refunded, err := newReconciler(local, remote).RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if settled != 1 || refunded != 0 || local.settled != 1 {
		t.Fatalf("unexpected settle result settled=%d refunded=%d status=%s", settled, refunded, local.reserved.Status)
	}
}

func TestRunOnceRefundsMissingExpiredReservation(t *testing.T) {
	local := &fakeLocalMoney{reserved: &EconomyTransfer{Status: "reserved"}}
	local.expired = []EconomyTransfer{*local.reserved}
	remote := &fakeRemoteMoney{err: &MoneyError{Code: "CROSS_CITY_SOURCE_NOT_FOUND"}}
	settled, refunded, err := newReconciler(local, remote).RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if settled != 0 || refunded != 1 || local.reserved.Status != "refunded" {
		t.Fatalf("unexpected refund result settled=%d refunded=%d status=%s", settled, refunded, local.reserved.Status)
	}
}

func TestRunOnceLeavesReservationPendingWhenPeerUnreachable(t *testing.T) {
	local := &fakeLocalMoney{reserved: &EconomyTransfer{Status: "reserved"}}
	local.expired = []EconomyTransfer{*local.reserved}
	remote := &fakeRemoteMoney{err: &MoneyError{Code: "CROSS_CITY_REMOTE_UNAVAILABLE"}}
	settled, refunded, err := newReconciler(local, remote).RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if settled != 0 || refunded != 0 || local.reserved.Status != "reserved" {
		t.Fatalf("unexpected pending result settled=%d refunded=%d status=%s", settled, refunded, local.reserved.Status)
	}
}
