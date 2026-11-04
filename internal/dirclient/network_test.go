// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package dirclient

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	corev1 "github.com/agntcy/dir/api/core/v1"
	routingv1 "github.com/agntcy/dir/api/routing/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// testSkillPath is a sample taxonomy path reused across the fixtures below.
const testSkillPath = "a/b"

func TestNetworkQueryToRoutingRPC(t *testing.T) {
	tests := []struct {
		cat  FilterCategory
		want routingv1.RecordQueryType
	}{
		{FilterSkill, routingv1.RecordQueryType_RECORD_QUERY_TYPE_SKILL},
		{FilterDomain, routingv1.RecordQueryType_RECORD_QUERY_TYPE_DOMAIN},
		{FilterModule, routingv1.RecordQueryType_RECORD_QUERY_TYPE_MODULE},
	}
	for _, tt := range tests {
		got := NetworkQuery{Category: tt.cat, Value: testSkillPath}.toRoutingRPC()
		if got.GetType() != tt.want || got.GetValue() != testSkillPath {
			t.Errorf("category %v: got %v %q, want %v %q", tt.cat, got.GetType(), got.GetValue(), tt.want, testSkillPath)
		}
	}
}

func TestDirAddrFrom(t *testing.T) {
	tests := []struct {
		name  string
		addrs []string
		want  string
	}{
		{"dir entry", []string{"/oci/r:5000", "/dir/h:8888"}, "h:8888"},
		{"no dir entry", []string{"/ip4/1.2.3.4/tcp/1"}, ""},
		{"nil", nil, ""},
	}
	for _, tt := range tests {
		if got := dirAddrFrom(tt.addrs); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestNetworkResultFromRPC(t *testing.T) {
	resp := &routingv1.SearchResponse{
		RecordRef: &corev1.RecordRef{Cid: "bafy1"},
		Peer:      &routingv1.Peer{Id: "12D3x", Addrs: []string{"/ip4/1.2.3.4/tcp/1", "/dir/h:8888"}},
		MatchQueries: []*routingv1.RecordQuery{
			{Type: routingv1.RecordQueryType_RECORD_QUERY_TYPE_SKILL, Value: testSkillPath},
		},
		MatchScore: 1,
	}
	want := NetworkResult{
		CID:     "bafy1",
		PeerID:  "12D3x",
		DirAddr: "h:8888",
		Matched: []NetworkQuery{{Category: FilterSkill, Value: testSkillPath}},
		Score:   1,
	}
	if got := networkResultFromRPC(resp); !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestLocalPresenceQueries(t *testing.T) {
	groups := localPresenceQueries([]NetworkQuery{
		{FilterSkill, "a"}, {FilterSkill, "b"}, {FilterDomain, "d"},
	})
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}
	want := [][]Query{
		{
			{Category: FilterSkill, Value: "a"},
			{Category: FilterSkill, Value: "a/*"},
			{Category: FilterSkill, Value: "b"},
			{Category: FilterSkill, Value: "b/*"},
		},
		{{Category: FilterDomain, Value: "d"}, {Category: FilterDomain, Value: "d/*"}},
	}
	if !reflect.DeepEqual(groups, want) {
		t.Errorf("got %+v, want %+v", groups, want)
	}
}

func TestIsPermissionDenied(t *testing.T) {
	wrapped := fmt.Errorf("searching: %w", status.Error(codes.PermissionDenied, "x"))
	if !IsPermissionDenied(wrapped) {
		t.Error("wrapped PermissionDenied: got false")
	}
	if IsPermissionDenied(errors.New("x")) {
		t.Error("plain error: got true")
	}
}
