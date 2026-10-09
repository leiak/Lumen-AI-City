package a2asrv

import (
	"testing"

	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestCrossCityTransferProtoContract(t *testing.T) {
	request := (&a2av1.TransferCrossCityRequest{}).ProtoReflect()
	service := request.Descriptor().ParentFile().Services().ByName("A2AGateway")
	if service == nil {
		t.Fatal("A2AGateway service descriptor not found")
	}

	wantMethods := map[string]bool{
		"TransferCrossCity":    false,
		"GetCrossCityTransfer": false,
	}
	methods := service.Methods()
	for i := 0; i < methods.Len(); i++ {
		if _, ok := wantMethods[string(methods.Get(i).Name())]; ok {
			wantMethods[string(methods.Get(i).Name())] = true
		}
	}
	for method, found := range wantMethods {
		if !found {
			t.Fatalf("A2AGateway is missing method %s", method)
		}
	}

	wantRequestFields := []string{
		"transfer_id",
		"source_city_id",
		"source_user_id",
		"destination_city_id",
		"destination_user_id",
		"currency",
		"amount",
		"idempotency_key",
		"trace_id",
		"reserved_at_ms",
		"expires_at_ms",
	}
	assertProtoFields(t, request, wantRequestFields)

	wantStatusValues := []string{
		"CROSS_CITY_TRANSFER_STATUS_UNSPECIFIED",
		"CROSS_CITY_TRANSFER_STATUS_RESERVED",
		"CROSS_CITY_TRANSFER_STATUS_CREDITED",
		"CROSS_CITY_TRANSFER_STATUS_SETTLED",
		"CROSS_CITY_TRANSFER_STATUS_REFUNDED",
		"CROSS_CITY_TRANSFER_STATUS_FAILED",
	}
	status := a2av1.CrossCityTransferStatus(0).Descriptor()
	for _, value := range wantStatusValues {
		if status.Values().ByName(protoreflect.Name(value)) == nil {
			t.Fatalf("CrossCityTransferStatus is missing value %s", value)
		}
	}
}

func assertProtoFields(t *testing.T, message protoreflect.Message, wantFields []string) {
	t.Helper()
	for _, field := range wantFields {
		if message.Descriptor().Fields().ByName(protoreflect.Name(field)) == nil {
			t.Fatalf("%s is missing field %s", message.Descriptor().FullName(), field)
		}
	}
}
