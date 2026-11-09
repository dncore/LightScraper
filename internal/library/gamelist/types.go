// Package gamelist provides a read-only, source-preserving view of an
// EmulationStation/Batocera gamelist.xml document.
//
// T003-A deliberately does not write files. Parse and Load only read and index
// the original bytes; SetField and AddField record in-memory edits; Marshal
// applies those edits to a copy of the original bytes and returns the result.
package gamelist

import (
	"errors"
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
// Load. The original bytes and edit spans are private.
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
