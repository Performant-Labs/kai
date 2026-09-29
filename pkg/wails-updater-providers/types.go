package wails_updater_providers

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
