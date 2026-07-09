// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package oasf

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	sdkschema "github.com/agntcy/oasf-sdk/pkg/schema"
)

// DefaultServerAddress is used when the user does not override it.
const DefaultServerAddress = "https://schema.oasf.outshift.com"

// ClassType identifies the OASF taxonomy class type.
type ClassType string

const (
	ClassTypeSkill  ClassType = "skills"
	ClassTypeDomain ClassType = "domains"
	ClassTypeModule ClassType = "modules"
)

// ClassEntry is a lightweight display-oriented record for a taxonomy class.
type ClassEntry struct {
	ID      int
	Name    string
	Caption string
	Version string // OASF schema version this entry was fetched from
}

// ClassInfo holds user-visible info about a taxonomy class, including its
// position in the hierarchy.
type ClassInfo struct {
	ID          int
	Name        string
	Caption     string
	Description string
	Type        ClassType
	Ancestors   []ClassEntry // path from the taxonomy root to the parent (not including the item)
}

// Config configures the OASF schema client.
type Config struct {
	// ServerAddress is the base URL of the OASF schema server, e.g.
	// "https://schema.oasf.outshift.com". Protocol is optional.
	ServerAddress string
	// Timeout is the HTTP request timeout in seconds. Zero or negative
	// falls back to the default of 10 seconds.
	Timeout int
}

const defaultTimeout = 10

// Client wraps the OASF SDK schema client and caches class info lookups.
type Client struct {
	cfg    Config
	sdk    *sdkschema.Schema
	cacheM sync.Mutex
	cache  map[string]*ClassInfo
}

// NewClient constructs a new OASF client backed by the oasf-sdk/pkg/schema.
// The SDK performs URL normalization, so http/https schemes are handled.
func NewClient(cfg Config) (*Client, error) {
	if cfg.ServerAddress == "" {
		return nil, errors.New("OASF server address is required")
	}

	sdk, err := sdkschema.New(cfg.ServerAddress, sdkschema.WithCache(true))
	if err != nil {
		return nil, fmt.Errorf("creating OASF schema client: %w", err)
	}

	return &Client{
		cfg:   cfg,
		sdk:   sdk,
		cache: map[string]*ClassInfo{},
	}, nil
}

// ServerAddress returns the URL the client is currently pointed at.
func (c *Client) ServerAddress() string {
	return c.cfg.ServerAddress
}

// Ping verifies that the OASF schema server is reachable by issuing a
// lightweight taxonomy fetch. The result is cached by the SDK, so
// subsequent real fetches benefit from the warm cache.
func (c *Client) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	_, err := c.sdk.GetSchemaSkills(ctx)
	return err
}

func (c *Client) timeout() time.Duration {
	if c.cfg.Timeout > 0 {
		return time.Duration(c.cfg.Timeout) * time.Second
	}
	return defaultTimeout * time.Second
}

// Fetch retrieves class info (including ancestors) for a taxonomy class by
// name. schemaVersion may be empty to use the server default.
func (c *Client) Fetch(ctx context.Context, classType ClassType, name, schemaVersion string) (*ClassInfo, error) {
	key := string(classType) + "|" + name + "|" + schemaVersion
	c.cacheM.Lock()
	if v, ok := c.cache[key]; ok {
		c.cacheM.Unlock()
		return v, nil
	}
	c.cacheM.Unlock()

	reqCtx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()

	taxonomy, err := c.getTaxonomyVersioned(reqCtx, classType, schemaVersion)
	if err != nil {
		return nil, err
	}

	item, ancestors, ok := findItemWithPath(map[string]sdkschema.TaxonomyItem(taxonomy), name)
	if !ok {
		return nil, fmt.Errorf("class %q not found in OASF %s taxonomy", name, classType)
	}

	info := &ClassInfo{
		ID:          item.ID,
		Name:        item.Name,
		Caption:     item.Caption,
		Description: item.Description,
		Type:        classType,
		Ancestors:   ancestors,
	}
	if info.Description == "" {
		info.Description = "No description available."
	}

	c.cacheM.Lock()
	c.cache[key] = info
	c.cacheM.Unlock()

	return info, nil
}

// FetchAll returns a flat map of name → ClassEntry for every item in the
// taxonomy of the given type. schemaVersion may be empty to use the default.
func (c *Client) FetchAll(ctx context.Context, classType ClassType, schemaVersion string) (map[string]ClassEntry, error) {
	reqCtx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()

	taxonomy, err := c.getTaxonomyVersioned(reqCtx, classType, schemaVersion)
	if err != nil {
		return nil, err
	}

	entries := map[string]ClassEntry{}
	collectEntries(map[string]sdkschema.TaxonomyItem(taxonomy), entries)
	// Stamp the source schema version so callers can resolve version-specific
	// details (e.g. descriptions) for values that only exist in this version.
	for k, e := range entries {
		e.Version = schemaVersion
		entries[k] = e
	}
	return entries, nil
}

func collectEntries(items map[string]sdkschema.TaxonomyItem, out map[string]ClassEntry) {
	for _, item := range items {
		out[item.Name] = ClassEntry{
			ID:      item.ID,
			Name:    item.Name,
			Caption: item.Caption,
		}
		if len(item.Classes) > 0 {
			collectEntries(item.Classes, out)
		}
	}
}

func (c *Client) getTaxonomyVersioned(ctx context.Context, classType ClassType, schemaVersion string) (sdkschema.Taxonomy, error) {
	var opts []sdkschema.SchemaOption
	if schemaVersion != "" {
		opts = append(opts, sdkschema.WithSchemaVersion(schemaVersion))
	}
	switch classType {
	case ClassTypeSkill:
		return c.sdk.GetSchemaSkills(ctx, opts...)
	case ClassTypeDomain:
		return c.sdk.GetSchemaDomains(ctx, opts...)
	case ClassTypeModule:
		return c.sdk.GetSchemaModules(ctx, opts...)
	default:
		return nil, fmt.Errorf("unknown OASF class type %q", classType)
	}
}

// findItemWithPath walks the taxonomy tree and returns the matching item
// together with the ancestor chain from the root to its parent.
func findItemWithPath(items map[string]sdkschema.TaxonomyItem, name string) (sdkschema.TaxonomyItem, []ClassEntry, bool) {
	return findInItemsWithPath(items, name, nil)
}

func findInItemsWithPath(items map[string]sdkschema.TaxonomyItem, name string, path []ClassEntry) (sdkschema.TaxonomyItem, []ClassEntry, bool) {
	for _, item := range items {
		if item.Name == name {
			return item, path, true
		}
		if len(item.Classes) > 0 {
			deeper := append(append([]ClassEntry(nil), path...), ClassEntry{
				ID:      item.ID,
				Name:    item.Name,
				Caption: item.Caption,
			})
			if child, childPath, ok := findInItemsWithPath(item.Classes, name, deeper); ok {
				return child, childPath, true
			}
		}
	}
	return sdkschema.TaxonomyItem{}, nil, false
}
