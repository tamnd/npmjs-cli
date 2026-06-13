package npmjs

import (
	"encoding/json"
	"sort"
)

// Package is the primary record emitted by search and package commands.
type Package struct {
	Rank        int    `json:"rank"        table:"RANK"`
	Name        string `json:"name"        table:"NAME"`
	Version     string `json:"version"     table:"VERSION"`
	Description string `json:"description" table:"DESCRIPTION"`
	Author      string `json:"author"      table:"AUTHOR"`
	License     string `json:"license"     table:"LICENSE"`
	Downloads   int64  `json:"downloads"   table:"DOWNLOADS"`
	Updated     string `json:"updated"     table:"UPDATED"`
	URL         string `json:"url"         table:"URL"`
}

// VersionInfo is one record per published version, emitted by the versions command.
type VersionInfo struct {
	Version     string `json:"version"     table:"VERSION"`
	Date        string `json:"date"        table:"DATE"`
	Description string `json:"description" table:"DESCRIPTION"`
}

// Dep is one dependency entry, emitted by the deps command.
type Dep struct {
	Name  string `json:"name"  table:"NAME"`
	Range string `json:"range" table:"RANGE"`
	Kind  string `json:"kind"  table:"KIND"`
}

// DownloadStat is one time-window record, emitted by the downloads command.
type DownloadStat struct {
	Package   string `json:"package"   table:"PACKAGE"`
	Period    string `json:"period"    table:"PERIOD"`
	Downloads int64  `json:"downloads" table:"DOWNLOADS"`
	Start     string `json:"start"     table:"START"`
	End       string `json:"end"       table:"END"`
}

// ─── wire types ──────────────────────────────────────────────────────────────

// searchResp is the top-level response from /-/v1/search.
type searchResp struct {
	Objects []searchObject `json:"objects"`
	Total   int            `json:"total"`
}

type searchObject struct {
	Package searchPkg   `json:"package"`
	Score   searchScore `json:"score"`
}

type searchPkg struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Date        string            `json:"date"`
	Links       map[string]string `json:"links"`
	Publisher   searchPublisher   `json:"publisher"`
}

type searchPublisher struct {
	Username string `json:"username"`
}

type searchScore struct {
	Final  float64           `json:"final"`
	Detail searchScoreDetail `json:"detail"`
}

type searchScoreDetail struct {
	Quality     float64 `json:"quality"`
	Popularity  float64 `json:"popularity"`
	Maintenance float64 `json:"maintenance"`
}

// pkgDoc is the full package document from /{name}.
type pkgDoc struct {
	Name        string                     `json:"name"`
	Description string                     `json:"description"`
	DistTags    map[string]string          `json:"dist-tags"`
	Versions    map[string]versionManifest `json:"versions"`
	Time        map[string]string          `json:"time"`
	Author      json.RawMessage            `json:"author"`
	License     json.RawMessage            `json:"license"`
	Homepage    string                     `json:"homepage"`
}

// versionManifest is one entry in pkgDoc.Versions.
type versionManifest struct {
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	Description          string            `json:"description"`
	Dependencies         map[string]string `json:"dependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	PeerDependencies     map[string]string `json:"peerDependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
}

// dlResp is the response from the downloads point API.
type dlResp struct {
	Downloads int64  `json:"downloads"`
	Start     string `json:"start"`
	End       string `json:"end"`
	Package   string `json:"package"`
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// npmURL returns the canonical npm web URL for a package.
func npmURL(name string) string {
	return "https://www.npmjs.com/package/" + name
}

// extractAuthor extracts a human-readable author string from a raw JSON author field.
// The author field can be a string or an object with a "name" key.
func extractAuthor(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// try object first
	var obj struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil && obj.Name != "" {
		return obj.Name
	}
	// try string
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return ""
}

// extractLicense extracts a license string from a raw JSON license field.
// The license field can be a string or an object with a "type" key.
func extractLicense(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// try string first (most common)
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// try object
	var obj struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		return obj.Type
	}
	return ""
}

// wireSearchToPackages converts a search response to Package records.
func wireSearchToPackages(resp searchResp, limit int) []Package {
	n := len(resp.Objects)
	if limit > 0 && limit < n {
		n = limit
	}
	out := make([]Package, 0, n)
	for i := 0; i < n; i++ {
		obj := resp.Objects[i]
		pkg := obj.Package
		npmLink := pkg.Links["npm"]
		if npmLink == "" {
			npmLink = npmURL(pkg.Name)
		}
		out = append(out, Package{
			Rank:        i + 1,
			Name:        pkg.Name,
			Version:     pkg.Version,
			Description: pkg.Description,
			Author:      pkg.Publisher.Username,
			License:     "",
			Downloads:   0,
			Updated:     pkg.Date,
			URL:         npmLink,
		})
	}
	return out
}

// wireDocToPackage converts a full package document to one Package record.
func wireDocToPackage(doc pkgDoc) Package {
	latest := doc.DistTags["latest"]
	version := latest
	description := doc.Description
	if v, ok := doc.Versions[latest]; ok && description == "" {
		description = v.Description
	}
	updated := doc.Time["modified"]
	return Package{
		Rank:        0,
		Name:        doc.Name,
		Version:     version,
		Description: description,
		Author:      extractAuthor(doc.Author),
		License:     extractLicense(doc.License),
		Downloads:   0,
		Updated:     updated,
		URL:         npmURL(doc.Name),
	}
}

// wireDocToVersions converts a package document to a sorted list of VersionInfo.
func wireDocToVersions(doc pkgDoc, limit int) []VersionInfo {
	type entry struct {
		version string
		date    string
	}
	var entries []entry
	for k, v := range doc.Time {
		if k == "created" || k == "modified" {
			continue
		}
		entries = append(entries, entry{version: k, date: v})
	}
	// sort newest first
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].date > entries[j].date
	})
	n := len(entries)
	if limit > 0 && limit < n {
		n = limit
	}
	out := make([]VersionInfo, 0, n)
	for i := 0; i < n; i++ {
		e := entries[i]
		desc := ""
		if m, ok := doc.Versions[e.version]; ok {
			desc = m.Description
		}
		out = append(out, VersionInfo{
			Version:     e.version,
			Date:        e.date,
			Description: desc,
		})
	}
	return out
}

// wireManifestToDeps converts a version manifest to Dep records.
func wireManifestToDeps(manifest versionManifest) []Dep {
	var out []Dep
	for name, rangeStr := range manifest.Dependencies {
		out = append(out, Dep{Name: name, Range: rangeStr, Kind: "runtime"})
	}
	for name, rangeStr := range manifest.DevDependencies {
		out = append(out, Dep{Name: name, Range: rangeStr, Kind: "dev"})
	}
	for name, rangeStr := range manifest.PeerDependencies {
		out = append(out, Dep{Name: name, Range: rangeStr, Kind: "peer"})
	}
	for name, rangeStr := range manifest.OptionalDependencies {
		out = append(out, Dep{Name: name, Range: rangeStr, Kind: "optional"})
	}
	kindOrder := map[string]int{"runtime": 0, "dev": 1, "peer": 2, "optional": 3}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return kindOrder[out[i].Kind] < kindOrder[out[j].Kind]
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// wireDownloadToStat converts a download response to a DownloadStat record.
func wireDownloadToStat(resp dlResp, period string) DownloadStat {
	name := resp.Package
	return DownloadStat{
		Package:   name,
		Period:    period,
		Downloads: resp.Downloads,
		Start:     resp.Start,
		End:       resp.End,
	}
}
