// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package dirclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	routingv1 "github.com/agntcy/dir/api/routing/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// dirAddrPrefix marks the peer address entry that carries the peer's
// directory API endpoint.
const dirAddrPrefix = "/dir/"

// NetworkQuery is one routing search predicate. Category is limited to
// FilterSkill, FilterDomain and FilterModule.
type NetworkQuery struct {
	Category FilterCategory
	Value    string
}

// NetworkResult is one record announced on the network by a peer.
type NetworkResult struct {
	CID     string
	PeerID  string
	DirAddr string // peer's directory API address, empty if it announced none
	Matched []NetworkQuery
	Score   uint32
}

func (q NetworkQuery) toRoutingRPC() *routingv1.RecordQuery {
	var t routingv1.RecordQueryType
	switch q.Category {
	case FilterSkill:
		t = routingv1.RecordQueryType_RECORD_QUERY_TYPE_SKILL
	case FilterDomain:
		t = routingv1.RecordQueryType_RECORD_QUERY_TYPE_DOMAIN
	case FilterModule:
		t = routingv1.RecordQueryType_RECORD_QUERY_TYPE_MODULE
	}
	return &routingv1.RecordQuery{Type: t, Value: q.Value}
}

func networkQueryFromRPC(q *routingv1.RecordQuery) NetworkQuery {
	var cat FilterCategory
	switch q.GetType() {
	case routingv1.RecordQueryType_RECORD_QUERY_TYPE_DOMAIN:
		cat = FilterDomain
	case routingv1.RecordQueryType_RECORD_QUERY_TYPE_MODULE:
		cat = FilterModule
	default:
		cat = FilterSkill
	}
	return NetworkQuery{Category: cat, Value: q.GetValue()}
}

// dirAddrFrom returns the first "/dir/" peer address with the prefix
// stripped, or "" if there is none.
func dirAddrFrom(addrs []string) string {
	for _, a := range addrs {
		if rest, ok := strings.CutPrefix(a, dirAddrPrefix); ok {
			return rest
		}
	}
	return ""
}

func networkResultFromRPC(r *routingv1.SearchResponse) NetworkResult {
	res := NetworkResult{
		CID:     r.GetRecordRef().GetCid(),
		PeerID:  r.GetPeer().GetId(),
		DirAddr: dirAddrFrom(r.GetPeer().GetAddrs()),
		Score:   r.GetMatchScore(),
	}
	for _, q := range r.GetMatchQueries() {
		res.Matched = append(res.Matched, networkQueryFromRPC(q))
	}
	return res
}

// SearchNetwork runs a routing search and returns results sorted by match
// score, highest first. It reads the raw stream rather than SearchRouting,
// which logs and swallows stream errors, so PermissionDenied surfaces.
func (c *Client) SearchNetwork(ctx context.Context, queries []NetworkQuery, minScore, limit uint32) ([]NetworkResult, error) {
	if len(queries) == 0 {
		return nil, nil
	}
	req := &routingv1.SearchRequest{
		MinMatchScore: &minScore,
		Limit:         &limit,
	}
	for _, q := range queries {
		req.Queries = append(req.Queries, q.toRoutingRPC())
	}
	stream, err := c.c.Search(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("searching network: %w", err)
	}
	var results []NetworkResult
	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("searching network: %w", err)
		}
		results = append(results, networkResultFromRPC(resp))
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	return results, nil
}

// localPresenceQueries builds one local query group per category in use.
// Each value v becomes v and v/*: local search ORs values within a category
// and supports wildcards, which reproduces routing's exact-or-prefix match.
func localPresenceQueries(queries []NetworkQuery) [][]Query {
	var order []FilterCategory
	byCat := map[FilterCategory][]Query{}
	for _, q := range queries {
		if _, ok := byCat[q.Category]; !ok {
			order = append(order, q.Category)
		}
		byCat[q.Category] = append(byCat[q.Category],
			Query{Category: q.Category, Value: q.Value},
			Query{Category: q.Category, Value: q.Value + "/*"})
	}
	groups := make([][]Query, 0, len(order))
	for _, cat := range order {
		groups = append(groups, byCat[cat])
	}
	return groups
}

// LocalCIDs returns the CIDs held by this node that match any of the network
// queries. It uses one local search per category and unions the results
// instead of LookupBatch, which aborts at the first missing CID.
func (c *Client) LocalCIDs(ctx context.Context, queries []NetworkQuery) (map[string]bool, error) {
	out := make(map[string]bool)
	for _, group := range localPresenceQueries(queries) {
		cids, err := c.MatchingCIDs(ctx, group)
		if err != nil {
			return nil, err
		}
		for _, cid := range cids {
			out[cid] = true
		}
	}
	return out, nil
}

// IsPermissionDenied reports whether err is, or wraps, a gRPC PermissionDenied.
func IsPermissionDenied(err error) bool {
	return status.Code(err) == codes.PermissionDenied
}
