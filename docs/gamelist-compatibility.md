# gamelist.xml compatibility — LS-T003-A/T003-B

## Status

This document describes the accepted **T003-A in-memory core** and the
**T003-B safe-write layer** in `internal/library/gamelist`. The detailed write
contract, platform policy and failure states are in
[`gamelist-write.md`](gamelist-write.md).

T003-A parse/source-span/round-trip behavior is unchanged. T003-B adds
`Document.Save`, a same-directory temporary file, an exclusive backup, directory
sync policy, replacement verification and actual-target snapshot refresh. It
does not open a ROM, image, video, or media path. `Load` reads one file;
`Marshal` still only returns bytes in memory. No cross-process lock is added.

## Scope

The package provides:

- strict UTF-8/XML parsing and a source-span index over the exact original
  bytes;
- read-only game/folder views with all direct fields, including unknown
  fields and duplicates;
- root attributes, unknown top-level elements, comments and all untouched
  source text preserved by construction;
- in-memory `SetField` and `AddField` operations;
- byte-splice `Marshal` that returns a new byte slice and never writes a file;
- lexical ROM path classification; no filesystem access through `<path>`;
- T003-B `Save` to the private loaded target with same-directory temp/backup,
  directory-sync policy, commit verification and stale lifecycle.

## Upstream evidence

Primary compatibility target: the EmulationStation revision pinned by Batocera
43.1.

- Batocera Linux tag `batocera-43.1`, commit
  `155c2d8d304cbb53db52e9479dcf683392821d5c`.
- ES pin from
  [`batocera-emulationstation.mk`](https://github.com/batocera-linux/batocera.linux/blob/batocera-43.1/package/batocera/emulationstation/batocera-emulationstation/batocera-emulationstation.mk):
  commit `2c29a330e487210a7d51ad2650bb7b280ea44c86`.
- ES repository: <https://github.com/batocera-linux/batocera-emulationstation>
- Relevant source:
  - [`GAMELISTS.md`](https://github.com/batocera-linux/batocera-emulationstation/blob/master/GAMELISTS.md)
  - [`es-app/src/MetaData.cpp`](https://github.com/batocera-linux/batocera-emulationstation/blob/2c29a330e487210a7d51ad2650bb7b280ea44c86/es-app/src/MetaData.cpp)
  - [`es-app/src/Gamelist.cpp`](https://github.com/batocera-linux/batocera-emulationstation/blob/2c29a330e487210a7d51ad2650bb7b280ea44c86/es-app/src/Gamelist.cpp)
  - [`es-app/src/SystemData.cpp`](https://github.com/batocera-linux/batocera-emulationstation/blob/2c29a330e487210a7d51ad2650bb7b280ea44c86/es-app/src/SystemData.cpp)
  - [`es-core/src/utils/FileSystemUtil.cpp`](https://github.com/batocera-linux/batocera-emulationstation/blob/2c29a330e487210a7d51ad2650bb7b280ea44c86/es-core/src/utils/FileSystemUtil.cpp)

The Batocera fork adds fields and an `id` attribute. Its own
`MetaDataList::loadFromXML` keeps only unknown direct child text and unknown
direct attributes, and `updateGamelist` removes and re-adds dirty nodes. ES
therefore cannot be used as a lossless-write reference. This package treats the
original bytes as the only preservation source.

Official fixture provenance is recorded in
[`internal/library/gamelist/testdata/README.md`](../internal/library/gamelist/testdata/README.md).

## Source-span model

Parsing uses `encoding/xml.Decoder.RawToken` with:

- UTF-8 validation before parsing;
- explicit UTF-8 BOM handling;
- byte offset tracking via `InputOffset`;
- manual start/end element pairing and depth checks;
- start/end tags must match the original QName (prefix + local name), not
  merely the expanded namespace URI + local name; namespace URI resolution is
  still verified independently;
- exact root and single-root checks;
- attribute duplicate checks;
- namespace scope resolution (URI + local name) while retaining raw prefixes;
- bounds checks before any byte slice is used.

`Document` keeps the original bytes private. `Entry`, `Field`, `Attribute`,
`PathInfo` and `RawTopLevel` are views. No code reconstructs the document from
Go structs.

The BOM is not stripped and is never emitted as text. `InputOffset` remains an
offset into the original bytes, so a BOM shifts the index naturally rather than
requiring a lossy offset conversion. A no-edit `Marshal` returns the original
BOM and declaration byte-for-byte.

## Namespace handling

`QName` stores:

- `Prefix`: the original prefix, retained for rendering;
- `Namespace`: the resolved namespace URI, used for semantic matching;
- `Local`: the local name.

`encoding/xml`'s `Name.Space` is not assumed to be a prefix. Namespace
declarations are resolved with a lexical scope that is copied per element.
Known entries/fields require the same namespace URI and local name as their
parent. Fields in another namespace remain unknown and are preserved verbatim.

## Reading and writing API

```go
func Load(systemDir string) (*Document, error)
func Parse(systemDir string, data []byte) (*Document, error)

func (d *Document) EntryCount() int
func (d *Document) Field(entry int, name string, occurrence int) (Field, bool)
func (d *Document) SetField(entry int, name string, occurrence int, value string) error
func (d *Document) AddField(entry int, name, value string) error
func (d *Document) Marshal() ([]byte, error)
func (d *Document) Save(ctx context.Context, opts SaveOptions) (SaveResult, error)
func (d *Document) Stale() bool
```

`Load` reads exactly `<systemDir>/gamelist.xml`. It does not search
`~/.emulationstation/gamelists` or `/etc/emulationstation/gamelists`. A
`gamelist.xml` symlink is rejected with `ErrSymlink`; it is never resolved with
`EvalSymlinks` or followed. The `systemDir` itself may be reached through a
symlink selected by the caller.

`Load` returns distinguishable sentinel errors (`ErrNotFound`, `ErrEmpty`,
`ErrInvalidXML`, `ErrUnsupportedEncoding`, `ErrRoot`, `ErrSymlink`,
`ErrNotRegular`, `ErrUnreadable`). Other read errors are wrapped.
`SetField`/`AddField` reject values that are not valid UTF-8/XML text with
`ErrInvalidValue`.

## Fields and duplicates

`Entry.Fields` contains every direct child element in source order, including
unknown fields, `<scrap>`, empty elements and duplicates. `Field.Ordinal`
distinguishes duplicates of the same namespace + local name.

`SetField` **requires an explicit occurrence**. There is no API that defaults
to the first duplicate. A missing occurrence is `ErrFieldNotFound`.

Editable names are the known Batocera/ES metadata fields. `path` is the ROM
identity and is not editable in T003-A. `id` is an attribute, not an element.
Unknown fields are visible but not editable. Unknown content is preserved.

## CDATA and mixed content policy

`SetField` refuses to edit a field that contains CDATA, child elements,
comments or processing instructions, returning `ErrComplexField`. It does not
silently replace CDATA with escaped text. The original bytes remain untouched.

`AddField` inserts a new plain-text field. It never rewrites existing nodes.

## Saving and stale lifecycle

T003-B `Save` revalidates the private loaded target (identity, hash, size, mode,
mtime, system directory identity), writes a same-directory temp, creates an
exclusive backup by default, optionally syncs the parent directory, replaces
the target, verifies the resulting state, and refreshes the Document from the
actual target before returning `SnapshotCurrent=true`.

`SaveResult.Status` is `NotCommitted`, `Committed`, or `Unknown`.
`Committed + SnapshotCurrent=false` and `Unknown` mark the Document stale;
`SetField`, `AddField`, `Marshal` and `Save` then reject it with
`ErrDocumentStale`. Only a fresh `Load` restores the lifecycle. Default
`DurabilityBestEffort` treats an explicitly unsupported directory sync as a
warning before replacement, but real I/O errors stop; `DurabilityStrict` stops
on both. See [`gamelist-write.md`](gamelist-write.md) for the full state table and
platform matrix.

## Marshal and validation

`Marshal`:

1. copies the recorded edit list;
2. sorts by source offset;
3. rejects overlapping or out-of-bounds spans;
4. applies replacements/insertions to a copy of the original bytes;
5. verifies every byte outside the edit spans is unchanged;
6. strictly re-parses the result;
7. verifies root, entry count and each edited field value.

`Marshal` itself never writes to disk. `SetField` values are escaped with `xml.EscapeText`.
T003-B `Save` is the only write path; it uses an internal `marshal(loadedDir)`
so public `SystemDir` mutation cannot change Save validation.
Self-closing fields are converted to an open/text/close form only when that
specific field is edited. `AddField` inserts before the entry's closing tag and
reuses the existing indentation/line ending; it never reorders existing nodes.
Adding to a self-closing entry is rejected in T003-A.

## ROM path semantics

Classification is purely lexical:

- `./` and `.\\` are relative to `systemDir`;
- absolute paths inside `systemDir` are accepted for classification;
- absolute paths outside `systemDir` are `PathOutside` and are not touched;
- bare relative paths are `PathBareRelative` because upstream
  `resolveRelativePath` does not rebase them;
- `~/` is `PathHomeUnsupported` for ROM paths;
- empty paths and NUL bytes are invalid.

`PathInfo.Resolved` is diagnostic only. The package never calls `os.Stat`,
`os.Open`, `filepath.EvalSymlinks` or any media API with a `<path>` value.

## Tests

Run:

```bash
go test ./internal/library/gamelist
go test -race ./internal/library/gamelist
go test -run=^$ -fuzz=FuzzParseMarshalNoEdit -fuzztime=10s ./internal/library/gamelist
go test -run=^$ -fuzz=FuzzSetFieldBoundary -fuzztime=10s ./internal/library/gamelist
go vet ./internal/library/gamelist
gofmt -l internal/library/gamelist
```

The suite covers official NES/C64 samples, BOM, multiline text, namespaces,
CDATA, comments, unknown attributes/nodes, duplicate fields, empty and
self-closing elements, original whitespace/order, path boundaries, symlink
rejection, load/read-only rejection behavior, malformed input and local edit
spans. T003-B adds Save tests for identity, temp/backup/Rename failure stages,
directory-sync policy, stale lifecycle and actual-target snapshot refresh.
Fuzz tests assert that parse/marshal without edits is byte-identical and that
successful field edits survive strict re-parsing.

## Known limitations

- TOCTOU between identity checks and `os.Rename` remains; no CAS or lock is
  claimed.
- `SetField` rejects CDATA/mixed-content fields rather than preserving CDATA
  form during mutation.
- Only UTF-8 and UTF-8 BOM are supported; other encodings are rejected.
- Only `<systemDir>/gamelist.xml` is read; ES fallback locations are not
  searched.
- `AddField` is limited to known fields and non-self-closing entries.
- Windows ACL/owner/creation-time preservation is not claimed; only permission
  bits are applied.
- Directory sync support differs by platform/filesystem; network filesystems
  are best-effort only.
- Backups are intentionally retained; no automatic history cleanup exists.
