package gamelist

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func FuzzParseMarshalNoEdit(f *testing.F) {
	seeds := [][]byte{
		[]byte(`<gameList/>`),
		[]byte(`<gameList><game><path>./x.nes</path><name>X</name></game></gameList>`),
		readFixtureF(f, "batocera-43.1-nes-gamelist.xml"),
		readFixtureF(f, "synthetic-complex-gamelist.xml"),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	base := f.TempDir()
	f.Fuzz(func(t *testing.T, data []byte) {
		doc, err := Parse(base, data)
		if err != nil {
			return
		}
		output, err := doc.Marshal()
		if err != nil {
			t.Fatalf("Parse succeeded but Marshal failed: %v", err)
		}
		if !bytes.Equal(output, data) {
			t.Fatalf("no-edit Marshal changed bytes: in=%d out=%d", len(data), len(output))
		}
	})
}

func FuzzSetFieldBoundary(f *testing.F) {
	seeds := []string{
		`<gameList><game><path>./x.nes</path><name>X</name></game></gameList>`,
		`<gameList><game><path>./x.nes</path><name></name><desc/></game></gameList>`,
		string(readFixtureF(f, "synthetic-complex-gamelist.xml")),
	}
	for _, seed := range seeds {
		f.Add(seed, "new & <value>", 0)
		f.Add(seed, "", 1)
	}
	base := f.TempDir()
	f.Fuzz(func(t *testing.T, source, value string, occurrence int) {
		doc, err := Parse(base, []byte(source))
		if err != nil {
			return
		}
		if len(doc.Entries) == 0 {
			return
		}
		if err := doc.SetField(0, "name", occurrence, value); err != nil {
			return
		}
		output, err := doc.Marshal()
		if err != nil {
			t.Fatalf("SetField succeeded but Marshal failed: %v", err)
		}
		reparsed, err := Parse(base, output)
		if err != nil {
			t.Fatalf("Marshal produced invalid XML: %v", err)
		}
		field, ok := reparsed.Field(0, "name", occurrence)
		if !ok || field.Value != value {
			t.Fatalf("reparsed field = %#v ok=%v, want %q", field, ok, value)
		}
	})
}

func readFixtureF(f *testing.F, name string) []byte {
	f.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		f.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}
