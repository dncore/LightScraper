package scanner

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseOfficialFixtures(t *testing.T) {
	tests := []struct {
		name   string
		file   string
		expect []string
	}{
		{
			name:   "43.1 quoted with concatenated comment",
			file:   "batocera-43.1-nes_info.txt",
			expect: []string{".7z", ".nes", ".unf", ".unif", ".zip"},
		},
		{
			name:   "master inline unquoted",
			file:   "batocera-master-inline-nes_info.txt",
			expect: []string{".7z", ".nes", ".unf", ".unif", ".zip"},
		},
		{
			name:   "master emulator core block",
			file:   "batocera-master-multiline-nes_info.txt",
			expect: []string{".7z", ".nes", ".unf", ".unif", ".zip"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := os.Open(filepath.Join("testdata", test.file))
			if err != nil {
				t.Fatalf("open fixture: %v", err)
			}
			defer func() {
				if err := file.Close(); err != nil {
					t.Errorf("close fixture: %v", err)
				}
			}()

			extensions, err := parseInfo(file)
			if err != nil {
				t.Fatalf("parse fixture: %v", err)
			}
			if !reflect.DeepEqual(extensions, test.expect) {
				t.Fatalf("expected %v, got %v", test.expect, extensions)
			}
		})
	}
}

func Test43FixturePreservesConcatenatedComment(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "batocera-43.1-nes_info.txt"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `".nes .unif .unf .zip .7z"For more info:`) {
		t.Fatal("43.1 fixture no longer preserves the generator's concatenated comment form")
	}
}

func TestParseInfoQuotedWithCommentOnNextLine(t *testing.T) {
	input := `## SYSTEM TEST ##
-------------------------------------------------------------------------------
ROM files extensions accepted: ".NES .zip"

For more info: https://example.invalid
-------------------------------------------------------------------------------
`
	extensions, err := parseInfo(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse info: %v", err)
	}
	expected := []string{".nes", ".zip"}
	if !reflect.DeepEqual(extensions, expected) {
		t.Fatalf("expected %v, got %v", expected, extensions)
	}
}

func TestParseInfoFrenchFallback(t *testing.T) {
	input := `## SYSTEM TEST ##
-------------------------------------------------------------------------------
Extensions des fichiers ROMs permises: ".sfc .SMC"
`
	extensions, err := parseInfo(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse info: %v", err)
	}
	expected := []string{".sfc", ".smc"}
	if !reflect.DeepEqual(extensions, expected) {
		t.Fatalf("expected %v, got %v", expected, extensions)
	}
}

func TestParseInfoIgnoresInvalidExtensions(t *testing.T) {
	input := `ROM files extensions accepted: ".nes bad . .zip .NES .tar/gz .txt .bad; .tar.gz"
`
	extensions, err := parseInfo(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse info: %v", err)
	}
	expected := []string{".nes", ".txt", ".zip"}
	if !reflect.DeepEqual(extensions, expected) {
		t.Fatalf("expected %v, got %v", expected, extensions)
	}
}

func TestParseInfoConcatenatedCommentCannotAddExtensions(t *testing.T) {
	input := `ROM files extensions accepted: ".nes" comment ".fake .zip"
`
	extensions, err := parseInfo(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse info: %v", err)
	}
	expected := []string{".nes"}
	if !reflect.DeepEqual(extensions, expected) {
		t.Fatalf("expected %v, got %v", expected, extensions)
	}
}

func TestParseInfoMergesCoreExtensions(t *testing.T) {
	input := `ROM files extensions accepted:
- libretro/first: .nes
- libretro/second: .zip .7z
`
	extensions, err := parseInfo(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse info: %v", err)
	}
	expected := []string{".7z", ".nes", ".zip"}
	if !reflect.DeepEqual(extensions, expected) {
		t.Fatalf("expected %v, got %v", expected, extensions)
	}
}

func TestParseInfoStopsAtNonBulletAfterCoreBlock(t *testing.T) {
	input := `ROM files extensions accepted:
- libretro/core: .nes .zip
- For more info: .fake
- libretro/other: .sfc
`
	extensions, err := parseInfo(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse info: %v", err)
	}
	expected := []string{".nes", ".zip"}
	if !reflect.DeepEqual(extensions, expected) {
		t.Fatalf("expected %v, got %v", expected, extensions)
	}
}

func TestParseInfoRejectsNoUsableSection(t *testing.T) {
	tests := []string{
		"",
		"## SYSTEM TEST ##\n- no extensions here\n",
		`ROM files extensions accepted: "unterminated`,
		`ROM files extensions accepted: ""`,
	}
	for _, input := range tests {
		if _, err := parseInfo(strings.NewReader(input)); !errors.Is(err, errNoExtensions) {
			t.Fatalf("expected errNoExtensions for %q, got %v", input, err)
		}
	}
}

func TestParseInfoRequiresReader(t *testing.T) {
	if _, err := parseInfo(nil); err == nil {
		t.Fatal("expected nil reader to be rejected")
	}
}

func TestParseInfoReportsReaderFailure(t *testing.T) {
	if _, err := parseInfo(failingReader{}); err == nil {
		t.Fatal("expected reader error")
	}
}

func TestParseInfoAcceptsBOMAndCaseInsensitiveLabel(t *testing.T) {
	input := "\uFEFFROM files extensions accepted: .nes\n"
	extensions, err := parseInfo(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse info: %v", err)
	}
	if !reflect.DeepEqual(extensions, []string{".nes"}) {
		t.Fatalf("unexpected extensions: %v", extensions)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("read failure")
}
