package crosscity

import (
	"context"
	"time"

	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type RemoteTransfer struct {
	TransferID string
	Status     a2av1.CrossCityTransferStatus
	ErrorCode  string
	Message    string
	TraceID    string
}

type RemoteMoneyGateway interface {
	TransferCrossCity(context.Context, *a2av1.TransferCrossCityRequest) (*RemoteTransfer, error)
	GetCrossCityTransfer(context.Context, *a2av1.GetCrossCityTransferRequest) (*RemoteTransfer, error)
}

type GRPCMoneyGateway struct {
	Client a2av1.A2AGatewayClient
}

func NewGRPCMoneyGateway(conn *grpc.ClientConn) *GRPCMoneyGateway {
	return &GRPCMoneyGateway{Client: a2av1.NewA2AGatewayClient(conn)}
}

func (c *GRPCMoneyGateway) TransferCrossCity(ctx context.Context, req *a2av1.TransferCrossCityRequest) (*RemoteTransfer, error) {
	resp, err := c.Client.TransferCrossCity(ctx, req)
	if err != nil {
		return nil, remoteMoneyError(err)
	}
	return remoteTransferFromProto(resp), nil
}

func (c *GRPCMoneyGateway) GetCrossCityTransfer(ctx context.Context, req *a2av1.GetCrossCityTransferRequest) (*RemoteTransfer, error) {
	resp, err := c.Client.GetCrossCityTransfer(ctx, req)
	if err != nil {
		return nil, remoteMoneyError(err)
	}
	return remoteTransferFromProto(resp), nil
}

func remoteMoneyError(err error) error {
	if status.Code(err) == codes.NotFound {
		return &MoneyError{Code: "CROSS_CITY_SOURCE_NOT_FOUND", Message: "remote transfer not found", Status: 404}
	}
	return &MoneyError{Code: "CROSS_CITY_REMOTE_UNAVAILABLE", Message: err.Error(), Status: 503}
}

func remoteTransferFromProto(resp *a2av1.TransferCrossCityResponse) *RemoteTransfer {
	if resp == nil {
		return nil
	}
	return &RemoteTransfer{
		TransferID: resp.GetTransferId(),
		Status:     resp.GetStatus(),
		ErrorCode:  resp.GetErrorCode(),
		Message:    resp.GetMessage(),
		TraceID:    resp.GetTraceId(),
	}
}

type Reconciler struct {
	Local      MoneyGateway
	Remote     RemoteMoneyGateway
	PeerCityID string
}

func (r *Reconciler) Orchestrate(ctx context.Context, transfer *EconomyTransfer) (*EconomyTransfer, error) {
	reserved, err := r.Local.Reserve(ctx, transfer)
	if err != nil {
		return nil, err
	}
	remote, err := r.Remote.TransferCrossCity(ctx, remoteTransferRequest(reserved))
	if err != nil {
		return reserved, err
	}
	if remote == nil || remote.ErrorCode != "" || remote.Status != a2av1.CrossCityTransferStatus_CROSS_CITY_TRANSFER_STATUS_CREDITED {
		return reserved, &MoneyError{Code: remoteTransferErrorCode(remote), Message: "remote credit rejected"}
	}
	return r.Local.SettleOutbound(ctx, reserved.GlobalID)
}

func (r *Reconciler) RunOnce(ctx context.Context) (settled, refunded int, err error) {
	transfers, listErr := r.Local.ListExpiredOutbound(ctx)
	if listErr != nil {
		return 0, 0, listErr
	}
	for _, transfer := range transfers {
		remote, remoteErr := r.Remote.GetCrossCityTransfer(ctx, &a2av1.GetCrossCityTransferRequest{
			TransferId:        transfer.GlobalID,
			SourceCityId:      transfer.SourceCityID,
			DestinationCityId: transfer.DestinationCityID,
			IdempotencyKey:    "",
			TraceId:           transfer.TraceID,
		})
		if remoteErr != nil {
			if moneyErr, ok := remoteErr.(*MoneyError); ok && moneyErr.Code == "CROSS_CITY_SOURCE_NOT_FOUND" {
				if _, refundErr := r.Local.RefundOutbound(ctx, transfer.GlobalID); refundErr == nil {
					refunded++
				}
			}
			continue
		}
		if remote != nil && remote.Status == a2av1.CrossCityTransferStatus_CROSS_CITY_TRANSFER_STATUS_CREDITED {
			if _, settleErr := r.Local.SettleOutbound(ctx, transfer.GlobalID); settleErr == nil {
				settled++
			}
		}
	}
	return settled, refunded, nil
}

func StartCrossCityReconciler(ctx context.Context, reconciler *Reconciler, interval time.Duration) context.CancelFunc {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		_, _, _ = reconciler.RunOnce(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _, _ = reconciler.RunOnce(ctx)
			}
		}
	}()
	return cancel
}

func remoteTransferRequest(transfer *EconomyTransfer) *a2av1.TransferCrossCityRequest {
	req := &a2av1.TransferCrossCityRequest{
		TransferId:        transfer.GlobalID,
		SourceCityId:      transfer.SourceCityID,
		SourceUserId:      transfer.SourceUserID,
		DestinationCityId: transfer.DestinationCityID,
		DestinationUserId: transfer.DestinationUserID,
		Currency:          transfer.Currency,
		Amount:            transfer.Amount,
		TraceId:           transfer.TraceID,
	}
	if transfer.ReservedAt != nil {
		req.ReservedAtMs = transfer.ReservedAt.UnixMilli()
	}
	req.ExpiresAtMs = transfer.ExpiresAt.UnixMilli()
	return req
}

func remoteTransferErrorCode(remote *RemoteTransfer) string {
	if remote != nil && remote.ErrorCode != "" {
		return remote.ErrorCode
	}
	return "CROSS_CITY_REMOTE_REJECTED"
}
