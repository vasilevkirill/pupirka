package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDaemonRunsAgainWhenDeviceIsDue(t *testing.T) {
	root := t.TempDir()
	previous := ConfigV.GetString("path.backup")
	ConfigV.Set("path.backup", root)
	t.Cleanup(func() { ConfigV.Set("path.backup", previous) })
	device := Device{Name: "router", Every: 1}
	if err := os.Mkdir(filepath.Join(root, device.Name), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, device.Name, "backup.rsc"), []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	passes := 0
	runDaemon(ctx, func() (DeviceList, error) {
		passes++
		if passes == 2 {
			cancel()
		}
		return DeviceList{Devices: []Device{device}}, nil
	})
	if passes != 2 {
		t.Fatalf("got %d passes, want 2", passes)
	}
}

func TestDaemonStopsWhileWaiting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	passes := 0
	runDaemon(ctx, func() (DeviceList, error) {
		passes++
		cancel()
		return DeviceList{}, nil
	})
	if passes != 1 {
		t.Fatalf("got %d passes after cancellation", passes)
	}
}

func TestScanDeviceRefreshesFileList(t *testing.T) {
	dir := t.TempDir()
	previousPath := ConfigV.GetString("path.devices")
	previousFiles := DeviceFiles
	ConfigV.Set("path.devices", dir)
	t.Cleanup(func() {
		ConfigV.Set("path.devices", previousPath)
		DeviceFiles = previousFiles
	})
	if err := os.WriteFile(filepath.Join(dir, "first.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ScanDevice(); err != nil {
		t.Fatal(err)
	}
	if len(DeviceFiles) != 1 || DeviceFiles[0] != "first.json" {
		t.Fatalf("first scan: %v", DeviceFiles)
	}
	if err := os.Remove(filepath.Join(dir, "first.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "second.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ScanDevice(); err != nil {
		t.Fatal(err)
	}
	if len(DeviceFiles) != 1 || DeviceFiles[0] != "second.json" {
		t.Fatalf("second scan: %v", DeviceFiles)
	}
}
