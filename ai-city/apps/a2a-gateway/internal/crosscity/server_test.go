package crosscity

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"
)

func TestLoadServerOptionsRequiresCompleteConfiguration(t *testing.T) {
	tests := []struct {
		name string
		cert string
		key  string
		ca   string
		cn   string
	}{
		{name: "missing certificate", key: "key", ca: "ca", cn: "peer"},
		{name: "missing key", cert: "cert", ca: "ca", cn: "peer"},
		{name: "missing CA", cert: "cert", key: "key", cn: "peer"},
		{name: "missing peer CN", cert: "cert", key: "key", ca: "ca"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadServerOptions(test.cert, test.key, test.ca, test.cn)
			if err == nil {
				t.Fatal("LoadServerOptions() = nil, want error")
			}
		})
	}
}

func TestVerifyPeerCN(t *testing.T) {
	expectedCN := "a2a-gateway-city-b"
	if err := verifyPeerCN(tls.ConnectionState{}, expectedCN); err == nil {
		t.Fatal("verifyPeerCN(empty state) = nil, want error")
	}

	wrongState := tls.ConnectionState{PeerCertificates: []*x509.Certificate{{Subject: pkix.Name{CommonName: "wrong-peer"}}}}
	if err := verifyPeerCN(wrongState, expectedCN); err == nil {
		t.Fatal("verifyPeerCN(wrong CN) = nil, want error")
	}

	validState := tls.ConnectionState{PeerCertificates: []*x509.Certificate{{Subject: pkix.Name{CommonName: expectedCN}}}}
	if err := verifyPeerCN(validState, expectedCN); err != nil {
		t.Fatalf("verifyPeerCN(valid CN) = %v, want nil", err)
	}
}
