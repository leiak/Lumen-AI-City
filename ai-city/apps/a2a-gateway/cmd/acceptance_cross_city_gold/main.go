package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/aicity/a2a-gateway/internal/crosscity"
	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type walletResponse struct {
	UserID      string `json:"user_id"`
	GoldBalance int64  `json:"gold_balance"`
}

func fail(format string, args ...any) {
	log.Fatalf("acceptance_cross_city_gold: "+format, args...)
}

func check(condition bool, format string, args ...any) {
	if !condition {
		fail(format, args...)
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func sourceWallet(ctx context.Context, pool *pgxpool.Pool) string {
	var userID string
	err := pool.QueryRow(ctx, "SELECT user_id FROM wallet WHERE gold_balance >= 500 ORDER BY user_id LIMIT 1").Scan(&userID)
	if err != nil {
		fail("resolve source wallet: %v", err)
	}
	return userID
}

func checkHealth(ctx context.Context, client *http.Client, baseURL, label string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
	if err != nil {
		fail("%s health request: %v", label, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		fail("%s health request: %v", label, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		fail("%s health status = %d body=%s", label, resp.StatusCode, raw)
	}
}

func checkDestinationWallet(ctx context.Context, client *http.Client, baseURL, userID string, expected int64) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/wallet/"+userID, nil)
	if err != nil {
		fail("destination wallet request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		fail("destination wallet request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		fail("destination wallet status = %d body=%s", resp.StatusCode, raw)
	}
	var wallet walletResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&wallet); err != nil {
		fail("decode destination wallet: %v", err)
	}
	check(wallet.GoldBalance == expected, "destination gold balance = %d, want %d", wallet.GoldBalance, expected)
}

func newTransfer(sourceUserID, destinationUserID, idempotencyKey string, amount int64) *crosscity.EconomyTransfer {
	return &crosscity.EconomyTransfer{
		SourceCityID:      getEnv("CITY_ID", "city_a"),
		SourceUserID:      sourceUserID,
		DestinationCityID: getEnv("PEER_CITY_ID", "city_b"),
		DestinationUserID: destinationUserID,
		Currency:          "gold",
		Amount:            amount,
		IdempotencyKey:    idempotencyKey,
		TraceID:           "acceptance-cross-city-gold",
		ExpiresAt:         time.Now().Add(10 * time.Minute).UTC(),
	}
}

func remoteRequest(transfer *crosscity.EconomyTransfer) *a2av1.TransferCrossCityRequest {
	request := &a2av1.TransferCrossCityRequest{
		TransferId:        transfer.GlobalID,
		SourceCityId:      transfer.SourceCityID,
		SourceUserId:      transfer.SourceUserID,
		DestinationCityId: transfer.DestinationCityID,
		DestinationUserId: transfer.DestinationUserID,
		Currency:          transfer.Currency,
		Amount:            transfer.Amount,
		IdempotencyKey:    transfer.IdempotencyKey,
		TraceId:           transfer.TraceID,
	}
	if transfer.ReservedAt != nil {
		request.ReservedAtMs = transfer.ReservedAt.UnixMilli()
	}
	request.ExpiresAtMs = transfer.ExpiresAt.UnixMilli()
	return request
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	localEconomyURL := getEnv("CITY_A_ECONOMY_URL", "http://economy-service:8005")
	peerEconomyURL := getEnv("CITY_B_ECONOMY_URL", "http://host.docker.internal:8006")
	peerEndpoint := getEnv("PEER_A2A_ENDPOINT", "host.docker.internal:50062")
	token := getEnv("CROSS_CITY_INTERNAL_TOKEN", "dev-cross-city-service-token")
	httpClient := &http.Client{Timeout: 10 * time.Second}

	checkHealth(ctx, httpClient, localEconomyURL, "city-a economy")
	checkHealth(ctx, httpClient, peerEconomyURL, "city-b economy")

	pool, err := pgxpool.New(ctx, getEnv("DATABASE_URL", "postgresql://aicity:aicity_dev@postgres:5432/aicity"))
	if err != nil {
		fail("connect city-a database: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		fail("ping city-a database: %v", err)
	}
	sourceUserID := sourceWallet(ctx, pool)
	destinationUserID := uuid.NewString()
	localMoney := crosscity.NewHTTPMoneyClient(localEconomyURL, token, httpClient)

	settleRequest := newTransfer(sourceUserID, destinationUserID, uuid.NewString(), 31)
	firstReserved, err := localMoney.Reserve(ctx, settleRequest)
	if err != nil {
		fail("first reserve: %v", err)
	}
	secondReserved, err := localMoney.Reserve(ctx, settleRequest)
	if err != nil {
		fail("idempotent reserve: %v", err)
	}
	check(firstReserved.GlobalID != "" && firstReserved.GlobalID == secondReserved.GlobalID,
		"idempotent reserve returned different global IDs (%q != %q)", firstReserved.GlobalID, secondReserved.GlobalID)
	check(firstReserved.Status == "reserved" && secondReserved.Status == "reserved",
		"reserve statuses = %q / %q, want reserved", firstReserved.Status, secondReserved.Status)

	peerConn, err := crosscity.Dial(peerEndpoint)
	if err != nil {
		fail("peer mTLS dial: %v", err)
	}
	defer peerConn.Close()
	remoteMoney := crosscity.NewGRPCMoneyGateway(peerConn)
	reconciler := &crosscity.Reconciler{Local: localMoney, Remote: remoteMoney, PeerCityID: getEnv("PEER_CITY_ID", "city_b")}
	settled, err := reconciler.Orchestrate(ctx, settleRequest)
	if err != nil {
		fail("orchestrate settled transfer: %v", err)
	}
	check(settled.Status == "settled", "settled transfer status = %q, want settled", settled.Status)
	checkDestinationWallet(ctx, httpClient, peerEconomyURL, destinationUserID, 31)

	remoteRetry, err := remoteMoney.TransferCrossCity(ctx, remoteRequest(firstReserved))
	if err != nil {
		fail("remote idempotent credit: %v", err)
	}
	check(remoteRetry.TransferID == firstReserved.GlobalID &&
		remoteRetry.Status == a2av1.CrossCityTransferStatus_CROSS_CITY_TRANSFER_STATUS_CREDITED,
		"remote idempotent retry = transfer %q status %s, want transfer %q credited",
		remoteRetry.TransferID, remoteRetry.Status, firstReserved.GlobalID)
	remoteLookup, err := remoteMoney.GetCrossCityTransfer(ctx, &a2av1.GetCrossCityTransferRequest{
		TransferId:        firstReserved.GlobalID,
		SourceCityId:      firstReserved.SourceCityID,
		DestinationCityId: firstReserved.DestinationCityID,
		TraceId:           firstReserved.TraceID,
	})
	if err != nil {
		fail("remote transfer lookup: %v", err)
	}
	check(remoteLookup.Status == a2av1.CrossCityTransferStatus_CROSS_CITY_TRANSFER_STATUS_CREDITED,
		"remote lookup status = %s, want credited", remoteLookup.Status)

	refundUserID := uuid.NewString()
	refundRequest := newTransfer(sourceUserID, refundUserID, uuid.NewString(), 17)
	reservedForRefund, err := localMoney.Reserve(ctx, refundRequest)
	if err != nil {
		fail("reserve refund transfer: %v", err)
	}
	commandTag, err := pool.Exec(ctx,
		"UPDATE cross_city_transfer SET expires_at = NOW() - interval '1 second' WHERE global_id = $1 AND status = 'reserved'",
		reservedForRefund.GlobalID,
	)
	if err != nil {
		fail("expire refund reservation: %v", err)
	}
	check(commandTag.RowsAffected() == 1, "expire refund reservation affected %d rows, want 1", commandTag.RowsAffected())
	refunded, err := localMoney.RefundOutbound(ctx, reservedForRefund.GlobalID)
	if err != nil {
		fail("refund transfer: %v", err)
	}
	check(refunded.Status == "refunded", "refunded transfer status = %q, want refunded", refunded.Status)

	fmt.Println("acceptance_cross_city_gold: PASS (reserve, idempotency, credit, settle, lookup, refund)")
}
