package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/juju/fslock"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"gopkg.in/natefinch/lumberjack.v2"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

var ConfigV = viper.New()
var DeviceFiles []string
var LogGlobal = logrus.New()
var LogConsole = logrus.New()

// init loads configuration and prepares logging and Git before the first run.
func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	ConfigV.SetConfigType("json")
	ConfigV.SetConfigName("pupirka.config")
	ConfigV.AddConfigPath("./")
	ConfigV.SetDefault("path.backup", "./backup")
	ConfigV.SetDefault("path.key", "./keys")
	ConfigV.SetDefault("ssh.knownhosts", false)
	ConfigV.SetDefault("path.devices", "./device")
	ConfigV.SetDefault("path.log", "./log")
	ConfigV.SetDefault("path.templates", "./templates")
	ConfigV.SetDefault("devicedefault.portshh", 22) //todo need delete
	ConfigV.SetDefault("devicedefault.portssh", 22)
	ConfigV.SetDefault("devicedefault.timeout", 10)
	ConfigV.SetDefault("devicedefault.every", 3600)
	ConfigV.SetDefault("devicedefault.rotate", 730)
	ConfigV.SetDefault("devicedefault.key", "")
	ConfigV.SetDefault("devicedefault.command", "/export")
	ConfigV.SetDefault("devicedefault.template", "")
	ConfigV.SetDefault("devicedefault.timeformat", "20060102T1504")
	ConfigV.SetDefault("devicedefault.prefix", "")
	ConfigV.SetDefault("devicedefault.filenameformat", "%p%t.rcs")
	ConfigV.SetDefault("devicedefault.clearstring", "")
	ConfigV.SetDefault("devicedefault.project", "")
	ConfigV.SetDefault("devicedefault.hook.skip", "")
	ConfigV.SetDefault("devicedefault.hook.backup", "")
	ConfigV.SetDefault("devicedefault.hook.error", "")
	ConfigV.SetDefault("process.max", 10)
	ConfigV.SetDefault("log.maxday", 1)
	ConfigV.SetDefault("log.maxsize", 10)
	ConfigV.SetDefault("log.maxbackups", 10)
	ConfigV.SetDefault("log.format", "text")
	ConfigV.SetDefault("log.level", "info")
	ConfigV.SetDefault("global.onlyone", false)
	ConfigV.SetDefault("global.hook.pre", "")
	ConfigV.SetDefault("global.hook.post", "")
	ConfigV.SetDefault("git.email", "vk@mikrotik.me")
	ConfigV.SetDefault("git.user", "Pupirka")
	ConfigV.SetDefault("git.password", "")
	ConfigV.SetDefault("git.branch", "master")
	if err := ConfigV.ReadInConfig(); err != nil { // error read config
		log.Println(err)
		_ = ConfigV.SafeWriteConfig()
		//todo need try other exception
	}
	if _, err := validateProjectPath(ConfigV.GetString("devicedefault.project")); err != nil {
		log.Fatalf("invalid devicedefault.project: %v", err)
	}
	if name := ConfigV.GetString("devicedefault.template"); name != "" {
		if err := validateTemplateName(name); err != nil {
			log.Fatalf("invalid devicedefault.template: %v", err)
		}
	}

	for _, path := range ConfigV.GetStringMapString("path") {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			_ = os.Mkdir(path, os.ModePerm)
			log.Printf("Create Folder %s", path)
		}
	}
	LogConsole.SetOutput(os.Stdout)
	switch ConfigV.GetString("log.level") {
	case "info":
		LogConsole.SetLevel(logrus.InfoLevel)
	case "debug":
		LogConsole.SetLevel(logrus.DebugLevel)
	default:
		LogConsole.SetLevel(logrus.InfoLevel)
	}
	LogGlobal.SetOutput(&lumberjack.Logger{
		Filename:   fmt.Sprintf("%s/%s", ConfigV.GetString("path.log"), "pupirka.log"),
		MaxSize:    0,
		MaxAge:     ConfigV.GetInt("log.maxday"),
		MaxBackups: 0,
		LocalTime:  true,
		Compress:   false,
	})
	LogGlobal.SetLevel(LogConsole.GetLevel())
	if err := RunGlobalPreStart(); err != nil {
		log.Fatalln(err)
	}
}

// main runs a single backup pass or keeps scheduling passes in daemon mode.
func main() {
	daemon := flag.Bool("daemon", false, "keep running and back up devices according to their every interval")
	flag.Parse()
	filenameLock := "../pupirka.lock"
	if ConfigV.GetBool("global.onlyone") == true {
		lock := fslock.New(filenameLock)
		err := lock.TryLock()
		if err != nil {
			LogConsole.Info("Only One start Process Pupirka")
			return
		}
		defer lock.Unlock()
	}
	LogConsole.Info("Starting....")
	if *daemon {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		runDaemon(ctx, func() (DeviceList, error) { return runBackupCycle(false) })
		return
	}
	if _, err := runBackupCycle(true); err != nil {
		LogConsole.Errorln(err)
		os.Exit(1)
	}
}

// runBackupCycle reloads device files, rotates old backups, runs due jobs, and
// pushes Git changes. The legacy one-shot mode keeps its five-second delay.
func runBackupCycle(waitBeforeBackup bool) (DeviceList, error) {
	RunnningGlobalHookPre()
	LogConsole.Info("Scan Devices....")
	if err := ScanDevice(); err != nil {
		return DeviceList{}, err
	}
	MDeviceList = make(map[string]Device)
	var devices DeviceList
	ReadDevice(&devices)
	if len(devices.Devices) == 0 {
		LogConsole.Info("Not Device for backup")
		return devices, nil
	}
	LogConsole.Infof("Devices count %d", len(devices.Devices))
	RotateDevice(&devices)
	if waitBeforeBackup {
		time.Sleep(5 * time.Second)
	}
	RunBackups(&devices)
	if err := gitClient.CheckPush(); err != nil {
		LogConsole.Errorln(err)
	}
	RunnningGlobalHookPost()
	return devices, nil
}

// runDaemon runs a complete pass immediately, then wakes when a backup is due
// or after 30 seconds to discover changed device configuration.
func runDaemon(ctx context.Context, cycle func() (DeviceList, error)) {
	LogConsole.Info("Daemon mode started")
	for ctx.Err() == nil {
		devices, err := cycle()
		if err != nil {
			LogConsole.Errorln(err)
		}
		delay := nextBackupDelay(devices)
		LogConsole.Infof("Next device check in %s", delay)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
	LogConsole.Info("Daemon mode stopped")
}

// RunnningGlobalHookPre executes the configured hook before a backup pass.
func RunnningGlobalHookPre() {
	LogConsole.Info("Running Hook....")
	command := ConfigV.GetString("global.hook.pre")
	if command == "" {
		LogConsole.Info("Running Not Hook")
		return
	}

	o, err := RunCommandInOS(command)
	if err != nil {
		LogConsole.Warn(fmt.Sprintf("Error HOOK %s", err.Error()))
		return
	}
	LogConsole.Info(fmt.Sprintf("Result HOOK %s", o))
}

// RunnningGlobalHookPost executes the configured hook after a backup pass.
func RunnningGlobalHookPost() {
	command := ConfigV.GetString("global.hook.post")
	LogConsole.Info("Running Hook....")
	if command == "" {
		LogConsole.Info("Running Not Hook")
		return
	}

	o, err := RunCommandInOS(command)
	if err != nil {
		LogConsole.Warn(fmt.Sprintf("Error HOOK %s", err.Error()))
		return
	}
	LogConsole.Info(fmt.Sprintf("Result HOOK %s", o))
}

// RunCommandInOS runs a configured local hook as a program and arguments.
// It waits for completion and returns the program's stdout.
func RunCommandInOS(command string) (string, error) {
	execComandi := strings.Fields(command)
	if len(execComandi) == 0 {
		return "", fmt.Errorf("hook command is empty")
	}
	res, err := exec.Command(execComandi[0], execComandi[1:]...).Output()
	if err != nil {
		return "", err
	}
	return string(res), nil
}
