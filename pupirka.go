package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gammazero/workerpool"
	logrus "github.com/sirupsen/logrus"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Logger logrus.Logger

func ScanDevice() {
	dir := ConfigV.GetString("path.devices")

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		es := fmt.Sprintf(err.Error())
		LogConsole.Error(es)
		LogConsole.Error("Program exit")
		os.Exit(1)
	}

	for _, f := range files {
		fr := regexp.MustCompile(`\.json$`)
		if frr := string(fr.Find([]byte(f.Name()))); frr == "" {
			es := fmt.Sprintf("File %s skipped, not valid file extension", f.Name())
			LogConsole.Error(es)
			continue
		}
		DeviceFiles = append(DeviceFiles, f.Name())
	}
}

func ReadDevice(Dev *DeviceList) {
	for _, f := range DeviceFiles {
		filepath := fmt.Sprintf("%s/%s", ConfigV.GetString("path.devices"), f)
		jsonFile, err := os.Open(filepath)
		if err != nil {
			es := fmt.Sprintf("Error file Open %s, Error:%s, Skip file", f, err.Error())
			LogConsole.Error(es)
			continue
		}
		defer jsonFile.Close()

		byteValueFromFile, err := ioutil.ReadAll(jsonFile)
		if err != nil {

			es := fmt.Sprintf("Error file Read %s, Error:%s, Skip file", f, err.Error())
			LogConsole.Error(es)
			jsonFile.Close()
			continue
		}
		var d Device
		err = json.Unmarshal(byteValueFromFile, &d)
		if err != nil {
			es := fmt.Sprintf("Error Read file json  %s, Error:%s, Skip file", f, err.Error())
			LogConsole.Error(es)
			jsonFile.Close()
			continue
		}
		d.Name = f[:len(f)-5]
		d.LogConfig()
		SetDefaultParameter(&d)

		MDeviceList[d.Name] = d
		Dev.Devices = append(Dev.Devices, d)
	}
}

func RotateDevice(device *DeviceList) {
	LogConsole.Info("Rotate device list...")

	for i, d := range device.Devices {
		d.LogDebug("RotateDevice: check directory backup", d.Name, d.Dirbackup)
		if _, err := os.Stat(d.Dirbackup); os.IsNotExist(err) {
			_ = os.Mkdir(d.Dirbackup, os.ModePerm)
			LogConsole.Info(fmt.Sprintf("Create Folder %s for backup ", d.Dirbackup))
			continue
		}
		d.LogDebug("RotateDevice: read folder backup", d.Name, d.Dirbackup)
		files, err := ioutil.ReadDir(d.Dirbackup)
		if err != nil {
			es := fmt.Sprintf("Error read folder backup %s, Error:%s", d.Dirbackup, err.Error())
			LogConsole.Error(es)
			device.Devices[i].StatusJob = "skip"
			continue
		}
		d.LogDebug("RotateDevice: Find old backup", d.Name, d.Dirbackup)
		for _, f := range files {
			d.LogDebug(fmt.Sprintf("RotateDevice: Check File %s, time %s", f.Name(), f.ModTime().String()))
			now := time.Now()
			fdifftimesecond := now.Sub(f.ModTime()).Seconds()
			diffday := fdifftimesecond / 60 / 24
			if int(diffday) > d.Rotate {
				d.LogDebug(fmt.Sprintf("RotateDevice:File %s old backup for device %s", f.Name(), d.Name))
				if len(files) > 5 {
					d.LogDebug("RotateDevice: File Count > 5 in backup", d.Name)
					d.LogDebug("RotateDevice: Remove old files", f.Name())
					err := os.Remove(fmt.Sprintf("%s/%s", d.Dirbackup, f.Name()))
					if err != nil {
						es := fmt.Sprintf("Error Remove file %s, Error:%s", f.Name(), err.Error())
						LogConsole.Error(es)
						continue
					}
				}

			}
			if int(fdifftimesecond) < d.Every {
				d.LogDebug(fmt.Sprintf("RotateDevice: control check file time (%f), need time (%d)", fdifftimesecond, d.Every))
				d.LogDebug("RotateDevice: Device no need backup", d.Name)
				device.Devices[i].StatusJob = "skip"
				break
			}

		}

	}
	/*
		newDev := DeviceList{}
		for _, d := range device.Devices {
			if d.Name == "" {
				continue
			}
			newDev.Devices = append(newDev.Devices, d)
		}
		device.Devices = newDev.Devices
	*/
}

func RunBackups(Dev *DeviceList) {
	LogConsole.Info("Backup Start ---->")
	//todo need create dynamic group, curent group 10 and wait all execute after next 10.
	//todo need if one exit group, send one next to group
	wp := workerpool.New(ConfigV.GetInt("process.max"))
	for _, d := range Dev.Devices {
		d := d
		if d.StatusJob == "skip" {
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

func backup(device *Device) {

	LogConsole.Warn(fmt.Sprintf("Starting backup %s ...", device.Name))
	device.LogDebug("Starting backup ", device.Name)
	var bytefromsshclient []byte

	if device.Parent != "" {
		device.LogDebug("backup: parent exist ", device.Name)
		parent, child, err := SshNeedForward(device)
		if err != nil {
			ers := fmt.Sprintf("Fatal error Device:%s get parent %s error: %s", device.Name, device.Parent, err.Error())
			LogConsole.Error(ers)
			device.LogError(ers)
			device.StatusJob = "error"
			return
		}

		newd := SshForwardNewDevice(parent, child)

		bytefromsshclient, err = SshClientRun(&newd)
		if err != nil {
			ers := fmt.Sprintf("Fatal error Forward Device:%s: %s", device.Name, err.Error())
			LogConsole.Error(ers)
			device.LogError(ers)
			device.StatusJob = "error"
			return
		}
	} else {
		device.LogDebug("backup: no parent ", device.Name)
		bytefromssh, err := SshClientRun(device)
		if err != nil {
			ers := fmt.Sprintf("Fatal error Device:%s: %s", device.Name, err.Error())
			LogConsole.Error(ers)
			device.LogError(ers)
			device.StatusJob = "error"
			return
		}
		bytefromsshclient = bytefromssh
	}

	if bytefromsshclient == nil {
		ers := fmt.Sprintf("Fatal error Device:%s not bytes for backup", device.Name)
		LogConsole.Error(ers)
		device.LogError(ers)
		device.StatusJob = "error"
		return
	}
	err := SaveBackupFile(device, bytefromsshclient)
	if err != nil {
		ers := fmt.Sprintf("Bad saved config device %s Error: %s", device.Name, err.Error())
		LogConsole.Error(ers)
		device.LogError(ers)
		device.StatusJob = "error"
		return
	}
	device.StatusJob = "backup"
	device.LogInfo("Backup complete")

}

func SaveBackupFile(device *Device, b []byte) error {

	device.LogDebug(fmt.Sprintf("SaveBackupFile: saved backups... %s", device.Name))
	backupfile := fmt.Sprintf("%s/%s", device.Dirbackup, device.BackupFileName)
	result := b
	if device.Clearstring != "" {
		device.LogDebug(fmt.Sprintf("SaveBackupFile: Need clear string in config... %s", device.Name))
		result = RemoveStringFromBakcup(device, b)
	}
	device.LogDebug(fmt.Sprintf("SaveBackupFile: Create file... %s", backupfile))
	fn, err := os.Create(backupfile)
	if err != nil {
		return errors.New(fmt.Sprintf("SaveBackupFile: Create file Error:%s", err.Error()))
	}
	device.LogDebug(fmt.Sprintf("SaveBackupFile: Write to file... %s", backupfile))
	_, err = fn.Write(result)
	if err != nil {
		return errors.New(fmt.Sprintf("SaveBackupFile: Write to file Error:%s", err.Error()))
	}
	device.LogDebug(fmt.Sprintf("SaveBackupFile: Close file... %s", backupfile))
	_ = fn.Close()
	return nil
}

func RemoveStringFromBakcup(device *Device, b []byte) []byte {
	device.LogDebug(fmt.Sprintf("RemoveStringFromBakcup: ... %s", device.Name))
	device.LogDebug(fmt.Sprintf("RemoveStringFromBakcup: ... %s", device.Name))
	regstr := fmt.Sprintf(`(?m:^%s.*$)`, device.Clearstring)
	device.LogDebug(fmt.Sprintf("RemoveStringFromBakcup: regexp %s", regstr))
	re := regexp.MustCompile(regstr)
	device.LogDebug(fmt.Sprintf("RemoveStringFromBakcup: replace string %s", device.Name))
	config := re.ReplaceAllString(string(b), "")
	config = strings.Trim(config, "\r\n")
	device.LogDebug(fmt.Sprintf("RemoveStringFromBakcup: Remove empty string %s", device.Name))
	return []byte(config)
}

func SetDefaultParameter(d *Device) {

	if d.Timeout == 0 {

		d.Timeout = ConfigV.GetInt("devicedefault.timeout")
		d.LogDebug("SetDefaultParameter: Set default Timeout", d.Name, d.Timeout)
	}
	if d.Every == 0 {

		d.Every = ConfigV.GetInt("devicedefault.every")

		d.LogDebug("SetDefaultParameter: Set default Every", d.Name, d.Every)
	}
	if d.Rotate == 0 {

		d.Rotate = ConfigV.GetInt("devicedefault.rotate")
		d.LogDebug("SetDefaultParameter: Set default Rotate", d.Name, d.Rotate)
	}
	if d.Command == "" {

		d.Command = ConfigV.GetString("devicedefault.command")
		d.LogDebug("SetDefaultParameter: Set default Command", d.Name, d.Command)
	}
	if d.PortSSH == 0 {
		d.LogDebug("SetDefaultParameter: Set default PortSSH", d.Name)
		p, err := strconv.Atoi(ConfigV.GetString("devicedefault.portshh"))
		if err != nil {

			d.PortSSH = 22
			d.LogDebug("SetDefaultParameter: Error parse ssh port field", d.PortSSH)
		}

		if err == nil || p < 65535 || p > 0 {
			d.PortSSH = uint16(p)
			d.LogDebug("SetDefaultParameter: Set default PortSSH", d.Name, d.PortSSH)
		} else {

			d.PortSSH = 22
			d.LogDebug("SetDefaultParameter: Error port ssh not range 1-65535", d.Name, d.PortSSH)
		}
	}
	d.Authkey = false
	if d.Key != "" {
		d.LogDebug("SetDefaultParameter: Need use private key uniq", d.Name, d.Key)
		d.Authkey = true
	}

	if d.Authkey == false && d.Password == "" && d.Key != "" {
		d.LogDebug("SetDefaultParameter: Need use private key uniq", d.Name, d.Key)
		d.Authkey = true
	}
	if d.Authkey == false && d.Password == "" && d.Key == "" && ConfigV.GetString("devicedefault.key") != "" {

		d.Authkey = true
		d.Key = ConfigV.GetString("devicedefault.key")
		d.LogDebug("SetDefaultParameter: Need use private key default", d.Name, d.Key)
	}

	if d.TimeFormat == "" {
		d.TimeFormat = ConfigV.GetString("devicedefault.timeformat")
		d.LogDebug("SetDefaultParameter: Set default time format", d.Name, d.TimeFormat)
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
	d.Dirbackup = fmt.Sprintf("%s/%s", ConfigV.GetString("path.backup"), d.Name)
	ditetimestring := time.Now().Format(d.TimeFormat)
	nameinprefix := strings.ReplaceAll(d.FileNameFormat, "%p", d.Prefix)
	nameintime := strings.ReplaceAll(nameinprefix, "%t", ditetimestring)
	d.BackupFileName = nameintime
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
}
