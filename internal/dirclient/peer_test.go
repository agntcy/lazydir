// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package dirclient

import (
	"errors"
	"testing"

	corev1 "github.com/agntcy/dir/api/core/v1"
)

func TestPeerDialFor(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		want    peerDial
		wantErr error
	}{
		{"insecure", Config{AuthMode: "insecure"}, peerDial{insecure: true}, nil},
		{"none", Config{AuthMode: "none"}, peerDial{insecure: true}, nil},
		{"empty", Config{}, peerDial{insecure: true}, nil},
		{"oidc", Config{AuthMode: "oidc", TLSCAFile: "/ca.pem", TLSSkipVerify: true}, peerDial{caFile: "/ca.pem", skipVerify: true}, nil},
		{"tls", Config{AuthMode: "tls"}, peerDial{}, nil},
		{"token", Config{AuthMode: "token"}, peerDial{}, nil},
		{"jwt-tls", Config{AuthMode: "jwt-tls"}, peerDial{}, nil},
		{"x509", Config{AuthMode: "x509"}, peerDial{}, ErrPeerPreviewUnsupported},
		{"jwt", Config{AuthMode: "jwt"}, peerDial{}, ErrPeerPreviewUnsupported},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := peerDialFor(tt.cfg)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRecordJSON_NilData(t *testing.T) {
	got, err := recordJSON(&corev1.Record{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "{}" {
		t.Errorf("got %q, want {}", got)
	}
}
