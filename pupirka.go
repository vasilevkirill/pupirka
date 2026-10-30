package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gammazero/workerpool"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ScanDevice refreshes the list of JSON files in path.devices. Daemon mode
// calls it before every pass so added and removed devices are discovered.
func ScanDevice() error {
	dir := ConfigV.GetString("path.devices")

	files, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read device directory %s: %w", dir, err)
	}

	DeviceFiles = DeviceFiles[:0]
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".json") {
			es := fmt.Sprintf("File %s skipped, not valid file extension", f.Name())
			LogConsole.Error(es)
			continue
		}
		DeviceFiles = append(DeviceFiles, f.Name())
	}
	return nil
}

// ReadDevice parses each discovered file, applies defaults, and skips invalid
// device definitions without changing their backup destination.
func ReadDevice(Dev *DeviceList) {
	for _, f := range DeviceFiles {
		path := filepath.Join(ConfigV.GetString("path.devices"), f)
		contents, err := os.ReadFile(path)
		if err != nil {
			LogConsole.Errorf("Cannot read device file %s: %v; skipping", f, err)
			continue
		}
		var d Device
		err = json.Unmarshal(contents, &d)
		if err != nil {
			LogConsole.Errorf("Cannot parse device file %s: %v; skipping", f, err)
			continue
		}
		d.Name = f[:len(f)-5]
		d.LogConfig()
		if err := SetDefaultParameter(&d); err != nil {
			LogConsole.Errorf("Invalid device %s: %v; skipping", d.Name, err)
			continue
		}

		MDeviceList[d.Name] = d
		Dev.Devices = append(Dev.Devices, d)
	}
}

// RotateDevice removes expired regular files for each device, keeping at least
// five backups, then marks a device as skip when a recent backup exists.
func RotateDevice(device *DeviceList) {
	LogConsole.Info("Rotate device list...")
	for i, d := range device.Devices {
		root, err := openDeviceBackupRoot(&d)
		if err != nil {
			d.LogError("Cannot open backup directory: ", err)
			device.Devices[i].StatusJob = "error"
			continue
		}
		files, err := regularBackupFiles(root)
		if err != nil {
			_ = root.Close()
			d.LogError("Cannot read backup directory: ", err)
			device.Devices[i].StatusJob = "error"
			continue
		}
		sort.Slice(files, func(i, j int) bool { return files[i].ModTime().Before(files[j].ModTime()) })
		remaining := len(files)
		for _, file := range files {
			if remaining <= 5 {
				break
			}
			if time.Since(file.ModTime()) <= time.Duration(d.Rotate)*24*time.Hour {
				continue
			}
			info, err := root.Lstat(file.Name())
			if err != nil || !info.Mode().IsRegular() {
				continue
			}
			if err := root.Remove(file.Name()); err != nil {
				d.LogWarn("Cannot remove old backup: ", err)
				continue
			}
			remaining--
			if err := gitClient.RemoveFile(filepath.Join(d.Dirbackup, file.Name())); err != nil {
				d.LogWarn(err)
			}
		}
		files, err = regularBackupFiles(root)
		_ = root.Close()
		if err != nil {
			d.LogError("Cannot recheck backup directory: ", err)
			device.Devices[i].StatusJob = "error"
			continue
		}
		for _, file := range files {
			if time.Since(file.ModTime()) < time.Duration(d.Every)*time.Second {
				device.Devices[i].StatusJob = "skip"
				break
			}
		}
	}
}

// RunBackups starts due devices in the configured worker pool and waits for
// their backups to finish before the global post-hook or Git push runs.
func RunBackups(Dev *DeviceList) {
	LogConsole.Info("Backup Start ---->")
	wp := workerpool.New(ConfigV.GetInt("process.max"))
	for _, d := range Dev.Devices {
		d := d
		if d.StatusJob == "skip" || d.StatusJob == "error" {
			d.Hook()
			continue
		}
		d.LogDebug("RunBackups: wait groups", d.Name)
		wp.Submit(func() {
			d.LogDebug("RunBackups: enter groups", d.Name)
			backup(&d)
			d.Hook()
		})
		d.LogDebug("RunBackups: Exit groups", d.Name)
	}
	wp.StopWait()
	LogConsole.Info("Backup Finish <----")
}

// nextBackupDelay returns the earliest due time, capped at 30 seconds so new
// device files are noticed. Failed devices get another chance after that cap.
func nextBackupDelay(devices DeviceList) time.Duration {
	const refresh = 30 * time.Second
	const minimum = time.Second
	delay := refresh
	now := time.Now()
	for _, device := range devices.Devices {
		if device.StatusJob == "error" || device.Every <= 0 {
			continue
		}
		root, err := openDeviceBackupRoot(&device)
		if err != nil {
			continue
		}
		files, err := regularBackupFiles(root)
		_ = root.Close()
		if err != nil || len(files) == 0 {
			continue
		}
		latest := files[0].ModTime()
		for _, file := range files[1:] {
			if file.ModTime().After(latest) {
				latest = file.ModTime()
			}
		}
		until := latest.Add(time.Duration(device.Every) * time.Second).Sub(now)
		if until > 0 && until < delay {
			delay = until
		}
	}
	if delay < minimum {
		return minimum
	}
	return delay
}

// backup retrieves one device's configuration, then writes it only after all
// SSH and template steps succeed. A parent device provides an SSH tunnel.
func backup(device *Device) {
	LogConsole.Warnf("Starting backup %s ...", device.Name)
	target := device
	if device.Parent != "" {
		parent, child, err := SshNeedForward(device)
		if err != nil {
			failBackup(device, fmt.Errorf("resolve parent %s: %w", device.Parent, err))
			return
		}
		forwarded := SshForwardNewDevice(parent, child)
		target = &forwarded
	}
	output, err := runDeviceSSH(target)
	if err != nil {
		failBackup(device, fmt.Errorf("retrieve configuration: %w", err))
		return
	}
	if output == nil {
		failBackup(device, fmt.Errorf("device returned no backup data"))
		return
	}
	if err := SaveBackupFile(device, output); err != nil {
		failBackup(device, fmt.Errorf("save configuration: %w", err))
		return
	}
	device.StatusJob = "backup"
	device.LogInfo("Backup complete")
}

// failBackup records one error and leaves the device eligible for a later pass.
func failBackup(device *Device, err error) {
	device.StatusJob = "error"
	device.LogError(fmt.Errorf("backup %s: %w", device.Name, err))
}

// SaveBackupFile compares the processed result with the current file, writes
// changes atomically under path.backup, and records them in Git if configured.
func SaveBackupFile(device *Device, b []byte) error {
	if err := validateBackupName(device.BackupFileName); err != nil {
		return fmt.Errorf("backup filename: %w", err)
	}
	root, err := openDeviceBackupRoot(device)
	if err != nil {
		return err
	}
	defer root.Close()
	backupfile := filepath.Join(ConfigV.GetString("path.backup"), device.ProjectPath, device.Name, device.BackupFileName)
	result := b
	if device.Clearstring != "" {
		result = RemoveStringFromBakcup(device, b)
	}
	info, err := root.Lstat(device.BackupFileName)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if exists {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("backup target %q is not a regular file", device.BackupFileName)
		}
		previous, err := root.ReadFile(device.BackupFileName)
		if err != nil {
			return err
		}
		if bytes.Equal(previous, result) {
			return nil
		}
	}
	if err := writeBackupAtomically(root, device.BackupFileName, result); err != nil {
		return err
	}
	if exists {
		return gitClient.SetCommit(backupfile)
	}
	return gitClient.AddFile(backupfile)
}

// writeBackupAtomically writes a temporary regular file and renames it inside
// the device root, so replacing a file never follows a target symlink.
func writeBackupAtomically(root *os.Root, filename string, data []byte) error {
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		return err
	}
	temporary := ".pupirka-tmp-" + hex.EncodeToString(random[:])
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if info, err := root.Lstat(filename); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("backup target %q is not a regular file", filename)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return root.Rename(temporary, filename)
}

// RemoveStringFromBakcup removes lines matching the device's clearstring
// regular expression before comparing or storing a backup.
func RemoveStringFromBakcup(device *Device, b []byte) []byte {
	regstr := fmt.Sprintf(`(?m:^%s.*$)`, device.Clearstring)
	device.LogDebug("RemoveStringFromBackup: regexp ", regstr)
	re := regexp.MustCompile(regstr)
	config := re.ReplaceAllString(string(b), "")
	config = strings.Trim(config, "\r\n")
	return []byte(config)
}

// SetDefaultParameter resolves inherited settings, validates all backup path
// components, and prepares the filename used by this backup pass.
func SetDefaultParameter(d *Device) error {
	if err := validateBackupName(d.Name); err != nil {
		return fmt.Errorf("device name: %w", err)
	}
	project := ConfigV.GetString("devicedefault.project")
	if d.Project != nil {
		project = *d.Project
	}
	projectPath, err := validateProjectPath(project)
	if err != nil {
		return fmt.Errorf("project: %w", err)
	}
	d.ProjectPath = projectPath
	templateName := ConfigV.GetString("devicedefault.template")
	if d.Template != nil {
		templateName = *d.Template
	}
	if templateName != "" {
		if err := validateTemplateName(templateName); err != nil {
			return fmt.Errorf("template: %w", err)
		}
	}
	d.TemplateName = templateName

	// Numeric and command defaults are resolved only when a device omits them.
	if d.Timeout == 0 {
		d.Timeout = ConfigV.GetInt("devicedefault.timeout")
		d.LogDebug("SetDefaultParameter: Set default Timeout ", d.Name, d.Timeout)
	}
	if d.Every == 0 {
		d.Every = ConfigV.GetInt("devicedefault.every")
		d.LogDebug("SetDefaultParameter: Set default Every ", d.Name, d.Every)
	}
	if d.Rotate == 0 {
		d.Rotate = ConfigV.GetInt("devicedefault.rotate")
		d.LogDebug("SetDefaultParameter: Set default Rotate ", d.Name, d.Rotate)
	}
	if d.Command == "" {
		d.Command = ConfigV.GetString("devicedefault.command")
		d.LogDebug("SetDefaultParameter: Set default Command ", d.Name, d.Command)
	}
	if d.PortSSH == 0 {
		p, err := strconv.Atoi(ConfigV.GetString("devicedefault.portshh"))
		if err != nil || p < 1 || p > 65535 {
			d.PortSSH = 22
			d.LogWarn("Invalid default SSH port; using 22")
		} else {
			d.PortSSH = uint16(p)
		}
		d.LogDebug("SetDefaultParameter: Set default PortSSH ", d.Name, d.PortSSH)
	}

	// A device-specific key takes precedence over password authentication.
	d.Authkey = d.Key != ""
	if d.Key != "" {
		d.LogDebug("SetDefaultParameter: Need use private key uniq ", d.Name, d.Key)
	} else if d.Password == "" && ConfigV.GetString("devicedefault.key") != "" {
		d.Authkey = true
		d.Key = ConfigV.GetString("devicedefault.key")
		d.LogDebug("SetDefaultParameter: Need use private key default ", d.Name, d.Key)
	}

	// The final filename is validated after all substitutions, before any I/O.
	if d.TimeFormat == "" {
		d.TimeFormat = ConfigV.GetString("devicedefault.timeformat")
		d.LogDebug("SetDefaultParameter: Set default time format ", d.Name, d.TimeFormat)
	}
	if d.Prefix == "" {
		d.Prefix = ConfigV.GetString("devicedefault.prefix")
		d.LogDebug("SetDefaultParameter: Set default Prefix", d.Name, d.Prefix)
	}
	if d.FileNameFormat == "" {
		d.FileNameFormat = ConfigV.GetString("devicedefault.filenameformat")
		d.LogDebug("SetDefaultParameter: Set default FileNameFormat", d.Name, d.FileNameFormat)
	}
	if d.Clearstring == "" {
		d.Clearstring = ConfigV.GetString("devicedefault.clearstring")
		d.LogDebug("SetDefaultParameter: Set default Clear string", d.Name, d.Clearstring)
	}
	if d.Clearstring != "" {
		if _, err := regexp.Compile(fmt.Sprintf(`(?m:^%s.*$)`, d.Clearstring)); err != nil {
			return fmt.Errorf("clearstring: %w", err)
		}
	}
	d.Dirbackup = filepath.Join(ConfigV.GetString("path.backup"), d.ProjectPath, d.Name)
	ditetimestring := time.Now().Format(d.TimeFormat)
	nameinprefix := strings.ReplaceAll(d.FileNameFormat, "%p", d.Prefix)
	nameintime := strings.ReplaceAll(nameinprefix, "%t", ditetimestring)
	nameinname := strings.ReplaceAll(nameintime, "%n", d.Name)
	d.BackupFileName = nameinname
	if err := validateBackupName(d.BackupFileName); err != nil {
		return fmt.Errorf("backup filename: %w", err)
	}
	d.LogDebug("SetDefaultParameter: Set default BackupFileName", d.Name, d.BackupFileName)
	if d.DeviceHooks.Error == "" {
		d.DeviceHooks.Error = ConfigV.GetString("devicedefault.hook.error")
		d.LogDebug(fmt.Sprintf("SetDefaultParameter: Set default devicedefault.hook.error Velue %s in Device %s", d.DeviceHooks.Error, d.Name))
	}
	if d.DeviceHooks.Backup == "" {
		d.DeviceHooks.Backup = ConfigV.GetString("devicedefault.hook.backup")
		d.LogDebug(fmt.Sprintf("SetDefaultParameter: Set default devicedefault.hook.backup Velue %s in Device %s", d.DeviceHooks.Backup, d.Name))
	}
	if d.DeviceHooks.Skip == "" {
		d.DeviceHooks.Skip = ConfigV.GetString("devicedefault.hook.skip")
		d.LogDebug(fmt.Sprintf("SetDefaultParameter: Set default devicedefault.hook.skip Velue %s in Device %s", d.DeviceHooks.Skip, d.Name))
	}
	return nil
}
