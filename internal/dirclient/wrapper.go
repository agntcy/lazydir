// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package dirclient

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "github.com/agntcy/dir/api/core/v1"
	searchv1 "github.com/agntcy/dir/api/search/v1"
	"github.com/agntcy/dir/client"
	"github.com/agntcy/oasf-sdk/pkg/decoder"
	"google.golang.org/protobuf/encoding/protojson"
)

// Config holds the connection configuration for a directory server.
type Config struct {
	ServerAddress string
	AuthMode      string
	TLSSkipVerify bool
	TLSCAFile     string
	TLSCertFile   string
	TLSKeyFile    string
	AuthToken     string
	OIDCIssuer    string
	OIDCClientID  string
}

// RecordStatus describes the lifecycle state of a record from the TUI's
// perspective. The zero value (StatusLocal) means the record is fully
// available on the connected server.
type RecordStatus int

const (
	StatusLocal       RecordStatus = iota // fully synced / native record
	StatusSyncing                         // CreateSync in progress
	StatusReconciling                     // sync done, waiting for indexer
	StatusFailed                          // sync failed
)

// RecordSummary is a lightweight representation of a directory record. It
// exposes only the fields the TUI renders or filters on; everything else from
// the wire record is discarded by extractSummary.
type RecordSummary struct {
	CID           string
	Name          string
	Version       string
	SchemaVersion string
	Authors       []string
	Skills        []string
	Domains       []string
	Modules       []string
	Status        RecordStatus // lifecycle state; zero = StatusLocal
	StatusError   string       // error message when Status == StatusFailed
	Trusted       bool         // lazydir-only; background-enriched via MatchingCIDs
	Verified      bool         // lazydir-only; background-enriched via MatchingCIDs
}

// Client wraps the agntcy/dir gRPC client.
type Client struct {
	c             *client.Client
	Config        Config
	FirstPageSize int
	BatchSize     int
}

// Connect creates a new connected client.
func Connect(ctx context.Context, cfg Config) (*Client, error) {
	dirCfg := &client.Config{
		ServerAddress: cfg.ServerAddress,
		AuthMode:      cfg.AuthMode,
		TlsSkipVerify: cfg.TLSSkipVerify,
		TlsCAFile:     cfg.TLSCAFile,
		TlsCertFile:   cfg.TLSCertFile,
		TlsKeyFile:    cfg.TLSKeyFile,
		AuthToken:     cfg.AuthToken,
		OIDCIssuer:    cfg.OIDCIssuer,
		OIDCClientID:  cfg.OIDCClientID,
	}

	c, err := client.New(ctx, client.WithConfig(dirCfg))
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", cfg.ServerAddress, err)
	}

	return &Client{c: c, Config: cfg}, nil
}

// Close closes the underlying connection.
func (c *Client) Close() {
	if c.c != nil {
		_ = c.c.Close()
	}
}

// Ping verifies the server is reachable by issuing a lightweight SearchCIDs
// RPC (database-only, no OCI store pull) and waiting for the first response or
// stream completion. This avoids server-side log noise from store pulls being
// interrupted by context cancellation.
func (c *Client) Ping(ctx context.Context) error {
	rpcCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	limit := uint32(1)
	req := &searchv1.SearchCIDsRequest{Limit: &limit}
	result, err := c.c.SearchCIDs(rpcCtx, req)
	if err != nil {
		return err
	}

	resCh := result.ResCh()
	errCh := result.ErrCh()
	for {
		select {
		case _, ok := <-resCh:
			if ok {
				return nil
			}
			resCh = nil
		case err, ok := <-errCh:
			if !ok {
				errCh = nil
				continue
			}
			if err != nil {
				return err
			}
		case <-result.DoneCh():
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// FilterCategory identifies a server-side filter predicate. Each value maps
// 1:1 to a RecordQueryType in the agntcy.dir.search.v1 protobuf API.
type FilterCategory int

const (
	FilterSkill FilterCategory = iota
	FilterDomain
	FilterModule
	FilterSchemaVersion
	FilterVersion
	FilterAuthor
	FilterTrusted
	FilterVerified
)

// Query is one server-side predicate. Multiple Query values combine on the
// server with the semantics defined by the directory implementation.
type Query struct {
	Category FilterCategory
	Value    string
}

func (q Query) toRPC() *searchv1.RecordQuery {
	var t searchv1.RecordQueryType
	switch q.Category {
	case FilterSkill:
		t = searchv1.RecordQueryType_RECORD_QUERY_TYPE_SKILL_NAME
	case FilterDomain:
		t = searchv1.RecordQueryType_RECORD_QUERY_TYPE_DOMAIN_NAME
	case FilterModule:
		t = searchv1.RecordQueryType_RECORD_QUERY_TYPE_MODULE_NAME
	case FilterSchemaVersion:
		t = searchv1.RecordQueryType_RECORD_QUERY_TYPE_SCHEMA_VERSION
	case FilterVersion:
		t = searchv1.RecordQueryType_RECORD_QUERY_TYPE_VERSION
	case FilterAuthor:
		t = searchv1.RecordQueryType_RECORD_QUERY_TYPE_AUTHOR
	case FilterTrusted:
		t = searchv1.RecordQueryType_RECORD_QUERY_TYPE_TRUSTED
	case FilterVerified:
		t = searchv1.RecordQueryType_RECORD_QUERY_TYPE_VERIFIED
	}
	return &searchv1.RecordQuery{Type: t, Value: q.Value}
}

const (
	defaultFirstPageSize = 100
	defaultBatchSize     = 50
)

func (c *Client) firstPageSize() int {
	if c.FirstPageSize > 0 {
		return c.FirstPageSize
	}
	return defaultFirstPageSize
}

func (c *Client) batchSize() int {
	if c.BatchSize > 0 {
		return c.BatchSize
	}
	return defaultBatchSize
}

// StreamCallbacks bundle the optional notification hooks for Stream. Any of
// the callbacks may be nil. They are invoked from the goroutine driving the
// stream — callers must not block inside them.
type StreamCallbacks struct {
	// OnFirstPage fires once after the first batch of records has been
	// received (controlled by Client.FirstPageSize, default 100) or after
	// the stream ends, whichever comes first.
	OnFirstPage func(summaries []*RecordSummary)
	// OnBatch fires for every subsequent batch of records (controlled by
	// Client.BatchSize, default 50). Batching exists so callers can
	// amortize per-update work like UI redraws.
	OnBatch func(summaries []*RecordSummary)
	// OnDone fires exactly once when the stream finishes — either cleanly,
	// because of an error, or because ctx was cancelled. err is nil on a
	// clean finish.
	OnDone func(err error)
}

// Stream issues a single SearchRecords RPC with the supplied queries and
// drains the returned server stream until the server closes it (EOF) or ctx
// is cancelled. The first batch of records (sized by Client.FirstPageSize)
// is delivered via OnFirstPage; remaining records arrive in OnBatch chunks
// (sized by Client.BatchSize).
//
// No limit/offset is set on the RPC — the server decides how many records
// to return. Once the gRPC stream is exhausted, OnDone(nil) fires.
//
// Callbacks fire on this goroutine; cancel ctx to stop reading at any time.
func (c *Client) Stream(ctx context.Context, queries []Query, cb StreamCallbacks) {
	rpcQueries := make([]*searchv1.RecordQuery, 0, len(queries))
	for _, q := range queries {
		rpcQueries = append(rpcQueries, q.toRPC())
	}

	fps := c.firstPageSize()
	bs := c.batchSize()
	buf := make([]*RecordSummary, 0, fps)
	firstPageSent := false

	handOff := func(capHint int) []*RecordSummary {
		out := buf
		buf = make([]*RecordSummary, 0, capHint)
		return out
	}

	flushFirstPage := func() {
		if firstPageSent {
			return
		}
		firstPageSent = true
		batch := handOff(bs)
		if cb.OnFirstPage != nil {
			cb.OnFirstPage(batch)
		}
	}
	flushBatch := func() {
		if !firstPageSent {
			flushFirstPage()
			return
		}
		if len(buf) == 0 {
			return
		}
		batch := handOff(bs)
		if cb.OnBatch != nil {
			cb.OnBatch(batch)
		}
	}
	finish := func(err error) {
		flushBatch()
		if cb.OnDone != nil {
			cb.OnDone(err)
		}
	}

	req := &searchv1.SearchRecordsRequest{
		Queries: rpcQueries,
	}
	result, err := c.c.SearchRecords(ctx, req)
	if err != nil {
		finish(fmt.Errorf("searching records: %w", err))
		return
	}

	for {
		select {
		case resp, ok := <-result.ResCh():
			if !ok {
				finish(nil)
				return
			}
			record := resp.GetRecord()
			if record == nil {
				continue
			}
			s := extractSummary(record)
			if s == nil {
				continue
			}
			buf = append(buf, s)
			if !firstPageSent {
				if len(buf) >= fps {
					flushFirstPage()
				}
				continue
			}
			if len(buf) >= bs {
				flushBatch()
			}
		case streamErr := <-result.ErrCh():
			if streamErr != nil {
				finish(fmt.Errorf("receiving record: %w", streamErr))
				return
			}
		case <-result.DoneCh():
			finish(nil)
			return
		case <-ctx.Done():
			finish(ctx.Err())
			return
		}
	}
}

// MatchingCIDs issues a SearchCIDs RPC with the supplied queries and returns
// the CIDs of every matching record. It drains the CID-only server stream
// until EOF or ctx cancellation. Used to resolve server-only predicates
// (trusted/verified) into a CID set applied to cached records.
func (c *Client) MatchingCIDs(ctx context.Context, queries []Query) ([]string, error) {
	rpcQueries := make([]*searchv1.RecordQuery, 0, len(queries))
	for _, q := range queries {
		rpcQueries = append(rpcQueries, q.toRPC())
	}

	req := &searchv1.SearchCIDsRequest{Queries: rpcQueries}
	result, err := c.c.SearchCIDs(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("searching CIDs: %w", err)
	}

	var cids []string
	for {
		select {
		case resp, ok := <-result.ResCh():
			if !ok {
				return cids, nil
			}
			if resp != nil {
				cids = append(cids, resp.GetRecordCid())
			}
		case streamErr := <-result.ErrCh():
			if streamErr != nil {
				return nil, fmt.Errorf("receiving CID: %w", streamErr)
			}
		case <-result.DoneCh():
			return cids, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// PullJSON fetches a single record by CID and returns it as formatted JSON.
func (c *Client) PullJSON(ctx context.Context, cid string) (string, error) {
	record, err := c.c.Pull(ctx, &corev1.RecordRef{Cid: cid})
	if err != nil {
		return "", fmt.Errorf("pulling record %s: %w", cid, err)
	}

	data := record.GetData()
	if data == nil {
		return "{}", nil
	}

	// Marshal using protojson for proper field names.
	b, err := protojson.MarshalOptions{
		Multiline:       true,
		Indent:          "  ",
		EmitUnpopulated: false,
	}.Marshal(data)
	if err != nil {
		// Fallback to standard JSON if protojson fails.
		b2, err2 := json.MarshalIndent(data, "", "  ")
		if err2 != nil {
			return "", fmt.Errorf("marshaling record to JSON: %w", err)
		}
		return string(b2), nil
	}

	return string(b), nil
}

// RecordInfo mirrors the output of "dirctl info --output json".
type RecordInfo struct {
	CID           string            `json:"cid"`
	Annotations   map[string]string `json:"annotations,omitempty"`
	SchemaVersion string            `json:"schemaVersion,omitempty"`
	CreatedAt     string            `json:"createdAt,omitempty"`
}

// PullInfo fetches a single record by CID and returns its metadata, matching
// the "dirctl info" output.
func (c *Client) PullInfo(ctx context.Context, cid string) (*RecordInfo, error) {
	record, err := c.c.Pull(ctx, &corev1.RecordRef{Cid: cid})
	if err != nil {
		return nil, fmt.Errorf("pulling record %s: %w", cid, err)
	}

	info := &RecordInfo{CID: cid}

	data := record.GetData()
	if data == nil {
		return info, nil
	}

	decoded, err := decoder.DecodeRecord(data)
	if err != nil || decoded == nil {
		return info, nil
	}

	type recordFields interface {
		GetName() string
		GetVersion() string
		GetSchemaVersion() string
		GetAnnotations() map[string]string
		GetCreatedAt() string
	}

	var r recordFields
	switch {
	case decoded.HasV1():
		r = decoded.GetV1()
	case decoded.HasV1Alpha2():
		r = decoded.GetV1Alpha2()
	case decoded.HasV1Alpha1():
		r = decoded.GetV1Alpha1()
	}
	if r == nil {
		return info, nil
	}

	info.SchemaVersion = r.GetSchemaVersion()
	info.CreatedAt = r.GetCreatedAt()
	info.Annotations = map[string]string{}
	if name := r.GetName(); name != "" {
		info.Annotations["name"] = name
	}
	if v := r.GetSchemaVersion(); v != "" {
		info.Annotations["oasf-version"] = v
	}
	if v := r.GetVersion(); v != "" {
		info.Annotations["version"] = v
	}
	for k, v := range r.GetAnnotations() {
		info.Annotations[k] = v
	}

	return info, nil
}

// Delete removes a record from the directory by CID.
func (c *Client) Delete(ctx context.Context, cid string) error {
	if err := c.c.Delete(ctx, &corev1.RecordRef{Cid: cid}); err != nil {
		return fmt.Errorf("deleting record %s: %w", cid, err)
	}
	return nil
}

// CreateSync instructs the server to pull the given CIDs from remoteURL.
// It returns the sync ID which can be polled with GetSyncStatus.
func (c *Client) CreateSync(ctx context.Context, remoteURL string, cids []string) (string, error) {
	syncID, err := c.c.CreateSync(ctx, remoteURL, cids, nil)
	if err != nil {
		return "", fmt.Errorf("creating sync from %s: %w", remoteURL, err)
	}
	return syncID, nil
}

// SyncStatus represents the state of a sync operation.
type SyncStatus int

const (
	SyncPending    SyncStatus = 1
	SyncInProgress SyncStatus = 2
	SyncFailed     SyncStatus = 3
	SyncCompleted  SyncStatus = 6
)

// SyncInfo holds the response from a GetSync call with all available context.
type SyncInfo struct {
	SyncID             string
	Status             SyncStatus
	RemoteDirectoryURL string
	CreatedTime        string
	LastUpdateTime     string
}

// GetSyncInfo returns the full status info for a sync operation.
func (c *Client) GetSyncInfo(ctx context.Context, syncID string) (*SyncInfo, error) {
	resp, err := c.c.GetSync(ctx, syncID)
	if err != nil {
		return nil, fmt.Errorf("getting sync status: %w", err)
	}
	return &SyncInfo{
		SyncID:             resp.GetSyncId(),
		Status:             SyncStatus(resp.GetStatus()),
		RemoteDirectoryURL: resp.GetRemoteDirectoryUrl(),
		CreatedTime:        resp.GetCreatedTime(),
		LastUpdateTime:     resp.GetLastUpdateTime(),
	}, nil
}

// DeleteSync cancels/deletes a sync operation.
func (c *Client) DeleteSync(ctx context.Context, syncID string) error {
	if err := c.c.DeleteSync(ctx, syncID); err != nil {
		return fmt.Errorf("deleting sync %s: %w", syncID, err)
	}
	return nil
}

// namesOf collects the non-empty names from a slice of OASF sub-objects
// (skills, domains, modules), each of which exposes GetName().
func namesOf[T interface{ GetName() string }](items []T) []string {
	if len(items) == 0 {
		return nil
	}
	names := make([]string, 0, len(items))
	for _, it := range items {
		if n := it.GetName(); n != "" {
			names = append(names, n)
		}
	}
	return names
}

// extractSummary pulls name/version/skills/domains/modules from a raw record.
func extractSummary(record *corev1.Record) *RecordSummary {
	cid := record.GetCid()
	data := record.GetData()
	if data == nil {
		return nil
	}

	decoded, err := decoder.DecodeRecord(data)
	if err != nil || decoded == nil {
		return nil
	}

	s := &RecordSummary{CID: cid}

	// Every supported OASF schema version (v1alpha1/0.7.x, v1alpha2/0.8.x,
	// v1/1.x) exposes the same skill/domain/module accessors, so extract them
	// uniformly. Skipping the older versions here left their records out of the
	// filter option lists.
	switch {
	case decoded.HasV1():
		r := decoded.GetV1()
		if r == nil {
			return nil
		}
		s.Name = r.GetName()
		s.Version = r.GetVersion()
		s.SchemaVersion = r.GetSchemaVersion()
		s.Authors = append(s.Authors, r.GetAuthors()...)
		s.Skills = namesOf(r.GetSkills())
		s.Domains = namesOf(r.GetDomains())
		s.Modules = namesOf(r.GetModules())
	case decoded.HasV1Alpha2():
		r := decoded.GetV1Alpha2()
		if r == nil {
			return nil
		}
		s.Name = r.GetName()
		s.Version = r.GetVersion()
		s.SchemaVersion = r.GetSchemaVersion()
		s.Authors = append(s.Authors, r.GetAuthors()...)
		s.Skills = namesOf(r.GetSkills())
		s.Domains = namesOf(r.GetDomains())
		s.Modules = namesOf(r.GetModules())
	case decoded.HasV1Alpha1():
		r := decoded.GetV1Alpha1()
		if r == nil {
			return nil
		}
		s.Name = r.GetName()
		s.Version = r.GetVersion()
		s.SchemaVersion = r.GetSchemaVersion()
		s.Authors = append(s.Authors, r.GetAuthors()...)
		s.Skills = namesOf(r.GetSkills())
		s.Domains = namesOf(r.GetDomains())
		s.Modules = namesOf(r.GetModules())
	default:
		return nil
	}

	if s.Name == "" && cid != "" {
		s.Name = cid[:min(20, len(cid))]
	}

	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}

	return b
}
