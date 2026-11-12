// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package dirclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	corev1 "github.com/agntcy/dir/api/core/v1"
	storev1 "github.com/agntcy/dir/api/store/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// ErrPeerPreviewUnsupported is returned when the current auth mode needs
// credentials that cannot be presented to an arbitrary peer.
var ErrPeerPreviewUnsupported = errors.New("peer preview not supported for this auth mode")

// peerDial describes how to dial a peer's directory API without credentials.
type peerDial struct {
	insecure   bool
	caFile     string
	skipVerify bool
}

// peerDialFor derives the transport settings for a peer from the current
// connection config. The SDK's "tls" mode is mutual TLS, which cannot express
// "TLS without credentials", so peers are dialled with grpc-go directly.
func peerDialFor(current Config) (peerDial, error) {
	switch current.AuthMode {
	case "", "none", "insecure":
		return peerDial{insecure: true}, nil
	case "tls", "token", "oidc", "jwt-tls":
		return peerDial{caFile: current.TLSCAFile, skipVerify: current.TLSSkipVerify}, nil
	default: // x509, jwt: identity is bound to the SPIFFE workload API.
		return peerDial{}, fmt.Errorf("%w: %q", ErrPeerPreviewUnsupported, current.AuthMode)
	}
}

// PeerClient is a credential-free connection to another peer's directory API.
type PeerClient struct {
	conn *grpc.ClientConn
	Addr string
}

// ConnectPeer dials dirAddr using transport settings derived from current.
func ConnectPeer(current Config, dirAddr string) (*PeerClient, error) {
	d, err := peerDialFor(current)
	if err != nil {
		return nil, err
	}

	var creds credentials.TransportCredentials
	if d.insecure {
		creds = insecure.NewCredentials()
	} else {
		cfg := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: d.skipVerify} //nolint:gosec // opt-in via the user's TLS skip-verify setting
		if d.caFile != "" {
			pem, err := os.ReadFile(d.caFile)
			if err != nil {
				return nil, fmt.Errorf("reading CA file: %w", err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(pem) {
				return nil, fmt.Errorf("no certificates found in CA file %s", d.caFile)
			}
			cfg.RootCAs = pool
		}
		creds = credentials.NewTLS(cfg)
	}

	conn, err := grpc.NewClient(dirAddr, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("dialing peer %s: %w", dirAddr, err)
	}
	return &PeerClient{conn: conn, Addr: dirAddr}, nil
}

// PullJSON fetches a single record by CID from the peer and returns it as
// formatted JSON.
func (p *PeerClient) PullJSON(ctx context.Context, cid string) (string, error) {
	stream, err := storev1.NewStoreServiceClient(p.conn).Pull(ctx)
	if err != nil {
		return "", fmt.Errorf("opening pull stream: %w", err)
	}
	if err := stream.Send(&corev1.RecordRef{Cid: cid}); err != nil {
		return "", fmt.Errorf("pulling record %s: %w", cid, err)
	}
	if err := stream.CloseSend(); err != nil {
		return "", fmt.Errorf("pulling record %s: %w", cid, err)
	}
	record, err := stream.Recv()
	if err != nil {
		return "", fmt.Errorf("pulling record %s: %w", cid, err)
	}
	return recordJSON(record)
}

// Close releases the peer connection.
func (p *PeerClient) Close() {
	_ = p.conn.Close()
}
