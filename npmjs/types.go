package npmjs

// Package is the primary record emitted by the package command.
type Package struct {
	Name        string   `kit:"id" json:"name"`
	Version     string   `json:"version"`    // latest version
	Description string   `json:"description"`
	License     string   `json:"license"`
	Homepage    string   `json:"homepage"`
	Repository  string   `json:"repository"` // extracted from repository.url
	Keywords    []string `json:"keywords"`
	Author      string   `json:"author"` // extracted from author.name or author string
}

// SearchResult is one record emitted by the search command.
type SearchResult struct {
	Name        string  `kit:"id" json:"name"`
	Version     string  `json:"version"`
	Description string  `json:"description"`
	Score       float64 `json:"score"`
	Date        string  `json:"date"`
}

// PackageVersion is one record emitted by the version command.
type PackageVersion struct {
	Name         string `kit:"id" json:"name"`
	Version      string `json:"version"`
	License      string `json:"license"`
	Main         string `json:"main"`
	UnpackedSize int64  `json:"unpacked_size"`
}
