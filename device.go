package main

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
	"os/exec"
	"strings"
)

// LogConfig creates the per-device logger using the configured format, level,
// and rotating log file.
func (device *Device) LogConfig() {
	device.Logdevice = logrus.New()

	switch ConfigV.GetString("log.format") {
	case "text":
		device.Logdevice.SetFormatter(&logrus.TextFormatter{})
	case "json":
		device.Logdevice.SetFormatter(&logrus.JSONFormatter{})
	default:
		device.Logdevice.SetFormatter(&logrus.TextFormatter{})
	}

	device.Logdevice.SetOutput(&lumberjack.Logger{
		Filename:   fmt.Sprintf("%s/%s/%s", ConfigV.GetString("path.log"), device.Name, "device.log"),
		MaxSize:    ConfigV.GetInt("log.maxsize"),
		MaxAge:     ConfigV.GetInt("log.maxday"),
		MaxBackups: ConfigV.GetInt("log.maxbackups"),
		LocalTime:  true,
		Compress:   false,
	})

	device.Logdevice.SetLevel(LogConsole.GetLevel())
}

// LogError records a device failure in the device, console, and global logs.
func (device *Device) LogError(s ...interface{}) {
	device.Logdevice.Errorln(s...)
	LogConsole.Errorln(s...)
	LogGlobal.Errorln(s...)
}

// LogInfo records routine device progress in its own log.
func (device *Device) LogInfo(s ...interface{}) {
	device.Logdevice.Infoln(s...)
}

// LogWarn records a device warning in its own log and the global log.
func (device *Device) LogWarn(s ...interface{}) {
	device.Logdevice.Warnln(s...)
	LogGlobal.Warnln(s...)
}

// LogDebug records diagnostic details at debug level.
func (device *Device) LogDebug(s ...interface{}) {
	device.Logdevice.Debugln(s...)
	LogGlobal.Debugln(s...)
	LogConsole.Debugln(s...)
}

// Hook starts the command configured for the device's final job status. The
// command runs asynchronously; its goroutine waits to reap the child process.
func (device *Device) Hook() {
	device.LogDebug(fmt.Sprintf("Run Hook '%s': Device %s", device.StatusJob, device.Name))
	var command string
	switch device.StatusJob {
	case "skip":
		command = device.DeviceHooks.Skip
	case "error":
		command = device.DeviceHooks.Error
	case "backup":
		command = device.DeviceHooks.Backup
	default:
		command = ""
	}
	if command == "" {
		device.LogDebug(fmt.Sprintf("Hook '%s': No command for hook: Device %s", device.StatusJob, device.Name))
		return
	}
	device.LogDebug(fmt.Sprintf("Hook '%s': Command for hook '%s': Device %s", device.StatusJob, command, device.Name))
	rVariable := []string{"%name", "%parent", "%filename", "%address", "%portssh"}
	rValue := []string{device.Name, device.Parent, device.BackupFileName, device.Address, fmt.Sprintf("%d", device.PortSSH)}
	execCommand := strings.NewReplacer(zipStringsForReplacer(rVariable, rValue)...).Replace(command)
	device.LogDebug(fmt.Sprintf("Hook '%s': Command Compile for hook '%s': Device %s", device.StatusJob, execCommand, device.Name))
	execComandi := strings.Fields(execCommand)
	if len(execComandi) == 0 {
		device.LogError("Hook command is empty after substitution")
		return
	}
	cmd := exec.Command(execComandi[0], execComandi[1:]...)
	device.LogDebug(fmt.Sprintf("Hook '%s': Hook Runiing: Device %s", device.StatusJob, device.Name))
	err := cmd.Start()
	if err != nil {
		device.LogError("Error running command: ", err)
		return
	}
	// A long-running process must reap completed hooks to avoid zombies.
	go func() {
		if err := cmd.Wait(); err != nil {
			device.LogError("Hook command failed: ", err)
		}
	}()
}

// zipStringsForReplacer interleaves placeholder names and values for
// strings.NewReplacer.
func zipStringsForReplacer(a1, a2 []string) []string {
	r := make([]string, 2*len(a1))
	for i, e := range a1 {
		r[i*2] = e
		r[i*2+1] = a2[i]
	}
	return r
}
