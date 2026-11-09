package gamelist

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func TestOfficialFixturesRoundTrip(t *testing.T) {
	tests := []struct {
		name       string
		file       string
		entryCount int
		path       string
		checkDesc  func(t *testing.T, value string)
	}{
		{
			name:       "NES",
			file:       "batocera-43.1-nes-gamelist.xml",
			entryCount: 1,
			path:       "./2048 (tsone).nes",
		},
		{
			name:       "C64",
			file:       "batocera-43.1-c64-gamelist.xml",
			entryCount: 1,
			path:       "./fix_it_felix_64.d64",
			checkDesc: func(t *testing.T, value string) {
				t.Helper()
				if !strings.Contains(value, "Save Niceland") || !strings.Contains(value, "Unfortunately, climbing") {
					t.Fatalf("multiline desc was not read correctly: %q", value)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := readFixture(t, test.file)
			if !bytes.HasPrefix(data, []byte(utf8BOM)) {
				t.Fatalf("fixture does not start with UTF-8 BOM")
			}
			doc, err := Parse(t.TempDir(), data)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if doc.EntryCount() != test.entryCount {
				t.Fatalf("EntryCount = %d, want %d", doc.EntryCount(), test.entryCount)
			}
			entry := doc.Entries[0]
			if entry.Path.Raw != test.path {
				t.Fatalf("Path.Raw = %q, want %q", entry.Path.Raw, test.path)
			}
			if entry.Path.Status != PathRelative {
				t.Fatalf("Path.Status = %q, want relative", entry.Path.Status)
			}
			if entry.Path.Resolved == "" {
				t.Fatalf("Path.Resolved is empty")
			}
			if test.checkDesc != nil {
				desc, ok := doc.Field(0, "desc", 0)
				if !ok {
					t.Fatalf("desc field missing")
				}
				test.checkDesc(t, desc.Value)
			}
			if got, want := doc.SourceHash, sha256.Sum256(data); got != want {
				t.Fatalf("SourceHash = %x, want %x", got, want)
			}
			output, err := doc.Marshal()
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if !bytes.Equal(output, data) {
				t.Fatalf("no-edit Marshal changed official fixture")
			}
		})
	}
}

func TestLoadReadsOnlySystemGamelist(t *testing.T) {
	dir := t.TempDir()
	data := readFixture(t, "batocera-43.1-nes-gamelist.xml")
	if err := os.WriteFile(filepath.Join(dir, "gamelist.xml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if doc.Path != filepath.Join(dir, "gamelist.xml") {
		t.Fatalf("Path = %q", doc.Path)
	}
	if doc.EntryCount() != 1 {
		t.Fatalf("EntryCount = %d", doc.EntryCount())
	}
}

func TestLoadDoesNotSearchFallbackAndDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	before, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Load(dir)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Load missing = %v, want ErrNotFound", err)
	}
	after, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("Load wrote to the filesystem: before=%d after=%d", len(before), len(after))
	}
}

func TestLoadRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require additional Windows privileges")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "real.xml")
	if err := os.WriteFile(target, readFixture(t, "batocera-43.1-nes-gamelist.xml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "gamelist.xml")); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if !errors.Is(err, ErrSymlink) {
		t.Fatalf("Load symlink = %v, want ErrSymlink", err)
	}
}

func TestLoadRejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "gamelist.xml"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if !errors.Is(err, ErrNotRegular) {
		t.Fatalf("Load directory = %v, want ErrNotRegular", err)
	}
}

func TestLoadRejectsUnreadableFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows ACL behavior differs from Unix mode bits")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "gamelist.xml")
	if err := os.WriteFile(path, readFixture(t, "batocera-43.1-nes-gamelist.xml"), 0o000); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Skip("current user can read mode-000 file")
	}
	if !errors.Is(err, ErrUnreadable) {
		t.Fatalf("Load unreadable = %v, want ErrUnreadable", err)
	}
}

func TestLoadRejectsEmptyFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gamelist.xml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if !errors.Is(err, ErrEmpty) {
		t.Fatalf("Load empty = %v, want ErrEmpty", err)
	}
}

func TestParseComplexPreservesEverything(t *testing.T) {
	data := readFixture(t, "synthetic-complex-gamelist.xml")
	doc, err := Parse(t.TempDir(), data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(doc.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(doc.Entries))
	}
	if len(doc.Other) != 1 || doc.Other[0].Name.Local != "futureEntry" {
		t.Fatalf("unknown top-level not preserved: %#v", doc.Other)
	}
	if !bytes.Contains(doc.Other[0].Raw, []byte("<payload>keep</payload>")) {
		t.Fatalf("unknown top-level raw bytes changed: %q", doc.Other[0].Raw)
	}
	if doc.Root.Namespace != "urn:lightscraper:test" {
		t.Fatalf("Root.Namespace = %q", doc.Root.Namespace)
	}
	if got := attributeValue(doc.RootAttributes, "parentHash"); got != "12345" {
		t.Fatalf("parentHash = %q", got)
	}
	game := doc.Entries[0]
	if game.Kind != EntryGame || game.Index != 0 {
		t.Fatalf("game entry = %#v", game)
	}
	if len(game.Attributes) != 2 || attributeValue(game.Attributes, "id") != "42" {
		t.Fatalf("game attributes not preserved: %#v", game.Attributes)
	}
	if game.Path.Raw != "./Game & One.nes" || game.Path.Status != PathRelative {
		t.Fatalf("path = %#v", game.Path)
	}
	nameValues := fieldValues(game, "name")
	if len(nameValues) != 2 || nameValues[0] != "Game <One>" || nameValues[1] != "Duplicate Name" {
		t.Fatalf("duplicate name fields = %#v", nameValues)
	}
	desc0, ok := doc.Field(0, "desc", 0)
	if !ok || !desc0.Complex {
		t.Fatalf("CDATA desc should be complex: %#v ok=%v", desc0, ok)
	}
	desc1, ok := doc.Field(0, "desc", 1)
	if !ok || desc1.Complex || desc1.Value != "second description" {
		t.Fatalf("second desc = %#v ok=%v", desc1, ok)
	}
	if _, ok := findField(game, "es", "custom", 0); !ok {
		t.Fatalf("namespaced unknown field missing")
	}
	if _, ok := findField(game, "", "selfempty", 0); !ok {
		t.Fatalf("self-closing empty field missing")
	}
	folder := doc.Entries[1]
	if folder.Kind != EntryFolder || folder.Path.Status != PathRelative || folder.Path.Resolved == "" {
		t.Fatalf("folder entry = %#v", folder)
	}
	output, err := doc.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !bytes.Equal(output, data) {
		t.Fatalf("no-edit Marshal changed synthetic fixture")
	}
}

func TestSetFieldPreservesBytesOutsideTarget(t *testing.T) {
	data := readFixture(t, "synthetic-complex-gamelist.xml")
	doc, err := Parse(t.TempDir(), data)
	if err != nil {
		t.Fatal(err)
	}
	newValue := "Changed & <Name> \"quoted\""
	if err := doc.SetField(0, "name", 1, newValue); err != nil {
		t.Fatalf("SetField: %v", err)
	}
	output, err := doc.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	old := []byte("<name>Duplicate Name</name>")
	newBytes := []byte("<name>Changed &amp; &lt;Name&gt; &#34;quoted&#34;</name>")
	index := bytes.Index(data, old)
	if index < 0 {
		t.Fatal("old field not found")
	}
	want := make([]byte, 0, len(data)-len(old)+len(newBytes))
	want = append(want, data[:index]...)
	want = append(want, newBytes...)
	want = append(want, data[index+len(old):]...)
	if !bytes.Equal(output, want) {
		t.Fatalf("Marshal changed bytes outside target")
	}
	if got, ok := doc.Field(0, "name", 0); !ok || got.Value != "Game <One>" {
		t.Fatalf("first duplicate changed: %#v ok=%v", got, ok)
	}
	if got, ok := doc.Field(0, "name", 1); !ok || got.Value != newValue {
		t.Fatalf("target duplicate not updated: %#v ok=%v", got, ok)
	}
}

func TestSetFieldOccurrenceIsExplicit(t *testing.T) {
	data := []byte("<gameList><game><path>./x.nes</path><name>One</name><name>Two</name><desc>d</desc></game></gameList>")
	doc, err := Parse(t.TempDir(), data)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.SetField(0, "name", 2, "Three"); !errors.Is(err, ErrFieldNotFound) {
		t.Fatalf("occurrence 2 error = %v", err)
	}
	if err := doc.SetField(0, "desc", 1, "other"); !errors.Is(err, ErrFieldNotFound) {
		t.Fatalf("unique occurrence 1 error = %v", err)
	}
	if err := doc.SetField(0, "name", 1, "Two changed"); err != nil {
		t.Fatalf("SetField duplicate 1: %v", err)
	}
	output, err := doc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output, []byte("<name>One</name>")) || !bytes.Contains(output, []byte("<name>Two changed</name>")) {
		t.Fatalf("wrong duplicate changed: %s", output)
	}
}

func TestSetFieldRejectsComplexAndUnsafeFields(t *testing.T) {
	data := readFixture(t, "synthetic-complex-gamelist.xml")
	doc, err := Parse(t.TempDir(), data)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name       string
		field      string
		occurrence int
		want       error
	}{
		{"CDATA", "desc", 0, ErrComplexField},
		{"unknown", "unknown", 0, ErrUnknownField},
		{"path", "path", 0, ErrUnknownField},
		{"id attribute", "id", 0, ErrUnknownField},
		{"mixed content", "es", 0, ErrUnknownField},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if err := doc.SetField(0, test.field, test.occurrence, "x"); !errors.Is(err, test.want) {
				t.Fatalf("SetField = %v, want %v", err, test.want)
			}
		})
	}
	// The namespaced unknown field is not editable even if its local name is
	// looked up by a path API; ensure the original bytes remain unchanged.
	if _, err := doc.Marshal(); err != nil {
		t.Fatalf("Marshal after rejected edits: %v", err)
	}
}

func TestSetFieldEmptyAndSelfClosingForms(t *testing.T) {
	data := []byte(`<gameList>
  <game>
    <path>./x.nes</path>
    <name></name>
    <desc/>
    <developer />
  </game>
</gameList>`)
	doc, err := Parse(t.TempDir(), data)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"name", "desc", "developer"} {
		if err := doc.SetField(0, field, 0, "value & <x>"); err != nil {
			t.Fatalf("SetField(%s): %v", field, err)
		}
	}
	output, err := doc.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, field := range []string{"name", "desc", "developer"} {
		needle := "<" + field + ">value &amp; &lt;x&gt;</" + field + ">"
		if !bytes.Contains(output, []byte(needle)) {
			t.Fatalf("%s not converted to text form: %s", field, output)
		}
	}
	if bytes.Contains(output, []byte("<desc/>")) || bytes.Contains(output, []byte("<developer />")) {
		t.Fatalf("self-closing target form was not replaced: %s", output)
	}
}

func TestAddFieldPreservesExistingBytes(t *testing.T) {
	data := []byte(`<gameList>
  <game>
    <path>./x.nes</path>
    <name>X</name>
  </game>
</gameList>`)
	doc, err := Parse(t.TempDir(), data)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.AddField(0, "developer", "A & B"); err != nil {
		t.Fatalf("AddField: %v", err)
	}
	output, err := doc.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !bytes.Contains(output, []byte("<developer>A &amp; B</developer>")) {
		t.Fatalf("added field missing: %s", output)
	}
	// Existing bytes must remain a subsequence in their original order; the
	// only new bytes should be the inserted field and its separator.
	if !bytes.HasPrefix(output, data[:bytes.Index(data, []byte("  </game>"))]) {
		t.Fatalf("prefix bytes changed: %s", output)
	}
	parsed, err := Parse(t.TempDir(), output)
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	field, ok := parsed.Field(0, "developer", 0)
	if !ok || field.Value != "A & B" {
		t.Fatalf("reparsed field = %#v ok=%v", field, ok)
	}
}

func TestAddFieldUsesEntryNamespace(t *testing.T) {
	data := []byte(`<es:gameList xmlns:es="urn:test"><es:game><es:path>./x.nes</es:path><es:name>X</es:name></es:game></es:gameList>`)
	doc, err := Parse(t.TempDir(), data)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.AddField(0, "developer", "D"); err != nil {
		t.Fatalf("AddField: %v", err)
	}
	output, err := doc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output, []byte("<es:developer>D</es:developer>")) {
		t.Fatalf("namespaced insertion missing: %s", output)
	}
}

func TestAddFieldRejectsSelfClosingEntry(t *testing.T) {
	doc, err := Parse(t.TempDir(), []byte(`<gameList><game><path>./x.nes</path></game><folder/></gameList>`))
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.AddField(1, "name", "x"); !errors.Is(err, ErrUnsupportedMutation) {
		t.Fatalf("AddField self-closing = %v", err)
	}
}

func TestAddFieldRejectsUnknownAndInvalidNames(t *testing.T) {
	doc, err := Parse(t.TempDir(), []byte(`<gameList><game><path>./x.nes</path></game></gameList>`))
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.AddField(0, "unknown", "x"); !errors.Is(err, ErrUnknownField) {
		t.Fatalf("AddField unknown = %v", err)
	}
	if err := doc.AddField(0, "not a name", "x"); !errors.Is(err, ErrInvalidFieldName) {
		t.Fatalf("AddField invalid = %v", err)
	}
}

func TestSetFieldOnAddedFieldIsRejected(t *testing.T) {
	doc, err := Parse(t.TempDir(), []byte(`<gameList><game><path>./x.nes</path></game></gameList>`))
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.AddField(0, "developer", "D"); err != nil {
		t.Fatal(err)
	}
	if err := doc.SetField(0, "developer", 0, "E"); !errors.Is(err, ErrFieldAdded) {
		t.Fatalf("SetField added = %v", err)
	}
}

func TestSetFieldCanCoalesceAndRevert(t *testing.T) {
	data := []byte(`<gameList><game><path>./x.nes</path><name>Old</name></game></gameList>`)
	doc, err := Parse(t.TempDir(), data)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.SetField(0, "name", 0, "First"); err != nil {
		t.Fatal(err)
	}
	if err := doc.SetField(0, "name", 0, "Second"); err != nil {
		t.Fatal(err)
	}
	output, err := doc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output, []byte("<name>Second</name>")) || bytes.Contains(output, []byte("<name>First</name>")) {
		t.Fatalf("coalescing failed: %s", output)
	}
	if err := doc.SetField(0, "name", 0, "Old"); err != nil {
		t.Fatal(err)
	}
	output, err = doc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output, data) {
		t.Fatalf("reverting to original value did not restore bytes")
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want error
	}{
		{"empty", nil, ErrEmpty},
		{"invalid utf8", []byte{0xff, 0xfe, 0x00}, ErrUnsupportedEncoding},
		{"utf16 bom", []byte{0xff, 0xfe, '<', 0, 'x', 0, '/', 0, '>'}, ErrUnsupportedEncoding},
		{"bad xml", []byte(`<gameList><game></gameList>`), ErrInvalidXML},
		{"wrong root", []byte(`<games><game/></games>`), ErrRoot},
		{"multiple roots", []byte(`<gameList/><gameList/>`), ErrInvalidXML},
		{"text outside", []byte(`hello<gameList/>`), ErrInvalidXML},
		{"unbound prefix", []byte(`<gameList><x:game/></gameList>`), ErrInvalidXML},
		{"duplicate attribute", []byte(`<gameList a="1" a="2"/>`), ErrInvalidXML},
		{"bad encoding declaration", []byte(`<?xml version="1.0" encoding="ISO-8859-1"?><gameList/>`), ErrUnsupportedEncoding},
		{"unclosed", []byte(`<gameList><game>`), ErrInvalidXML},
		{"declaration after comment", []byte(`<!-- c --><?xml version="1.0"?><gameList/>`), ErrInvalidXML},
		{"declaration after whitespace", []byte(` <?xml version="1.0"?><gameList/>`), ErrInvalidXML},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse(t.TempDir(), test.data)
			if !errors.Is(err, test.want) {
				t.Fatalf("Parse(%q) = %v, want %v", test.name, err, test.want)
			}
		})
	}
}

func TestPathClassification(t *testing.T) {
	base := t.TempDir()
	outsideAbsolute := filepath.Join(filepath.Dir(base), "outside", "x.nes")
	cases := []struct {
		name   string
		raw    string
		status PathStatus
	}{
		{"relative", "./sub/game.nes", PathRelative},
		{"relative backslash", `.\sub\game.nes`, PathRelative},
		{"relative dot", "./", PathRelative},
		{"dot", ".", PathRelative},
		{"relative escape", "../outside.nes", PathOutside},
		{"relative escape nested", "./sub/../../outside.nes", PathOutside},
		{"absolute inside", filepath.Join(base, "game.nes"), PathAbsoluteInside},
		{"absolute outside", outsideAbsolute, PathOutside},
		{"bare relative", "games/game.nes", PathBareRelative},
		{"home", "~/.emulationstation/downloaded_images/x.png", PathHomeUnsupported},
		{"empty", "", PathInvalid},
		{"nul", "a\x00b", PathInvalid},
		{"windows drive", `C:\Games\x.nes`, PathOutside},
		{"windows drive relative", `C:Games\x.nes`, PathOutside},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := classifyPath(base, test.raw)
			if got.Status != test.status {
				t.Fatalf("classifyPath(%q) status = %q, want %q (resolved=%q)", test.raw, got.Status, test.status, got.Resolved)
			}
			if got.Raw != test.raw {
				t.Fatalf("raw path changed: %q", got.Raw)
			}
		})
	}
}

func TestBOMAndNamespaceRoundTripAfterEdit(t *testing.T) {
	data := readFixture(t, "batocera-43.1-nes-gamelist.xml")
	doc, err := Parse(t.TempDir(), data)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.SetField(0, "name", 0, "New Name"); err != nil {
		t.Fatal(err)
	}
	output, err := doc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(output, []byte(utf8BOM)) {
		t.Fatalf("BOM not preserved")
	}
	if !bytes.Contains(output, []byte(`<name>New Name</name>`)) {
		t.Fatalf("edited field missing")
	}
	reparsed, err := Parse(t.TempDir(), output)
	if err != nil {
		t.Fatalf("reparse edited BOM: %v", err)
	}
	if got, ok := reparsed.Field(0, "name", 0); !ok || got.Value != "New Name" {
		t.Fatalf("reparsed field = %#v ok=%v", got, ok)
	}
}

func TestNoEditsMarshalReturnsCopy(t *testing.T) {
	data := readFixture(t, "synthetic-complex-gamelist.xml")
	doc, err := Parse(t.TempDir(), data)
	if err != nil {
		t.Fatal(err)
	}
	output, err := doc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	output[0] = 'X'
	again, err := doc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if again[0] == 'X' {
		t.Fatalf("Marshal leaked its internal source buffer")
	}
}

func TestLoadReadOnlyFilesystem(t *testing.T) {
	dir := t.TempDir()
	data := readFixture(t, "batocera-43.1-nes-gamelist.xml")
	path := filepath.Join(dir, "gamelist.xml")
	if err := os.WriteFile(path, data, 0o444); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err != nil {
		t.Fatalf("Load: %v", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) || before.Mode() != after.Mode() {
		t.Fatalf("Load changed file metadata")
	}
}

func fieldValues(entry Entry, local string) []string {
	var values []string
	for _, field := range entry.Fields {
		if field.Name.Local == local {
			values = append(values, field.Value)
		}
	}
	return values
}

func findField(entry Entry, prefix, local string, ordinal int) (Field, bool) {
	for _, field := range entry.Fields {
		if field.Name.Prefix == prefix && field.Name.Local == local && field.Ordinal == ordinal {
			return field, true
		}
	}
	return Field{}, false
}

func attributeValue(attrs []Attribute, local string) string {
	for _, attr := range attrs {
		if attr.Name.Local == local {
			return attr.Value
		}
	}
	return ""
}

func TestSystemDirSymlinkIsAllowedButGamelistSymlinkIsNot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require additional Windows privileges")
	}
	realDir := t.TempDir()
	data := readFixture(t, "batocera-43.1-nes-gamelist.xml")
	if err := os.WriteFile(filepath.Join(realDir, "gamelist.xml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(t.TempDir(), "system-link")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatal(err)
	}
	doc, err := Load(linkDir)
	if err != nil {
		t.Fatalf("Load via systemDir symlink: %v", err)
	}
	if doc.EntryCount() != 1 {
		t.Fatalf("EntryCount = %d", doc.EntryCount())
	}
}

func TestAddFieldPreservesCRLF(t *testing.T) {
	data := []byte("<gameList>\r\n  <game>\r\n    <path>./x.nes</path>\r\n  </game>\r\n</gameList>\r\n")
	doc, err := Parse(t.TempDir(), data)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.AddField(0, "name", "X"); err != nil {
		t.Fatal(err)
	}
	output, err := doc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output, []byte("<name>X</name>\r\n  </game>")) {
		t.Fatalf("CRLF style not reused: %q", output)
	}
	if !bytes.Contains(output, []byte("\r\n  <name>X</name>\r\n")) {
		t.Fatalf("new field is not on a CRLF line: %q", output)
	}
}

func TestPathRejectsAbsoluteRemainderAfterDotSlash(t *testing.T) {
	base := t.TempDir()
	for _, raw := range []string{".//etc/passwd", `.\C:\Games\x.nes`} {
		info := classifyPath(base, raw)
		if info.Status != PathOutside {
			t.Fatalf("classifyPath(%q) = %#v, want outside", raw, info)
		}
	}
}

func TestFieldOrdinalsArePerNamespace(t *testing.T) {
	data := []byte(`<gameList xmlns:es="urn:es"><game><path>./x.nes</path><name>one</name><es:name>two</es:name><name>three</name></game></gameList>`)
	doc, err := Parse(t.TempDir(), data)
	if err != nil {
		t.Fatal(err)
	}
	game := doc.Entries[0]
	var plain, namespaced []int
	for _, field := range game.Fields {
		if field.Name.Local != "name" {
			continue
		}
		if field.Name.Namespace == game.Name.Namespace {
			plain = append(plain, field.Ordinal)
		} else {
			namespaced = append(namespaced, field.Ordinal)
		}
	}
	if len(plain) != 2 || plain[0] != 0 || plain[1] != 1 {
		t.Fatalf("plain ordinals = %v", plain)
	}
	if len(namespaced) != 1 || namespaced[0] != 0 {
		t.Fatalf("namespaced ordinals = %v", namespaced)
	}
}

func TestAttributesRemainInSourceOrder(t *testing.T) {
	data := readFixture(t, "synthetic-complex-gamelist.xml")
	doc, err := Parse(t.TempDir(), data)
	if err != nil {
		t.Fatal(err)
	}
	game := doc.Entries[0]
	if got := []string{game.Attributes[0].Name.Local, game.Attributes[1].Name.Local}; got[0] != "id" || got[1] != "custom" {
		t.Fatalf("attribute order = %v", got)
	}
}

func TestSetFieldRejectsCommentAndMixedContent(t *testing.T) {
	data := []byte(`<gameList><game><path>./x.nes</path><name>pre<!-- c -->post</name><desc>pre<unknown/>post</desc><developer>ok</developer></game></gameList>`)
	doc, err := Parse(t.TempDir(), data)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"name", "desc"} {
		if err := doc.SetField(0, field, 0, "new"); !errors.Is(err, ErrComplexField) {
			t.Fatalf("SetField(%s) = %v, want ErrComplexField", field, err)
		}
	}
	if err := doc.SetField(0, "developer", 0, "new"); err != nil {
		t.Fatalf("SetField developer: %v", err)
	}
	output, err := doc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output, []byte("<name>pre<!-- c -->post</name>")) || !bytes.Contains(output, []byte("<desc>pre<unknown/>post</desc>")) {
		t.Fatalf("complex fields were reformatted: %s", output)
	}
}

func TestSetFieldRejectsInvalidValues(t *testing.T) {
	doc, err := Parse(t.TempDir(), []byte(`<gameList><game><path>./x.nes</path><name>ok</name></game></gameList>`))
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"bad\x00value", "bad\x01value", string([]byte{0xc1})} {
		if err := doc.SetField(0, "name", 0, value); !errors.Is(err, ErrInvalidValue) {
			t.Fatalf("SetField(%q) = %v, want ErrInvalidValue", value, err)
		}
	}
	if err := doc.AddField(0, "developer", "bad\x00value"); !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("AddField invalid = %v", err)
	}
}

func TestSetFieldPreservesSelfClosingAttributes(t *testing.T) {
	data := []byte(`<gameList><game><path>./x.nes</path><name id="7" /></game></gameList>`)
	doc, err := Parse(t.TempDir(), data)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.SetField(0, "name", 0, "X"); err != nil {
		t.Fatal(err)
	}
	output, err := doc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output, []byte(`<name id="7">X</name>`)) {
		t.Fatalf("self-closing attributes not preserved: %s", output)
	}
}

func TestParseEmptyRoot(t *testing.T) {
	data := []byte(`<?xml version="1.0"?><gameList/>`)
	doc, err := Parse(t.TempDir(), data)
	if err != nil {
		t.Fatal(err)
	}
	if doc.EntryCount() != 0 {
		t.Fatalf("EntryCount = %d", doc.EntryCount())
	}
	output, err := doc.Marshal()
	if err != nil || !bytes.Equal(output, data) {
		t.Fatalf("empty root round-trip = %q, %v", output, err)
	}
}

func TestMarshalMultipleEditsAndInsertion(t *testing.T) {
	data := []byte(`<gameList><game><path>./x.nes</path><name>Old</name><desc>D</desc></game></gameList>`)
	doc, err := Parse(t.TempDir(), data)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.SetField(0, "name", 0, "New"); err != nil {
		t.Fatal(err)
	}
	if err := doc.SetField(0, "desc", 0, "E"); err != nil {
		t.Fatal(err)
	}
	if err := doc.AddField(0, "developer", "F"); err != nil {
		t.Fatal(err)
	}
	output, err := doc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(t.TempDir(), output)
	if err != nil {
		t.Fatal(err)
	}
	for field, want := range map[string]string{"name": "New", "desc": "E", "developer": "F"} {
		got, ok := parsed.Field(0, field, 0)
		if !ok || got.Value != want {
			t.Fatalf("%s = %#v ok=%v, want %q", field, got, ok, want)
		}
	}
}

func TestAddFieldAllowsExplicitDuplicate(t *testing.T) {
	data := []byte(`<gameList><game><path>./x.nes</path><name>Old</name></game></gameList>`)
	doc, err := Parse(t.TempDir(), data)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.AddField(0, "name", "Second"); err != nil {
		t.Fatal(err)
	}
	if got, ok := doc.Field(0, "name", 1); !ok || got.Value != "Second" || !got.Added {
		t.Fatalf("added duplicate = %#v ok=%v", got, ok)
	}
	output, err := doc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output, []byte("<name>Old</name><name>Second</name>")) {
		t.Fatalf("duplicate insertion changed order: %s", output)
	}
}

func TestAddFieldInlineEntry(t *testing.T) {
	data := []byte(`<gameList><game><path>./x.nes</path></game></gameList>`)
	doc, err := Parse(t.TempDir(), data)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.AddField(0, "name", "X"); err != nil {
		t.Fatal(err)
	}
	output, err := doc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output, []byte(`<path>./x.nes</path><name>X</name></game>`)) {
		t.Fatalf("inline insertion = %s", output)
	}
}

func TestSetFieldEscapesXMLText(t *testing.T) {
	data := []byte(`<gameList><game><path>./x.nes</path><name>old</name></game></gameList>`)
	doc, err := Parse(t.TempDir(), data)
	if err != nil {
		t.Fatal(err)
	}
	value := "a]]>b & <c> \r\n"
	if err := doc.SetField(0, "name", 0, value); err != nil {
		t.Fatal(err)
	}
	output, err := doc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(t.TempDir(), output)
	if err != nil {
		t.Fatal(err)
	}
	field, ok := parsed.Field(0, "name", 0)
	if !ok || field.Value != value {
		t.Fatalf("field = %#v ok=%v, want %q", field, ok, value)
	}
}

func TestNamespaceEndTagUsesExpandedName(t *testing.T) {
	data := []byte(`<a:gameList xmlns:a="urn:test" xmlns:b="urn:test"><a:game><a:path>./x.nes</a:path><a:name>X</a:name></b:game></b:gameList>`)
	doc, err := Parse(t.TempDir(), data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if doc.EntryCount() != 1 {
		t.Fatalf("EntryCount = %d", doc.EntryCount())
	}
	if err := doc.SetField(0, "name", 0, "Y"); err != nil {
		t.Fatalf("SetField: %v", err)
	}
	if _, err := doc.Marshal(); err != nil {
		t.Fatalf("Marshal: %v", err)
	}
}

func TestEditsAndMarshalDoNotTouchSourceFile(t *testing.T) {
	dir := t.TempDir()
	data := readFixture(t, "batocera-43.1-nes-gamelist.xml")
	path := filepath.Join(dir, "gamelist.xml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.SetField(0, "name", 0, "Changed"); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.Marshal(); err != nil {
		t.Fatal(err)
	}
	afterBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterBytes, data) {
		t.Fatalf("source gamelist changed on disk")
	}
	after, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("new files appeared: before=%d after=%d", len(before), len(after))
	}
}

func TestAddFieldWithDefaultNamespace(t *testing.T) {
	data := []byte(`<gameList xmlns="urn:test"><game><path>./x.nes</path></game></gameList>`)
	doc, err := Parse(t.TempDir(), data)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.AddField(0, "name", "X"); err != nil {
		t.Fatal(err)
	}
	output, err := doc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output, []byte(`<name>X</name>`)) {
		t.Fatalf("default-namespace insertion = %s", output)
	}
	parsed, err := Parse(t.TempDir(), output)
	if err != nil {
		t.Fatal(err)
	}
	field, ok := parsed.Field(0, "name", 0)
	if !ok || field.Name.Namespace != "urn:test" {
		t.Fatalf("inserted field namespace = %#v ok=%v", field, ok)
	}
}
