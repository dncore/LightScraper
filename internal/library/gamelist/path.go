package gamelist

import (
	"path/filepath"
	"strings"
)

// classifyPath performs lexical classification only. It never calls os.Stat,
// os.Lstat, filepath.EvalSymlinks or any other filesystem operation.
//
// ES treats a path beginning with "./" (or ".\\") as relative to the system
// directory. A bare relative path is not rebased by the upstream resolver and
// is therefore reported as unsupported rather than guessed. "~/" is a media
// path convention and is not expanded for ROM paths.
func classifyPath(systemDir, raw string) PathInfo {
	info := PathInfo{Raw: raw}
	if raw == "" || strings.IndexByte(raw, 0) >= 0 {
		info.Status = PathInvalid
		return info
	}

	// ES accepts both slash styles in the "./" and ".\\" prefixes. Normalize
	// only for classification; PathInfo.Raw remains untouched.
	path := strings.ReplaceAll(raw, `\`, "/")

	if path == "~" || strings.HasPrefix(path, "~/") {
		info.Status = PathHomeUnsupported
		return info
	}

	if systemDir == "" {
		info.Status = PathInvalid
		return info
	}
	base, err := filepath.Abs(systemDir)
	if err != nil {
		info.Status = PathInvalid
		return info
	}
	base = filepath.Clean(base)

	if path == "." {
		info.Status = PathRelative
		info.Resolved = base
		return info
	}

	if strings.HasPrefix(path, "./") {
		return classifyWithin(info, base, path[2:], PathRelative)
	}
	if path == ".." || strings.HasPrefix(path, "../") {
		info.Status = PathOutside
		return info
	}
	if isWindowsDriveRelative(path) {
		// C:foo is drive-relative on Windows and has no portable, safe base.
		info.Status = PathOutside
		return info
	}

	if isWindowsAbsolute(path) && !filepath.IsAbs(filepath.FromSlash(path)) {
		// A Windows-style absolute path on a non-Windows host cannot be
		// lexically contained by a POSIX system directory. Do not attempt to
		// convert it into a local path.
		info.Status = PathOutside
		return info
	}

	if strings.HasPrefix(path, "/") && !filepath.IsAbs(filepath.FromSlash(path)) {
		// On Windows a rooted path such as "/etc" is not filepath.IsAbs, but it
		// is still outside a caller-selected system directory.
		info.Status = PathOutside
		return info
	}

	if filepath.IsAbs(filepath.FromSlash(path)) {
		candidate := filepath.Clean(filepath.FromSlash(path))
		if pathIsWithin(base, candidate) {
			info.Status = PathAbsoluteInside
			info.Resolved = candidate
			return info
		}
		info.Status = PathOutside
		return info
	}

	info.Status = PathBareRelative
	return info
}

func classifyWithin(info PathInfo, base, path string, insideStatus PathStatus) PathInfo {
	// A path such as ".//etc/passwd" has an absolute-looking remainder after
	// stripping "./". It must not be silently rebased under systemDir.
	if strings.HasPrefix(path, "/") || isWindowsAbsolute(path) {
		info.Status = PathOutside
		return info
	}
	candidate := filepath.Clean(filepath.Join(base, filepath.FromSlash(path)))
	if pathIsWithin(base, candidate) {
		info.Status = insideStatus
		info.Resolved = candidate
		return info
	}
	info.Status = PathOutside
	return info
}

func pathIsWithin(base, candidate string) bool {
	relative, err := filepath.Rel(base, candidate)
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func isWindowsAbsolute(path string) bool {
	if len(path) >= 3 && isASCIIAlpha(path[0]) && path[1] == ':' && (path[2] == '/' || path[2] == '\\') {
		return true
	}
	return strings.HasPrefix(path, "//")
}

func isWindowsDriveRelative(path string) bool {
	return len(path) >= 2 && isASCIIAlpha(path[0]) && path[1] == ':'
}

func isASCIIAlpha(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}
