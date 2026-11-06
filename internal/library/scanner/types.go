package scanner

import "time"

// ScanResult is the in-memory result of one read-only ROM library scan.
// It is intentionally not persisted by this package.
type ScanResult struct {
	Root       string
	StartedAt  time.Time
	FinishedAt time.Time
	Systems    []SystemResult
	Issues     []ScanIssue
	Stats      ScanStats
	Cancelled  bool
}

// SystemResult is the result for one immediate child directory of Root.
// A system may have no ROMs and still appear here when its _info.txt is
// missing or invalid; the accompanying issue explains why.
type SystemResult struct {
	ID         string
	Path       string
	Extensions []string
	ROMCount   int
	ROMs       []ROMEntry
	Issues     []ScanIssue
}

// ROMEntry describes one regular file whose extension is advertised by the
// system's _info.txt. DisplayName is only the filename without its final
// extension; it is not a verified game title.
type ROMEntry struct {
	SystemID     string
	Name         string
	AbsolutePath string
	RelativePath string
	Extension    string
	SizeBytes    int64
	ModTime      time.Time
	DisplayName  string
}

// ScanIssue describes a non-fatal problem encountered while scanning a system
// or one of its files. Root-level failures are returned as errors instead.
type ScanIssue struct {
	SystemID string
	Path     string
	Phase    IssuePhase
	Kind     IssueKind
	Message  string
}

// IssuePhase identifies the stage that produced an issue.
type IssuePhase string

const (
	PhaseSystem IssuePhase = "system"
	PhaseInfo   IssuePhase = "info"
	PhaseWalk   IssuePhase = "walk"
	PhaseStat   IssuePhase = "stat"
)

// IssueKind identifies the category of a non-fatal scan issue.
type IssueKind string

const (
	IssueInfoMissing    IssueKind = "info_missing"
	IssueInfoUnreadable IssueKind = "info_unreadable"
	IssueInfoInvalid    IssueKind = "info_invalid"
	IssueInfoSymlink    IssueKind = "info_symlink"
	IssueInfoNotRegular IssueKind = "info_not_regular"
	IssueWalk           IssueKind = "walk_error"
	IssueEntryStat      IssueKind = "entry_stat_failed"
)

// ScanStats summarizes scan work and intentional skips.
type ScanStats struct {
	Systems         int
	ROMFiles        int
	SkippedHidden   int
	SkippedSymlinks int
	SkippedBios     int
	Issues          int
}
