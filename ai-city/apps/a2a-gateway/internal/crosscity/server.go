package crosscity

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func serverTLSConfig(certFile, keyFile, caFile, expectedPeerCN string) (*tls.Config, error) {
	if certFile == "" && keyFile == "" && caFile == "" && expectedPeerCN == "" {
		return nil, nil
	}
	if certFile == "" || keyFile == "" || caFile == "" || expectedPeerCN == "" {
		return nil, errors.New("crosscity.LoadServerOptions: TLS_CERT, TLS_KEY, CLIENT_CA, and A2A_EXPECTED_PEER_CN are required together")
	}

	certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("crosscity.LoadServerOptions: load server cert/key: %w", err)
	}
	caData, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("crosscity.LoadServerOptions: read client CA bundle %s: %w", caFile, err)
	}
	clientCAs := x509.NewCertPool()
	if !clientCAs.AppendCertsFromPEM(caData) {
		return nil, fmt.Errorf("crosscity.LoadServerOptions: failed to append client CA certs from %s", caFile)
	}

	tlsConfig := &tls.Config{
		Certificates:     []tls.Certificate{certificate},
		ClientCAs:        clientCAs,
		ClientAuth:       tls.RequireAndVerifyClientCert,
		MinVersion:       tls.VersionTLS12,
		VerifyConnection: func(state tls.ConnectionState) error { return verifyPeerCN(state, expectedPeerCN) },
	}

	return tlsConfig, nil
}

func verifyPeerCN(state tls.ConnectionState, expectedPeerCN string) error {
	if len(state.PeerCertificates) == 0 {
		return errors.New("client certificate is required")
	}
	peerCN := state.PeerCertificates[0].Subject.CommonName
	if peerCN != expectedPeerCN {
		return fmt.Errorf("unexpected peer CN %q; expected %q", peerCN, expectedPeerCN)
	}
	return nil
}

func LoadServerOptions(certFile, keyFile, caFile, expectedPeerCN string) ([]grpc.ServerOption, error) {
	if certFile == "" && keyFile == "" && caFile == "" && expectedPeerCN == "" {
		return nil, nil
	}
	tlsConfig, err := serverTLSConfig(certFile, keyFile, caFile, expectedPeerCN)
	if err != nil {
		return nil, err
	}
	return []grpc.ServerOption{grpc.Creds(credentials.NewTLS(tlsConfig))}, nil
}
