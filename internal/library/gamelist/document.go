package gamelist

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"unicode/utf8"
)

// knownFields is the set of ES/Batocera metadata element names that this
// package is allowed to edit. "path" is intentionally absent: it is the ROM
// identity and is not metadata to rewrite in T003-A. "id" is an attribute, not
// a field. Unknown fields remain visible in Entry.Fields and are preserved.
var knownFields = map[string]struct{}{
	"name":             {},
	"desc":             {},
	"genre":            {},
	"tags":             {},
	"sortname":         {},
	"emulator":         {},
	"core":             {},
	"image":            {},
	"video":            {},
	"marquee":          {},
	"thumbnail":        {},
	"fanart":           {},
	"titleshot":        {},
	"manual":           {},
	"magazine":         {},
	"map":              {},
	"bezel":            {},
	"cartridge":        {},
	"boxart":           {},
	"boxback":          {},
	"wheel":            {},
	"mix":              {},
	"rating":           {},
	"releasedate":      {},
	"developer":        {},
	"publisher":        {},
	"family":           {},
	"arcadesystemname": {},
	"players":          {},
	"favorite":         {},
	"hidden":           {},
	"kidgame":          {},
	"playcount":        {},
	"lastplayed":       {},
	"crc32":            {},
	"md5":              {},
	"gametime":         {},
	"lang":             {},
	"region":           {},
	"cheevosHash":      {},
	"cheevosId":        {},
	"multidisk":        {},
}

// Load reads exactly <systemDir>/gamelist.xml. It never searches ES fallback
// locations and never writes. A symlink at gamelist.xml is rejected rather than
// followed.
func Load(systemDir string) (*Document, error) {
	if systemDir == "" {
		return nil, fmt.Errorf("%w: system directory is required", ErrNotFound)
	}
	absolute, err := filepath.Abs(systemDir)
	if err != nil {
		return nil, fmt.Errorf("gamelist: resolve system directory: %w", err)
	}
	path := filepath.Join(absolute, "gamelist.xml")

	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
		}
		if os.IsPermission(err) {
			return nil, fmt.Errorf("%w: %s", ErrUnreadable, path)
		}
		return nil, fmt.Errorf("gamelist: stat %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: %s", ErrSymlink, path)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s", ErrNotRegular, path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsPermission(err) {
			return nil, fmt.Errorf("%w: %s", ErrUnreadable, path)
		}
		return nil, fmt.Errorf("gamelist: read %s: %w", path, err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrEmpty, path)
	}
	doc, err := Parse(absolute, data)
	if err != nil {
		return nil, err
	}
	doc.Path = path
	return doc, nil
}

// EntryCount returns the number of recognised game and folder entries.
func (d *Document) EntryCount() int {
	if d == nil {
		return 0
	}
	return len(d.Entries)
}

// Field returns a copy of the requested field, using the entry namespace and an
// explicit duplicate occurrence. It returns false when the field is absent.
func (d *Document) Field(entryIndex int, name string, occurrence int) (Field, bool) {
	if d == nil || entryIndex < 0 || entryIndex >= len(d.refs) || entryIndex >= len(d.Entries) || occurrence < 0 {
		return Field{}, false
	}
	ref := d.refs[entryIndex]
	for _, field := range ref.fields {
		if field.name.Local == name && field.name.Namespace == ref.el.qname.Namespace && field.ordinal == occurrence {
			return d.Entries[entryIndex].Fields[field.fieldIndex], true
		}
	}
	return Field{}, false
}

// SetField records an in-memory replacement of one existing field's text. The
// occurrence must identify a duplicate explicitly; there is no "first match"
// default. path, id and unknown fields are not editable. Fields containing
// CDATA or mixed content are rejected rather than silently reformatted.
func (d *Document) SetField(entryIndex int, name string, occurrence int, value string) error {
	if d == nil {
		return ErrEntryIndex
	}
	if _, ok := knownFields[name]; !ok {
		return fmt.Errorf("%w: %q", ErrUnknownField, name)
	}
	if !validXMLValue(value) {
		return fmt.Errorf("%w: %s", ErrInvalidValue, name)
	}
	if entryIndex < 0 || entryIndex >= len(d.refs) || entryIndex >= len(d.Entries) {
		return fmt.Errorf("%w: %d", ErrEntryIndex, entryIndex)
	}
	if occurrence < 0 {
		return fmt.Errorf("%w: occurrence %d", ErrFieldNotFound, occurrence)
	}

	ref := &d.refs[entryIndex]
	var matched *fieldRef
	for index := range ref.fields {
		field := &ref.fields[index]
		if field.name.Local == name && field.name.Namespace == ref.el.qname.Namespace && field.ordinal == occurrence {
			matched = field
			break
		}
	}
	if matched == nil {
		return fmt.Errorf("%w: %s[%d]", ErrFieldNotFound, name, occurrence)
	}
	if matched.el == nil {
		return fmt.Errorf("%w: %s[%d]", ErrFieldAdded, name, occurrence)
	}
	if matched.el.complex() {
		return fmt.Errorf("%w: %s[%d]", ErrComplexField, name, occurrence)
	}

	if matched.fieldIndex < 0 || matched.fieldIndex >= len(d.Entries[entryIndex].Fields) {
		return fmt.Errorf("%w: field view no longer matches source", ErrEdit)
	}
	public := &d.Entries[entryIndex].Fields[matched.fieldIndex]

	if value == matched.originalValue {
		public.Value = matched.originalValue
		public.Complex = matched.el.complex()
		public.Added = false
		d.removeReplaceEdit(entryIndex, matched.name, matched.ordinal)
		return nil
	}

	escaped := escapeText(value)
	replacement := escaped
	start, end := matched.el.startTagEnd, matched.el.endTagStart
	if matched.el.selfClosing {
		rawTag := d.source[matched.el.start:matched.el.startTagEnd]
		if !bytes.HasSuffix(rawTag, []byte("/>")) {
			return fmt.Errorf("%w: malformed self-closing field", ErrUnsupportedMutation)
		}
		open := append([]byte(nil), bytes.TrimRight(rawTag[:len(rawTag)-2], " \t\r\n")...)
		open = append(open, '>')
		closeTag := []byte("</" + matched.el.qname.String() + ">")
		replacement = make([]byte, 0, len(open)+len(escaped)+len(closeTag))
		replacement = append(replacement, open...)
		replacement = append(replacement, escaped...)
		replacement = append(replacement, closeTag...)
		start, end = matched.el.start, matched.el.end
	}

	public.Value = value
	public.Complex = false
	public.Added = false
	d.recordReplaceEdit(edit{
		kind:         editReplace,
		start:        start,
		end:          end,
		replacement:  replacement,
		entryIndex:   entryIndex,
		fieldName:    matched.name,
		fieldOrdinal: matched.ordinal,
		newValue:     value,
	})
	return nil
}

// AddField records an in-memory insertion of a new field immediately before an
// entry's closing tag. Existing nodes are never reordered. The new field uses
// the entry's namespace/prefix. Adding to a self-closing entry is rejected in
// T003-A because that would require rewriting the entry's element form.
func (d *Document) AddField(entryIndex int, name, value string) error {
	if d == nil {
		return ErrEntryIndex
	}
	if !validFieldName(name) {
		return fmt.Errorf("%w: %q", ErrInvalidFieldName, name)
	}
	if _, ok := knownFields[name]; !ok {
		return fmt.Errorf("%w: %q", ErrUnknownField, name)
	}
	if !validXMLValue(value) {
		return fmt.Errorf("%w: %s", ErrInvalidValue, name)
	}
	if entryIndex < 0 || entryIndex >= len(d.refs) || entryIndex >= len(d.Entries) {
		return fmt.Errorf("%w: %d", ErrEntryIndex, entryIndex)
	}
	ref := &d.refs[entryIndex]
	if ref.el.selfClosing {
		return fmt.Errorf("%w: entry is self-closing", ErrUnsupportedMutation)
	}

	fieldName := QName{
		Prefix:    ref.el.qname.Prefix,
		Namespace: ref.el.qname.Namespace,
		Local:     name,
	}
	rawName := fieldName.String()
	escaped := escapeText(value)
	insertion := make([]byte, 0, len(rawName)*2+len(escaped)+5)
	insertion = append(insertion, '<')
	insertion = append(insertion, rawName...)
	insertion = append(insertion, '>')
	insertion = append(insertion, escaped...)
	insertion = append(insertion, '<', '/')
	insertion = append(insertion, rawName...)
	insertion = append(insertion, '>')

	// Reuse the indentation already present before </game>. Inserting the new
	// field there changes no existing byte; only the inserted text is new.
	before := d.source[:ref.el.endTagStart]
	lineEnding := []byte("\n")
	if newline := bytes.LastIndexByte(before, '\n'); newline > 0 && before[newline-1] == '\r' {
		lineEnding = []byte("\r\n")
	}
	if newline := bytes.LastIndexByte(before, '\n'); newline >= 0 {
		indent := before[newline+1:]
		if onlyIndent(indent) {
			insertion = append(insertion, lineEnding...)
			insertion = append(insertion, indent...)
		}
	}

	ordinal := 0
	for _, field := range ref.fields {
		if field.name.Local == fieldName.Local && field.name.Namespace == fieldName.Namespace {
			ordinal++
		}
	}
	fieldIndex := len(d.Entries[entryIndex].Fields)
	d.Entries[entryIndex].Fields = append(d.Entries[entryIndex].Fields, Field{
		Name:    fieldName,
		Value:   value,
		Ordinal: ordinal,
		Added:   true,
	})
	ref.fields = append(ref.fields, fieldRef{
		name:       fieldName,
		ordinal:    ordinal,
		fieldIndex: fieldIndex,
		el:         nil,
	})
	d.edits = append(d.edits, edit{
		kind:         editInsert,
		start:        ref.el.endTagStart,
		end:          ref.el.endTagStart,
		replacement:  insertion,
		entryIndex:   entryIndex,
		fieldName:    fieldName,
		fieldOrdinal: ordinal,
		newValue:     value,
		add:          true,
	})
	return nil
}

// Marshal applies pending edits to a copy of the original bytes and returns the
// result. It never writes to disk. The result is strictly re-parsed and every
// byte outside the recorded edit spans is checked against the source.
func (d *Document) Marshal() ([]byte, error) {
	if d == nil {
		return nil, ErrEdit
	}
	if len(d.edits) == 0 {
		return append([]byte(nil), d.source...), nil
	}
	if len(d.Entries) != len(d.refs) {
		return nil, fmt.Errorf("%w: public entries no longer match source index", ErrEdit)
	}

	edits := append([]edit(nil), d.edits...)
	sort.SliceStable(edits, func(i, j int) bool {
		return edits[i].start < edits[j].start
	})
	output, err := applyEdits(d.source, edits)
	if err != nil {
		return nil, err
	}
	if err := verifyUnchangedSegments(d.source, output, edits); err != nil {
		return nil, err
	}

	parsed, err := Parse(d.SystemDir, output)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal produced invalid XML: %v", ErrEdit, err)
	}
	if len(parsed.Entries) != len(d.Entries) {
		return nil, fmt.Errorf("%w: entry count changed from %d to %d", ErrEdit, len(d.Entries), len(parsed.Entries))
	}
	if parsed.Root != d.Root {
		return nil, fmt.Errorf("%w: root changed from %v to %v", ErrEdit, d.Root, parsed.Root)
	}
	for _, pending := range edits {
		entry, ok := findParsedEntry(parsed, pending.entryIndex, pending.fieldName, pending.fieldOrdinal)
		if !ok {
			return nil, fmt.Errorf("%w: edited field %s[%d] missing after reparse", ErrEdit, pending.fieldName.Local, pending.fieldOrdinal)
		}
		if entry.Value != pending.newValue {
			return nil, fmt.Errorf("%w: field %s[%d] value mismatch after reparse", ErrEdit, pending.fieldName.Local, pending.fieldOrdinal)
		}
	}
	return output, nil
}

func findParsedEntry(doc *Document, entryIndex int, name QName, occurrence int) (Field, bool) {
	if entryIndex < 0 || entryIndex >= len(doc.Entries) {
		return Field{}, false
	}
	for _, field := range doc.Entries[entryIndex].Fields {
		if field.Name.Local == name.Local && field.Name.Namespace == name.Namespace && field.Ordinal == occurrence {
			return field, true
		}
	}
	return Field{}, false
}

func applyEdits(source []byte, edits []edit) ([]byte, error) {
	output := make([]byte, 0, len(source))
	position := 0
	for _, pending := range edits {
		if pending.start < position || pending.end < pending.start || pending.end > len(source) {
			return nil, fmt.Errorf("%w: overlapping or out-of-bounds edit", ErrEdit)
		}
		output = append(output, source[position:pending.start]...)
		output = append(output, pending.replacement...)
		position = pending.end
	}
	output = append(output, source[position:]...)
	return output, nil
}

func verifyUnchangedSegments(source, output []byte, edits []edit) error {
	sourcePosition, outputPosition := 0, 0
	for _, pending := range edits {
		if pending.start < sourcePosition || pending.end < pending.start || pending.end > len(source) {
			return fmt.Errorf("%w: invalid edit span", ErrEdit)
		}
		unchanged := pending.start - sourcePosition
		if outputPosition+unchanged > len(output) || !bytes.Equal(source[sourcePosition:pending.start], output[outputPosition:outputPosition+unchanged]) {
			return fmt.Errorf("%w: bytes outside edit span changed", ErrEdit)
		}
		outputPosition += unchanged
		if outputPosition+len(pending.replacement) > len(output) || !bytes.Equal(output[outputPosition:outputPosition+len(pending.replacement)], pending.replacement) {
			return fmt.Errorf("%w: replacement bytes missing", ErrEdit)
		}
		outputPosition += len(pending.replacement)
		sourcePosition = pending.end
	}
	if sourcePosition > len(source) || outputPosition > len(output) {
		return fmt.Errorf("%w: edit bounds escaped source", ErrEdit)
	}
	if !bytes.Equal(source[sourcePosition:], output[outputPosition:]) {
		return fmt.Errorf("%w: trailing bytes changed", ErrEdit)
	}
	return nil
}

func (d *Document) recordReplaceEdit(pending edit) {
	for index := range d.edits {
		existing := &d.edits[index]
		if existing.kind == editReplace && existing.entryIndex == pending.entryIndex &&
			existing.fieldName == pending.fieldName && existing.fieldOrdinal == pending.fieldOrdinal {
			*existing = pending
			return
		}
	}
	d.edits = append(d.edits, pending)
}

func (d *Document) removeReplaceEdit(entryIndex int, name QName, occurrence int) {
	filtered := d.edits[:0]
	for _, pending := range d.edits {
		if pending.kind == editReplace && pending.entryIndex == entryIndex &&
			pending.fieldName == name && pending.fieldOrdinal == occurrence {
			continue
		}
		filtered = append(filtered, pending)
	}
	d.edits = filtered
}

func validXMLValue(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		switch {
		case character == '\t' || character == '\n' || character == '\r':
		case character >= 0x20 && character <= 0xd7ff:
		case character >= 0xe000 && character <= 0xfffd:
		case character >= 0x10000 && character <= 0x10ffff:
		default:
			return false
		}
	}
	return true
}

func escapeText(value string) []byte {
	var buffer bytes.Buffer
	_ = xml.EscapeText(&buffer, []byte(value))
	return buffer.Bytes()
}

func validFieldName(name string) bool {
	if name == "" {
		return false
	}
	for index, character := range name {
		if index == 0 {
			if !isNameStart(character) {
				return false
			}
			continue
		}
		if !isNameChar(character) {
			return false
		}
	}
	return true
}

func isNameStart(character rune) bool {
	return character == '_' || character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character > 0x7f
}

func isNameChar(character rune) bool {
	return isNameStart(character) || character >= '0' && character <= '9' || character == '-' || character == '.'
}

func onlyIndent(data []byte) bool {
	for _, character := range data {
		if character != ' ' && character != '\t' && character != '\r' {
			return false
		}
	}
	return true
}
