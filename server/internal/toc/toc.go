// Package toc reads a CMS Transparency in Coverage table-of-contents (index)
// file and resolves which in-network files to load. The index is the source
// of truth for file URLs; nothing else is hard-coded.
package toc

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
)

type Index struct {
	ReportingEntityName string      `json:"reporting_entity_name"`
	ReportingEntityType string      `json:"reporting_entity_type"`
	LastUpdatedOn       string      `json:"last_updated_on"`
	Version             string      `json:"version"`
	ReportingStructure  []Structure `json:"reporting_structure"`
}

type Structure struct {
	ReportingPlans    []Plan         `json:"reporting_plans"`
	InNetworkFiles    []FileLocation `json:"in_network_files"`
	AllowedAmountFile *FileLocation  `json:"allowed_amount_file"`
}

type Plan struct {
	PlanName       string `json:"plan_name"`
	PlanIDType     string `json:"plan_id_type"`
	PlanID         string `json:"plan_id"`
	PlanMarketType string `json:"plan_market_type"`
	IssuerName     string `json:"issuer_name"`
}

type FileLocation struct {
	Description string `json:"description"`
	Location    string `json:"location"`
}

// Parse decodes an index file. Index files are small (KBs), so a plain decode
// is fine here; only the in-network files need streaming.
func Parse(r io.Reader) (*Index, error) {
	var idx Index
	if err := json.NewDecoder(r).Decode(&idx); err != nil {
		return nil, fmt.Errorf("decode index: %w", err)
	}
	if len(idx.ReportingStructure) == 0 {
		return nil, fmt.Errorf("index has no reporting_structure entries")
	}
	return &idx, nil
}

// InNetworkFile is one in-network file selected from the index.
type InNetworkFile struct {
	StructureIndex int
	URL            string
	ShortName      string
}

// InNetworkFiles lists every distinct in-network file in the index, in order.
func (idx *Index) InNetworkFiles() []InNetworkFile {
	var out []InNetworkFile
	seen := map[string]bool{}
	for i, s := range idx.ReportingStructure {
		for _, f := range s.InNetworkFiles {
			loc := strings.TrimSpace(f.Location)
			if loc == "" || seen[loc] {
				continue
			}
			seen[loc] = true
			out = append(out, InNetworkFile{StructureIndex: i, URL: loc, ShortName: ShortName(loc)})
		}
	}
	return out
}

// Select filters files by short name. An empty selection or "all" keeps every
// file. Unknown names are returned so the caller can report them.
func Select(files []InNetworkFile, names []string) (selected []InNetworkFile, unknown []string) {
	if len(names) == 0 || (len(names) == 1 && strings.EqualFold(names[0], "all")) {
		return files, nil
	}
	for _, n := range names {
		found := false
		for _, f := range files {
			if strings.EqualFold(f.ShortName, n) {
				selected = append(selected, f)
				found = true
			}
		}
		if !found {
			unknown = append(unknown, n)
		}
	}
	return selected, unknown
}

// ShortName derives a readable id from a CMS file name. CMS names files
// <date>_<entity>_<plan>_<type>.json, so the plan segment is second to last:
//
//	2026-08-31_centene-management-company-llc_fidelis-ex_in-network.json -> fidelis-ex
func ShortName(location string) string {
	base := location
	if u, err := url.Parse(location); err == nil && u.Path != "" {
		base = u.Path
	}
	base = path.Base(strings.ReplaceAll(base, "\\", "/"))
	if unescaped, err := url.PathUnescape(base); err == nil {
		base = unescaped
	}
	base = strings.TrimSuffix(base, path.Ext(base))
	parts := strings.Split(base, "_")
	if len(parts) >= 4 {
		return parts[len(parts)-2]
	}
	return base
}
