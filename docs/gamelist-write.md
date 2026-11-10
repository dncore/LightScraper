# gamelist.xml safe write — LS-T003-B

## Status and scope

LS-T003-B adds `Document.Save` to `internal/library/gamelist`. It preserves the
T003-A parse/source-span/round-trip behavior and writes only the private
`<systemDir>/gamelist.xml` target that `Load` opened.

The package never writes a ROM, image, video, or other media path. It never
creates a missing `gamelist.xml` or a directory referenced by `<path>`. A Save
that cannot prove the loaded target is still the same file refuses before
replacement.

Design decisions and the reviewed state table are in the R4 report:
<https://github.com/dncore/LightScraper/issues/4#issuecomment-6760157537>.
This document describes the implementation contract; R1-R3 remain historical
context where R4 does not supersede them.

## API

```go
func Load(systemDir string) (*Document, error)
func Parse(systemDir string, data []byte) (*Document, error)

func (d *Document) Save(ctx context.Context, opts SaveOptions) (SaveResult, error)
func (d *Document) Stale() bool

func (d *Document) SetField(entry int, name string, occurrence int, value string) error
func (d *Document) AddField(entry int, name, value string) error
func (d *Document) Marshal() ([]byte, error)
```

`SaveOptions`:

```go
type BackupPolicy int
const (
    BackupDefault BackupPolicy = iota
    BackupNone
)

type DurabilityPolicy int
const (
    DurabilityBestEffort DurabilityPolicy = iota // default
    DurabilityStrict
)

type SaveOptions struct {
    Backup     BackupPolicy
    Durability DurabilityPolicy
}
```

The zero value is `BackupDefault` + `DurabilityBestEffort`.

`SaveResult`:

```go
type CommitStatus uint8
const (
    CommitNotCommitted CommitStatus = iota
    CommitCommitted
    CommitUnknown
)

type SaveResult struct {
    Status          CommitStatus
    Changed         bool
    SnapshotCurrent bool
    Path            string
    BackupPath      string
    Warnings        []error
}
```

## Private baseline

`Load` records, privately, the absolute target path, the opened file's
`os.FileInfo`, SHA-256, size, mode, mtime, and the system directory's identity
(`os.FileInfo` from `os.Stat`). Public `Path`, `SystemDir`, and `SourceHash`
remain informational and are never used as Save credentials.

`Save` uses only the private target and the private loaded directory. Public
field mutation cannot redirect the write or alter the strict Parse validation
directory. `Marshal()` continues to use `d.SystemDir` for its pure in-memory
validation; `Save` uses an internal `marshal(d.loadedDir)`.

`Load` follows the caller-selected system-directory symlink, matching T003-A.
A `gamelist.xml` symlink is rejected. The system directory's target identity is
checked before Write and again before replacement.

## Save sequence

1. Reject a stale Document with `ErrDocumentStale`.
2. Verify the private target path, system-directory identity, and a stable read
   of the current target (identity + hash + metadata) against the loaded
   baseline.
3. Marshal with `d.loadedDir`; failure returns `ErrSaveMarshal` without writing.
4. If the output equals the current source, return a no-op (no temp or backup).
5. Create a same-directory temp, write all bytes, preserve permission bits,
   `Sync`, close, read back byte-for-byte, and strictly Parse.
6. Create an exclusive backup (unless explicitly disabled), write the original
   source bytes, preserve permission bits, and `Sync`.
7. Sync the parent directory according to `DurabilityPolicy`.
8. Recheck system directory and target identity/hash/metadata.
9. Replace via `os.Rename`.
10. Verify the resulting target as Old/New/Other/Missing/Symlink/NonRegular/
    Unreadable, then sync the parent directory again.
11. For a confirmed New target, re-open the actual target, verify bytes/hash,
    strictly Parse, rebuild the source-span index, update the private baseline
    and public `SourceHash`, and clear edits.
12. For a confirmed Old target with a failed Rename, remain active and allow a
    retry. For an unknown/contradictory state, mark the Document stale.

A successful write is never inferred from temp metadata. The new baseline comes
only from the actual target path after replacement.

## Commit status and stale lifecycle

`SaveResult.SnapshotCurrent` means that the returned Document still represents
the verified target baseline and is safe for another Save attempt after any
needed edits. On a loaded Document, `SnapshotCurrent=false` is always
`Stale()==true`. A `Parse`-only Document has no disk target: Save returns
`ErrSaveNoSource`, `SnapshotCurrent=false`, but it is not stale for in-memory
Marshal.

`Committed + SnapshotCurrent=false` and `CommitUnknown` always mark the
Document stale and prohibit retry on the same Document. `Committed` with a
current snapshot does not permit replaying the same Save, but new edits and a
new Save against the refreshed baseline are allowed.

`SetField`, `AddField`, `Marshal`, and `Save` return `ErrDocumentStale` after a
stale transition. Read-only views may still show the old snapshot. There is no
implicit reload or rebase; callers must call `Load` again.

## Backup behavior

- Default: one uniquely named `gamelist.xml.bak.<UTC timestamp>-<random>` in the
  same directory, opened with `O_EXCL`; an existing name is never overwritten.
- Backup contains the exact original `d.source` bytes and uses the original
  permission bits.
- Backups are retained on success and failure. No history is automatically
  cleaned.
- `BackupNone` disables backup creation, but the same preflight, temporary-file,
  post-state, and snapshot rules still apply.

## Directory sync policy

`DurabilityBestEffort` is the default and the zero value. The directory sync is
always attempted when the platform exposes it.

- Pre-replace:
  - success: continue;
  - explicit platform/filesystem `ErrDirSyncUnsupported`: BestEffort records
    a warning and continues; Strict stops with `ErrDirSyncBeforeReplace`;
  - real I/O error: both policies stop before replacement.
- Post-replace: failure or unsupported capability records
  `ErrDirSyncAfterReplace` in `Warnings`; a committed file is never rolled back.

Platform expectations:

| Platform | Expected behavior |
|---|---|
| Linux local FS | `fsync(dir)` usually works; unsupported FS capability is classified separately from `EIO` |
| macOS | may return `EINVAL`/`ENOTSUP` for directory sync; BestEffort warns, Strict refuses pre-replace |
| Windows | stdlib has no portable directory sync; BestEffort warns, Strict refuses pre-replace |
| SMB/NFS | capability/durability is not guaranteed; only best-effort behavior without a persistence promise |

The implementation does not try to become root or use a volume-handle API.
Network filesystem success is not a durability guarantee.

## Error classification

Important sentinels:

- `ErrSaveNoSource`, `ErrSaveTarget`, `ErrInvalidSaveOptions`, `ErrDocumentStale`
- `ErrSourceMissing`, `ErrSourceChanged`, `ErrSourceReplaced`,
  `ErrSourceMetadataChanged`, `ErrSourcePermission`
- `ErrSystemDirMissing`, `ErrSystemDirChanged`, `ErrSystemDirNotDir`
- `ErrSaveMarshal`, `ErrTempCreate`, `ErrTempWrite`, `ErrTempSync`,
  `ErrTempValidate`, `ErrTempMetadata`
- `ErrBackupExists`, `ErrBackup`
- `ErrDirSyncBeforeReplace`, `ErrDirSyncAfterReplace`, `ErrDirSyncUnsupported`
- `ErrReplace`, `ErrReplaceReportedFailure`
- `ErrSnapshotRefreshFailed`, `ErrPostCommitVerify`, `ErrCommitUnknown`

Use `errors.Is` to classify. Errors are wrapped with the relevant sentinel and
operation/path context.

## No media access

No Save path uses `SetField`'s `<path>` text, `PathInfo.Resolved`, or any ROM or
media API. Only these paths may be created or modified:

- `<loadedDir>/gamelist.xml`
- a same-directory `gamelist.xml.tmp-*`
- a same-directory `gamelist.xml.bak.*`

Missing targets are errors, not creation opportunities.

## Tests

The suite includes traceable T003-A round-trip tests plus Save tests for:

- happy path, no-op, backup contents, and consecutive Saves;
- public `Path`/`SystemDir`/`SourceHash` mutation;
- source replacement, metadata change, missing target, symlink target, and
  retargeted system-directory symlinks;
- stale rejection for all editing/serialization/Save methods;
- temp create/write/sync/readback failures;
- backup conflict/open/write/sync failures;
- Rename error with Old target, Rename error with New target, Rename success
  with Other/Missing/Symlink/NonRegular targets;
- Committed snapshot-refresh failure and post-replace directory-sync warnings;
- BestEffort unsupported vs real I/O errors and Strict pre-replace rejection;
- context cancellation before writes and no implicit rebase.

Run:

```bash
go test ./...
go test -race ./...
go vet ./...
```

## Known limitations

- TOCTOU remains: identity/hash checks cannot make path-based rename a CAS.
- No cross-process lock is added.
- Windows ACL/owner/creation-time preservation is not claimed; only permission
  bits are applied through `Chmod`.
- `SameFile` identity can theoretically be affected by file-ID/inode reuse.
- Network filesystems may report success without durable or atomic semantics.
- Directory sync capability detection is platform/FS-specific.
- Only UTF-8 and UTF-8 BOM are supported, inherited from T003-A.
- `SetField` still rejects CDATA/mixed content; `AddField` still requires known
  fields and a non-self-closing entry.
