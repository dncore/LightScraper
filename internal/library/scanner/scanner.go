// Package scanner provides a read-only Batocera ROM library scanner.
package scanner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// fileOps is a small internal seam for deterministic tests of races such as a
// file disappearing between ReadDir and Lstat. It is not a public abstraction.
type fileOps interface {
	Stat(name string) (os.FileInfo, error)
	ReadDir(name string) ([]os.DirEntry, error)
	Lstat(name string) (os.FileInfo, error)
	ReadFile(name string) ([]byte, error)
}

type osFileOps struct{}

func (osFileOps) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

func (osFileOps) ReadDir(name string) ([]os.DirEntry, error) {
	return os.ReadDir(name)
}

func (osFileOps) Lstat(name string) (os.FileInfo, error) {
	return os.Lstat(name)
}

func (osFileOps) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

var (
	errInfoMissing    = errors.New("_info.txt is missing")
	errInfoUnreadable = errors.New("_info.txt is unreadable")
	errInfoInvalid    = errors.New("_info.txt has no usable extension section")
	errInfoSymlink    = errors.New("_info.txt is a symbolic link")
	errInfoNotRegular = errors.New("_info.txt is not a regular file")
)

type scanner struct {
	ops   fileOps
	stats ScanStats
}

// Scan performs a read-only scan of root. Root is the user-selected ROM
// library root (normally Batocera's roms directory); its immediate child
// directories are treated as systems.
//
// Root-level failures are returned as errors. Per-system and per-file failures
// are reported as ScanIssues so other systems can continue. A cancelled scan
// returns the partial result together with the context error.
func Scan(ctx context.Context, root string) (ScanResult, error) {
	return scanWithOps(ctx, root, osFileOps{})
}

func scanWithOps(ctx context.Context, root string, ops fileOps) (result ScanResult, err error) {
	result.StartedAt = time.Now().UTC()
	defer func() {
		result.FinishedAt = time.Now().UTC()
	}()

	if ctx == nil {
		return result, errors.New("context is required")
	}
	if ops == nil {
		return result, errors.New("file operations are required")
	}
	if err := ctx.Err(); err != nil {
		result.Cancelled = true
		return result, err
	}
	if strings.TrimSpace(root) == "" {
		return result, errors.New("ROM root is required")
	}

	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return result, fmt.Errorf("resolve ROM root: %w", err)
	}
	result.Root = absoluteRoot

	rootInfo, err := ops.Stat(absoluteRoot)
	if err != nil {
		return result, fmt.Errorf("stat ROM root: %w", err)
	}
	if !rootInfo.IsDir() {
		return result, fmt.Errorf("ROM root is not a directory: %s", absoluteRoot)
	}

	rootEntries, err := ops.ReadDir(absoluteRoot)
	if err != nil {
		if ctx.Err() != nil {
			result.Cancelled = true
			return result, ctx.Err()
		}
		return result, fmt.Errorf("read ROM root: %w", err)
	}

	state := &scanner{ops: ops}
	for _, entry := range rootEntries {
		if err := ctx.Err(); err != nil {
			result.Cancelled = true
			finishResult(&result, state.stats)
			return result, err
		}

		name := entry.Name()
		if isHidden(name) {
			state.stats.SkippedHidden++
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			state.stats.SkippedSymlinks++
			continue
		}
		if !entry.IsDir() {
			continue
		}
		if strings.EqualFold(name, "bios") {
			state.stats.SkippedBios++
			continue
		}

		systemResult, scanErr := state.scanSystem(ctx, name, filepath.Join(absoluteRoot, name))
		result.Systems = append(result.Systems, systemResult)
		result.Issues = append(result.Issues, systemResult.Issues...)
		if scanErr != nil {
			result.Cancelled = true
			finishResult(&result, state.stats)
			return result, scanErr
		}
	}

	if err := ctx.Err(); err != nil {
		result.Cancelled = true
		finishResult(&result, state.stats)
		return result, err
	}
	finishResult(&result, state.stats)
	return result, nil
}

func finishResult(result *ScanResult, stats ScanStats) {
	sort.Slice(result.Systems, func(i, j int) bool {
		return result.Systems[i].ID < result.Systems[j].ID
	})
	for index := range result.Systems {
		result.Systems[index].ROMCount = len(result.Systems[index].ROMs)
		sort.Slice(result.Systems[index].ROMs, func(i, j int) bool {
			left := result.Systems[index].ROMs[i]
			right := result.Systems[index].ROMs[j]
			if left.RelativePath != right.RelativePath {
				return left.RelativePath < right.RelativePath
			}
			return left.AbsolutePath < right.AbsolutePath
		})
	}
	stats.Systems = len(result.Systems)
	stats.Issues = len(result.Issues)
	result.Stats = stats
}

func (s *scanner) scanSystem(ctx context.Context, id, path string) (SystemResult, error) {
	result := SystemResult{ID: id, Path: path}
	if err := ctx.Err(); err != nil {
		return result, err
	}

	extensions, err := s.loadExtensions(ctx, path)
	if err != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		kind, message := classifyInfoError(err)
		s.addIssue(&result, filepath.Join(path, "_info.txt"), PhaseInfo, kind, message)
		return result, nil
	}
	result.Extensions = extensions

	extensionSet := make(map[string]struct{}, len(extensions))
	for _, extension := range extensions {
		extensionSet[extension] = struct{}{}
	}
	if err := s.walk(ctx, &result, path, path, extensionSet); err != nil {
		return result, err
	}
	return result, nil
}

func (s *scanner) loadExtensions(ctx context.Context, systemPath string) ([]string, error) {
	infoPath := filepath.Join(systemPath, "_info.txt")
	info, err := s.ops.Lstat(infoPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, errInfoMissing
		}
		return nil, fmt.Errorf("%w: %v", errInfoUnreadable, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		s.stats.SkippedSymlinks++
		return nil, errInfoSymlink
	}
	if !info.Mode().IsRegular() {
		return nil, errInfoNotRegular
	}

	data, err := s.ops.ReadFile(infoPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errInfoUnreadable, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	extensions, err := parseInfo(bytes.NewReader(data))
	if err != nil {
		if errors.Is(err, errNoExtensions) {
			return nil, errInfoInvalid
		}
		return nil, fmt.Errorf("%w: %v", errInfoInvalid, err)
	}
	return extensions, nil
}

func classifyInfoError(err error) (IssueKind, string) {
	switch {
	case errors.Is(err, errInfoMissing):
		return IssueInfoMissing, "system directory has no _info.txt"
	case errors.Is(err, errInfoUnreadable):
		return IssueInfoUnreadable, "could not read _info.txt"
	case errors.Is(err, errInfoSymlink):
		return IssueInfoSymlink, "_info.txt is a symbolic link and was not followed"
	case errors.Is(err, errInfoNotRegular):
		return IssueInfoNotRegular, "_info.txt is not a regular file"
	default:
		return IssueInfoInvalid, "could not parse a supported extension section from _info.txt"
	}
}

func (s *scanner) walk(
	ctx context.Context,
	result *SystemResult,
	systemRoot string,
	dir string,
	extensions map[string]struct{},
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	entries, err := s.ops.ReadDir(dir)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		s.addIssue(result, dir, PhaseWalk, IssueWalk, "could not read directory: "+err.Error())
		return nil
	}

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}

		name := entry.Name()
		if isHidden(name) {
			s.stats.SkippedHidden++
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			s.stats.SkippedSymlinks++
			continue
		}

		fullPath := filepath.Join(dir, name)
		if entry.IsDir() {
			if err := s.walk(ctx, result, systemRoot, fullPath, extensions); err != nil {
				return err
			}
			continue
		}
		if !entry.Type().IsRegular() {
			continue
		}
		if isControlFile(name) {
			continue
		}

		extension := strings.ToLower(filepath.Ext(name))
		if _, supported := extensions[extension]; !supported {
			continue
		}

		info, err := s.ops.Lstat(fullPath)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			s.addIssue(result, fullPath, PhaseStat, IssueEntryStat, "could not stat matching ROM file: "+err.Error())
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			s.stats.SkippedSymlinks++
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}

		relativePath, err := filepath.Rel(systemRoot, fullPath)
		if err != nil {
			s.addIssue(result, fullPath, PhaseStat, IssueEntryStat, "could not compute relative path: "+err.Error())
			continue
		}
		relativePath = filepath.ToSlash(relativePath)

		result.ROMs = append(result.ROMs, ROMEntry{
			SystemID:     result.ID,
			Name:         name,
			AbsolutePath: fullPath,
			RelativePath: relativePath,
			Extension:    extension,
			SizeBytes:    info.Size(),
			ModTime:      info.ModTime(),
			DisplayName:  strings.TrimSuffix(name, filepath.Ext(name)),
		})
		s.stats.ROMFiles++
	}
	return nil
}

func (s *scanner) addIssue(result *SystemResult, path string, phase IssuePhase, kind IssueKind, message string) {
	result.Issues = append(result.Issues, ScanIssue{
		SystemID: result.ID,
		Path:     path,
		Phase:    phase,
		Kind:     kind,
		Message:  message,
	})
}

func isHidden(name string) bool {
	return strings.HasPrefix(name, ".")
}

func isControlFile(name string) bool {
	lower := strings.ToLower(name)
	if lower == "_info.txt" || lower == "gamelist.xml" || lower == "readme" {
		return true
	}
	return strings.HasPrefix(lower, "readme.") || strings.HasSuffix(lower, ".log")
}
