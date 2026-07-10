package boundaryguard

// This file is the machine-readable source of truth for Streamclone clipper
// responsibility ownership. After the product-boundary lock, Streamclone owns
// ZERO clipper/ReplayForge product surfaces — Clip Studio lives in sibling
// replayforge; analytics moments live in private streampulse-backend.
//
// The boundary guard still fails CI if clip render/transcription/editor/export
// symbols reappear under cmd/ or internal/.

// ClipperResponsibility describes a clipper-related responsibility. Kept for
// historical test helpers; the allow-list is intentionally empty.
type ClipperResponsibility struct {
	Name          string
	Requirement   string
	Description   string
	RoutePatterns []string
	PackageHints  []string
}

// ClipperResponsibilityAllowList is empty: Streamclone does not ship Clip Studio,
// /studio redirects, or /v1/clipper proxy surfaces.
var ClipperResponsibilityAllowList = []ClipperResponsibility{}

// AllowedResponsibilityNames returns the names of permitted clipper-related
// responsibilities (none after the boundary lock).
func AllowedResponsibilityNames() []string {
	names := make([]string, 0, len(ClipperResponsibilityAllowList))
	for _, r := range ClipperResponsibilityAllowList {
		names = append(names, r.Name)
	}
	return names
}
