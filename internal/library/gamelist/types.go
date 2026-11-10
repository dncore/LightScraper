// Package gamelist provides a source-preserving view and safe writer for an
// EmulationStation/Batocera gamelist.xml document.
//
// Parse/Load index the original bytes without modifying them. SetField and
// AddField record in-memory edits; Marshal applies those edits to a copy of the
// original bytes. Save writes only the private <systemDir>/gamelist.xml target
// via a same-directory temporary file and an exclusive backup.
package gamelist

import (
	"errors"
	"os"
	"time"
)

// Sentinel errors returned by Load, Parse, SetField, AddField and Marshal.
// Callers can use errors.Is to classify failures without parsing an error
// string.
var (
	ErrNotFound            = errors.New("gamelist: file not found")
	ErrEmpty               = errors.New("gamelist: document is empty")
	ErrInvalidXML          = errors.New("gamelist: invalid XML")
	ErrUnsupportedEncoding = errors.New("gamelist: unsupported encoding")
	ErrRoot                = errors.New("gamelist: root element is not gameList")
	ErrSymlink             = errors.New("gamelist: gamelist.xml is a symbolic link")
	ErrNotRegular          = errors.New("gamelist: gamelist.xml is not a regular file")
	ErrUnreadable          = errors.New("gamelist: gamelist.xml is not readable")
	ErrEntryIndex          = errors.New("gamelist: entry index out of range")
	ErrFieldNotFound       = errors.New("gamelist: field not found")
	ErrUnknownField        = errors.New("gamelist: field cannot be edited")
	ErrComplexField        = errors.New("gamelist: field has CDATA or mixed content")
	ErrUnsupportedMutation = errors.New("gamelist: unsupported mutation")
	ErrFieldAdded          = errors.New("gamelist: field was added in memory")
	ErrInvalidFieldName    = errors.New("gamelist: invalid field name")
	ErrInvalidValue        = errors.New("gamelist: value is not valid UTF-8/XML text")
	ErrEdit                = errors.New("gamelist: edit could not be applied")

	ErrSaveNoSource               = errors.New("gamelist: document has no loaded source file")
	ErrInvalidSaveOptions         = errors.New("gamelist: invalid save options")
	ErrSaveTarget                 = errors.New("gamelist: save target is not the loaded gamelist.xml")
	ErrSourceMissing              = errors.New("gamelist: source file disappeared")
	ErrSourceChanged              = errors.New("gamelist: source content changed")
	ErrSourceReplaced             = errors.New("gamelist: source path refers to a different file")
	ErrSourceMetadataChanged      = errors.New("gamelist: source metadata changed")
	ErrSourcePermission           = errors.New("gamelist: source permission changed")
	ErrSourceChangedDuringLoad    = errors.New("gamelist: source changed during load")
	ErrSystemDirChangedDuringLoad = errors.New("gamelist: system directory changed during load")
	ErrSystemDirMissing           = errors.New("gamelist: system directory disappeared")
	ErrSystemDirChanged           = errors.New("gamelist: system directory identity changed")
	ErrSystemDirNotDir            = errors.New("gamelist: system directory is not a directory")
	ErrSaveMarshal                = errors.New("gamelist: marshal validation failed")
	ErrTempCreate                 = errors.New("gamelist: cannot create temporary file")
	ErrTempWrite                  = errors.New("gamelist: cannot write temporary file")
	ErrTempSync                   = errors.New("gamelist: cannot sync temporary file")
	ErrTempValidate               = errors.New("gamelist: temporary file validation failed")
	ErrTempMetadata               = errors.New("gamelist: cannot preserve temporary-file metadata")
	ErrBackupExists               = errors.New("gamelist: backup path already exists")
	ErrBackup                     = errors.New("gamelist: cannot create backup")
	ErrDirSyncBeforeReplace       = errors.New("gamelist: cannot persist backup directory entry")
	ErrDirSyncAfterReplace        = errors.New("gamelist: cannot persist replaced directory entry")
	ErrDirSyncUnsupported         = errors.New("gamelist: directory sync is unsupported")
	ErrReplace                    = errors.New("gamelist: cannot replace gamelist.xml")
	ErrReplaceReportedFailure     = errors.New("gamelist: replace reported failure but target is new")
	ErrSnapshotRefreshFailed      = errors.New("gamelist: cannot refresh document snapshot")
	ErrPostCommitVerify           = errors.New("gamelist: cannot verify target after replace")
	ErrCommitUnknown              = errors.New("gamelist: cannot determine replace result")
	ErrDocumentStale              = errors.New("gamelist: document snapshot is stale")
	ErrCleanup                    = errors.New("gamelist: cannot remove temporary file")
)

// QName identifies an XML element or attribute by its original prefix, its
// resolved namespace URI, and its local name. Namespace is the semantic key;
// Prefix is retained only for rendering and diagnostics.
type QName struct {
	Prefix    string
	Namespace string
	Local     string
}

// String returns the original prefix:local form, or local when unprefixed.
func (n QName) String() string {
	if n.Prefix != "" {
		return n.Prefix + ":" + n.Local
	}
	return n.Local
}

// Attribute is one source element attribute. Attributes are returned in source
// order, including xmlns declarations and unknown attributes.
type Attribute struct {
	Name  QName
	Value string
}

// Field is one direct child element of a game or folder entry. Fields are kept
// in source order; duplicate names are represented by separate Fields with
// distinct Ordinal values.
//
// Complex is true when the element contains CDATA, child elements, comments or
// processing instructions. SetField refuses to edit such a field because doing
// so would silently change its form. The original bytes are still preserved.
type Field struct {
	Name       QName
	Value      string
	Attributes []Attribute
	Ordinal    int
	Complex    bool
	Added      bool
}

// EntryKind classifies a gameList direct child that this package understands.
type EntryKind string

const (
	EntryGame   EntryKind = "game"
	EntryFolder EntryKind = "folder"
)

// Entry is a game or folder entry in a gamelist. Fields contains every direct
// child element, including unknown elements and <scrap>. The original bytes
// remain the source of truth; Entry is a view over that source.
type Entry struct {
	Kind       EntryKind
	Index      int
	Name       QName
	Attributes []Attribute
	Path       PathInfo
	Fields     []Field
}

// RawTopLevel is an unrecognised direct child of <gameList>, such as a future
// ES element. Raw is a copy of the exact source bytes for that element.
type RawTopLevel struct {
	Name QName
	Raw  []byte
}

// PathStatus is the result of lexical ROM path classification. It never
// implies that the path exists or that the package accessed it.
type PathStatus string

const (
	PathInvalid         PathStatus = "invalid"
	PathRelative        PathStatus = "relative_inside_system"
	PathAbsoluteInside  PathStatus = "absolute_inside_system"
	PathOutside         PathStatus = "outside_system"
	PathBareRelative    PathStatus = "bare_relative_unsupported"
	PathHomeUnsupported PathStatus = "home_path_unsupported"
)

// PathInfo contains the exact XML path text and a purely lexical classification
// relative to SystemDir. Resolved is populated only for paths classified as
// relative or absolute-inside; it is never used to open, write or stat a ROM or
// media file.
type PathInfo struct {
	Raw      string
	Status   PathStatus
	Resolved string
}

// Document is an in-memory, source-preserving view of one gamelist.xml.
// SourceHash is the SHA-256 of the exact bytes supplied to Parse or read by
// Load. The original bytes, edit spans and loaded-file identity are private.
type Document struct {
	Path           string
	SystemDir      string
	Root           QName
	RootAttributes []Attribute
	Entries        []Entry
	Other          []RawTopLevel
	SourceHash     [32]byte

	source []byte
	refs   []entryRef
	edits  []edit

	loadedPath     string
	loadedDir      string
	loadedFileInfo os.FileInfo
	loadedHash     [32]byte
	loadedSize     int64
	loadedMode     os.FileMode
	loadedMtime    time.Time
	loadedDirInfo  os.FileInfo
	ops            saveOperations
	stale          bool
	staleReason    error
}

// entryRef maps a public Entry back to its parsed source element and fields.
type entryRef struct {
	index  int
	el     *element
	fields []fieldRef
}

type fieldRef struct {
	name          QName
	ordinal       int
	fieldIndex    int
	el            *element
	originalValue string
}

// edit is a byte replacement or insertion against Document.source. All offsets
// are validated by applyEdits before any bytes are copied.
type edit struct {
	kind         editKind
	start        int
	end          int
	replacement  []byte
	entryIndex   int
	fieldName    QName
	fieldOrdinal int
	newValue     string
	add          bool
}

type editKind uint8

const (
	editReplace editKind = iota
	editInsert
)

// CommitStatus reports whether a Save replace operation is known to have
// happened. It does not describe durability.
type CommitStatus uint8

const (
	CommitNotCommitted CommitStatus = iota
	CommitCommitted
	CommitUnknown
)

// BackupPolicy controls whether Save creates a backup for an existing target.
type BackupPolicy int

const (
	BackupDefault BackupPolicy = iota
	BackupNone
)

// DurabilityPolicy controls how directory-sync capability limitations are
// treated before replacing the target.
type DurabilityPolicy int

const (
	DurabilityBestEffort DurabilityPolicy = iota
	DurabilityStrict
)

// SaveOptions controls backup and directory durability behavior. The zero
// value means BackupDefault and DurabilityBestEffort.
type SaveOptions struct {
	Backup     BackupPolicy
	Durability DurabilityPolicy
}

// SaveResult describes the outcome of a Save, including whether the target
// was replaced and whether the Document was refreshed to the actual target.
// SnapshotCurrent=false on a loaded Document means Stale() is true.
type SaveResult struct {
	Status          CommitStatus
	Changed         bool
	SnapshotCurrent bool
	Path            string
	BackupPath      string
	Warnings        []error
}
