package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func testBackupDevice(name string, project *string) *Device {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return &Device{Name: name, Project: project, Logdevice: logger}
}

func stringPointer(value string) *string { return &value }

func setTestBackupConfig(t *testing.T, root, defaultProject string) {
	t.Helper()
	oldRoot := ConfigV.GetString("path.backup")
	oldProject := ConfigV.GetString("devicedefault.project")
	ConfigV.Set("path.backup", root)
	ConfigV.Set("devicedefault.project", defaultProject)
	t.Cleanup(func() {
		ConfigV.Set("path.backup", oldRoot)
		ConfigV.Set("devicedefault.project", oldProject)
	})
}

func TestProjectLayoutAndDefaults(t *testing.T) {
	root := t.TempDir()
	setTestBackupConfig(t, root, "team/site")
	legacyDir := filepath.Join(root, "inherited")
	if err := os.Mkdir(legacyDir, 0700); err != nil {
		t.Fatal(err)
	}
	legacyFile := filepath.Join(legacyDir, "old.rsc")
	if err := os.WriteFile(legacyFile, []byte("previous backup"), 0600); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name    string
		project *string
		wantDir string
	}{
		{"inherited", nil, filepath.Join(root, "team", "site", "inherited")},
		{"explicit", stringPointer("other/group"), filepath.Join(root, "other", "group", "explicit")},
		{"legacy", stringPointer(""), filepath.Join(root, "legacy")},
	} {
		device := testBackupDevice(tc.name, tc.project)
		if err := SetDefaultParameter(device); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if device.Dirbackup != tc.wantDir {
			t.Fatalf("%s: got %s, want %s", tc.name, device.Dirbackup, tc.wantDir)
		}
		if err := SaveBackupFile(device, []byte(tc.name)); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		got, err := os.ReadFile(filepath.Join(tc.wantDir, device.BackupFileName))
		if err != nil || string(got) != tc.name {
			t.Fatalf("%s: got %q, err %v", tc.name, got, err)
		}
	}
	if got, err := os.ReadFile(legacyFile); err != nil || string(got) != "previous backup" {
		t.Fatalf("existing backup changed: %q, %v", got, err)
	}

	ConfigV.Set("devicedefault.project", "")
	device := testBackupDevice("unchanged", nil)
	if err := SetDefaultParameter(device); err != nil {
		t.Fatal(err)
	}
	if device.Dirbackup != filepath.Join(root, "unchanged") {
		t.Fatalf("empty default changed legacy path: %s", device.Dirbackup)
	}
}

func TestRejectUnsafeBackupPaths(t *testing.T) {
	for _, project := range []string{"/etc", "../etc", "team/../etc", "team//site", "team\\site", "C:/Windows", "CON", "team/NUL.txt", "team/CON .txt", "team/site.", "team/COM¹", "team/.git", "team/\x00"} {
		if _, err := validateProjectPath(project); err == nil {
			t.Errorf("accepted unsafe project %q", project)
		}
	}
	root := t.TempDir()
	setTestBackupConfig(t, root, "")
	for _, name := range []string{"..", "CON", "bad/name"} {
		if err := SetDefaultParameter(testBackupDevice(name, nil)); err == nil {
			t.Errorf("accepted unsafe device name %q", name)
		}
	}
	badFilename := testBackupDevice("router", nil)
	badFilename.FileNameFormat = "../config.rsc"
	if err := SetDefaultParameter(badFilename); err == nil {
		t.Fatal("accepted unsafe backup filename")
	}
	if _, err := os.Stat(filepath.Join(root, "router")); !os.IsNotExist(err) {
		t.Fatalf("invalid paths touched backup root: %v", err)
	}
}

func TestDefaultParametersHandleInvalidPatternsAndPorts(t *testing.T) {
	root := t.TempDir()
	setTestBackupConfig(t, root, "")
	invalidPattern := testBackupDevice("invalid-pattern", nil)
	invalidPattern.Clearstring = "["
	if err := SetDefaultParameter(invalidPattern); err == nil {
		t.Fatal("invalid clearstring regular expression was accepted")
	}

	previousPort := ConfigV.GetString("devicedefault.portshh")
	ConfigV.Set("devicedefault.portshh", "70000")
	t.Cleanup(func() { ConfigV.Set("devicedefault.portshh", previousPort) })
	device := testBackupDevice("invalid-port", nil)
	if err := SetDefaultParameter(device); err != nil {
		t.Fatal(err)
	}
	if device.PortSSH != 22 {
		t.Fatalf("invalid default port became %d, want 22", device.PortSSH)
	}
}

func TestReadDeviceSkipsInvalidProject(t *testing.T) {
	root := t.TempDir()
	setTestBackupConfig(t, root, "")
	deviceDir := t.TempDir()
	previousDeviceDir := ConfigV.GetString("path.devices")
	previousFiles := DeviceFiles
	previousMap := MDeviceList
	ConfigV.Set("path.devices", deviceDir)
	DeviceFiles = []string{"bad.json", "good.json"}
	MDeviceList = make(map[string]Device)
	t.Cleanup(func() {
		ConfigV.Set("path.devices", previousDeviceDir)
		DeviceFiles = previousFiles
		MDeviceList = previousMap
	})
	if err := os.WriteFile(filepath.Join(deviceDir, "bad.json"), []byte(`{"project":"../etc"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deviceDir, "good.json"), []byte(`{"project":"team/site"}`), 0600); err != nil {
		t.Fatal(err)
	}
	var devices DeviceList
	ReadDevice(&devices)
	if len(devices.Devices) != 1 || devices.Devices[0].Name != "good" {
		t.Fatalf("invalid device was not skipped: %+v", devices.Devices)
	}
	if _, err := os.Stat(filepath.Join(root, "bad")); !os.IsNotExist(err) {
		t.Fatalf("invalid device touched backup root: %v", err)
	}
}

func TestBackupRejectsSymlinksOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	marker := filepath.Join(outside, "marker")
	if err := os.WriteFile(marker, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	setTestBackupConfig(t, root, "")
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	device := testBackupDevice("router", stringPointer("escape"))
	if err := SetDefaultParameter(device); err != nil {
		t.Fatal(err)
	}
	if err := SaveBackupFile(device, []byte("new")); err == nil {
		t.Fatal("followed project symlink")
	}
	if err := os.Mkdir(filepath.Join(root, "team"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "team", "site")); err != nil {
		t.Fatal(err)
	}
	nested := testBackupDevice("router", stringPointer("team/site"))
	if err := SetDefaultParameter(nested); err != nil {
		t.Fatal(err)
	}
	if err := SaveBackupFile(nested, []byte("new")); err == nil {
		t.Fatal("followed nested project symlink")
	}
	if err := os.Mkdir(filepath.Join(root, "router"), 0700); err != nil {
		t.Fatal(err)
	}
	plain := testBackupDevice("router", nil)
	if err := SetDefaultParameter(plain); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(marker, filepath.Join(root, "router", plain.BackupFileName)); err != nil {
		t.Fatal(err)
	}
	if err := SaveBackupFile(plain, []byte("new")); err == nil {
		t.Fatal("followed backup file symlink")
	}
	got, err := os.ReadFile(marker)
	if err != nil || string(got) != "untouched" {
		t.Fatalf("outside file changed: %q, %v", got, err)
	}
}

func TestRotationKeepsFiveFilesAndIgnoresProjectDirectory(t *testing.T) {
	root := t.TempDir()
	setTestBackupConfig(t, root, "")
	legacy := testBackupDevice("team", nil)
	project := testBackupDevice("router", stringPointer("team"))
	for _, device := range []*Device{legacy, project} {
		if err := SetDefaultParameter(device); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(project.Dirbackup, 0700); err != nil {
		t.Fatal(err)
	}
	legacyFile := filepath.Join(legacy.Dirbackup, "legacy.rsc")
	if err := os.WriteFile(legacyFile, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-10 * 24 * time.Hour)
	for i := 0; i < 7; i++ {
		file := filepath.Join(project.Dirbackup, "old-"+string(rune('a'+i))+".rsc")
		if err := os.WriteFile(file, []byte("old"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(file, old, old); err != nil {
			t.Fatal(err)
		}
	}
	outside := t.TempDir()
	marker := filepath.Join(outside, "marker")
	if err := os.WriteFile(marker, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(marker, filepath.Join(project.Dirbackup, "linked.rsc")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	project.Rotate = 1
	legacy.Rotate = 1
	list := &DeviceList{Devices: []Device{*legacy, *project}}
	RotateDevice(list)
	entries, err := os.ReadDir(project.Dirbackup)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "old-") {
			count++
		}
	}
	if count != 5 {
		t.Fatalf("rotation kept %d old backups, want 5", count)
	}
	if got, err := os.ReadFile(legacyFile); err != nil || string(got) != "keep" {
		t.Fatalf("legacy backup changed: %q, %v", got, err)
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "keep" {
		t.Fatalf("outside file changed: %q, %v", got, err)
	}
}
