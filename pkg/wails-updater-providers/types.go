package wails_updater_providers

// ===================== CNB source =====================

// cnbReleaseListItem is one CNB releases list item (corresponding to the CNB API's
// api.Release element).
type cnbReleaseListItem struct {
	TagName     string            `json:"tag_name"`     // Version tag (may carry a v prefix)
	Name        string            `json:"name"`         // Release title
	Body        string            `json:"body"`         // Release notes (description)
	Prerelease  bool              `json:"prerelease"`   // Whether pre-release
	Draft       bool              `json:"draft"`        // Whether draft
	PublishedAt string            `json:"published_at"` // Publish time (RFC3339)
	CreatedAt   string            `json:"created_at"`   // Creation time (RFC3339)
	IsLatest    bool              `json:"is_latest"`    // Whether latest
	Assets      []cnbReleaseAsset `json:"assets"`       // Asset list
}

// GetPublishedAt implements the publishedAtGetter interface for the generic sort helper.
func (r cnbReleaseListItem) GetPublishedAt() string { return r.PublishedAt }

// cnbReleaseTagDetail is the detail of one CNB release (GetReleaseByTag response).
// Difference from the GitHub detail: CNB additionally returns ID / TagCommitish; GitHub does
// not.
type cnbReleaseTagDetail struct {
	ID           string            `json:"id"`            // CNB-only: unique release ID (absent on GitHub; the API returns it as a string)
	TagName      string            `json:"tag_name"`      // Version tag
	TagCommitish string            `json:"tag_commitish"` // CNB-only: associated commit (GitHub uses target_commitish)
	Name         string            `json:"name"`          // Release title
	Body         string            `json:"body"`          // Release notes
	Prerelease   bool              `json:"prerelease"`    // Whether pre-release
	PublishedAt  string            `json:"published_at"`  // Publish time (RFC3339)
	Assets       []cnbReleaseAsset `json:"assets"`        // Asset list
}

// cnbReleaseAsset is one CNB release asset (an upgrade artifact candidate; CNB-specific
// internal type).
// Note: CNB and GitHub asset fields (name/size) look identical but belong to two independent
// APIs;
// keeping separate types avoids cross-contamination if the two sources' response structures
// diverge later.
type cnbReleaseAsset struct {
	Name string `json:"name"` // File name
	Size int64  `json:"size"` // File size
}

// ===================== GitHub source =====================

// githubRelease is the GitHub release response (corresponding to the GitHub API's release
// object).
type githubRelease struct {
	TagName         string        `json:"tag_name"`         // Version tag
	TargetCommitish string        `json:"target_commitish"` // Target commit (for skipping same-commit nightlies)
	Name            string        `json:"name"`             // Release title
	Body            string        `json:"body"`             // Release notes
	Draft           bool          `json:"draft"`            // Whether draft
	Prerelease      bool          `json:"prerelease"`       // Whether pre-release
	HTMLURL         string        `json:"html_url"`         // Release page URL
	PublishedAt     string        `json:"published_at"`     // Publish time (RFC3339)
	Assets          []githubAsset `json:"assets"`           // Asset list
}

// GetPublishedAt implements the publishedAtGetter interface for the generic sort helper.
func (r githubRelease) GetPublishedAt() string { return r.PublishedAt }

// githubAsset is one GitHub release asset.
type githubAsset struct {
	Name string `json:"name"` // File name
	Size int64  `json:"size"` // File size
}
