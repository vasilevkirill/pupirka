package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// validateBackupName accepts one portable filename component, never a path.
// It rejects traversal, separators, control characters, and reserved names.
func validateBackupName(name string) error {
	if name == "" || name == "." || name == ".." || !fs.ValidPath(name) {
		return fmt.Errorf("invalid path component %q", name)
	}
	if strings.ContainsAny(name, `/\:`) || strings.TrimRight(name, " .") != name {
		return fmt.Errorf("invalid path component %q", name)
	}
	for _, char := range name {
		if unicode.IsControl(char) || strings.ContainsRune(`<>"|?*`, char) {
			return fmt.Errorf("invalid path component %q", name)
		}
	}
	base := strings.ToUpper(strings.TrimRight(strings.SplitN(name, ".", 2)[0], " "))
	if strings.EqualFold(name, ".git") {
		return fmt.Errorf("reserved repository directory %q", name)
	}
	switch base {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return fmt.Errorf("reserved system name %q", name)
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
		return fmt.Errorf("reserved system name %q", name)
	}
	for _, prefix := range []string{"COM", "LPT"} {
		if strings.HasPrefix(base, prefix) && len([]rune(base)) == 4 {
			last := []rune(base)[3]
			if last == '¹' || last == '²' || last == '³' {
				return fmt.Errorf("reserved system name %q", name)
			}
		}
	}
	return nil
}

// validateProjectPath checks every slash-separated component and converts the
// result to the host operating system's path format.
func validateProjectPath(project string) (string, error) {
	if project == "" {
		return "", nil
	}
	for _, part := range strings.Split(project, "/") {
		if err := validateBackupName(part); err != nil {
			return "", err
		}
	}
	local, err := filepath.Localize(project)
	if err != nil || !filepath.IsLocal(local) {
		return "", fmt.Errorf("invalid project path %q", project)
	}
	return local, nil
}

// openDeviceBackupRoot anchors all later file operations to path.backup and
// refuses symlinks at every project and device directory component.
func openDeviceBackupRoot(device *Device) (*os.Root, error) {
	if err := validateBackupName(device.Name); err != nil {
		return nil, fmt.Errorf("device name: %w", err)
	}
	project, err := validateProjectPath(filepath.ToSlash(device.ProjectPath))
	if err != nil {
		return nil, fmt.Errorf("project: %w", err)
	}
	base := ConfigV.GetString("path.backup")
	root, err := os.OpenRoot(base)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(base, 0700); err != nil {
			return nil, fmt.Errorf("create backup root: %w", err)
		}
		root, err = os.OpenRoot(base)
	}
	if err != nil {
		return nil, fmt.Errorf("open backup root: %w", err)
	}
	parts := []string{}
	if project != "" {
		parts = append(parts, strings.Split(filepath.ToSlash(project), "/")...)
	}
	parts = append(parts, device.Name)
	for _, part := range parts {
		info, statErr := root.Lstat(part)
		if os.IsNotExist(statErr) {
			if err := root.Mkdir(part, 0700); err != nil && !os.IsExist(err) {
				_ = root.Close()
				return nil, fmt.Errorf("create backup directory %q: %w", part, err)
			}
			info, statErr = root.Lstat(part)
		}
		if statErr != nil {
			_ = root.Close()
			return nil, fmt.Errorf("inspect backup directory %q: %w", part, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			_ = root.Close()
			return nil, fmt.Errorf("backup directory %q is not a real directory", part)
		}
		next, err := root.OpenRoot(part)
		_ = root.Close()
		if err != nil {
			return nil, fmt.Errorf("open backup directory %q: %w", part, err)
		}
		opened, err := next.Stat(".")
		if err != nil || !os.SameFile(info, opened) {
			_ = next.Close()
			return nil, fmt.Errorf("backup directory %q changed while opening", part)
		}
		root = next
	}
	return root, nil
}

// regularBackupFiles lists only valid regular files belonging to one device.
// Directories, symlinks, special files, and temporary writes are excluded.
func regularBackupFiles(root *os.Root) ([]os.FileInfo, error) {
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	files, readErr := dir.Readdir(-1)
	closeErr := dir.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	regular := make([]os.FileInfo, 0, len(files))
	for _, file := range files {
		if file.Mode().IsRegular() && !strings.HasPrefix(file.Name(), ".pupirka-tmp-") && validateBackupName(file.Name()) == nil {
			regular = append(regular, file)
		}
	}
	return regular, nil
}
