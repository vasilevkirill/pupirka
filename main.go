package main

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"gopkg.in/natefinch/lumberjack.v2"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

var ConfigV = viper.New()
var DeviceFiles []string
var LogGlobal = logrus.New()
var LogConsole = logrus.New()

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	ConfigV.SetConfigType("json")
	ConfigV.SetConfigName("pupirka.config")
	ConfigV.AddConfigPath("./")
	ConfigV.SetDefault("path.backup", "./backup")
	ConfigV.SetDefault("path.key", "./keys")
	ConfigV.SetDefault("path.devices", "./device")
	ConfigV.SetDefault("path.log", "./log")
	ConfigV.SetDefault("devicedefault.portshh", 22) //todo need delete
	ConfigV.SetDefault("devicedefault.portssh", 22)
	ConfigV.SetDefault("devicedefault.timeout", 10)
	ConfigV.SetDefault("devicedefault.every", 3600)
	ConfigV.SetDefault("devicedefault.rotate", 730)
	ConfigV.SetDefault("devicedefault.key", "")
	ConfigV.SetDefault("devicedefault.command", "/export")
	ConfigV.SetDefault("devicedefault.timeformat", "20060102T1504")
	ConfigV.SetDefault("devicedefault.prefix", "")
	ConfigV.SetDefault("devicedefault.filenameformat", "%p%t.rcs")
	ConfigV.SetDefault("devicedefault.clearstring", "")
	ConfigV.SetDefault("devicedefault.hook.skip", "")
	ConfigV.SetDefault("devicedefault.hook.backup", "")
	ConfigV.SetDefault("devicedefault.hook.error", "")
	ConfigV.SetDefault("process.max", 10)
	ConfigV.SetDefault("log.maxday", 1)
	ConfigV.SetDefault("log.maxsize", 10)
	ConfigV.SetDefault("log.maxbackups", 10)
	ConfigV.SetDefault("log.format", "text")
	ConfigV.SetDefault("log.level", "info")
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

func main() {
	LogConsole.Info("Starting....")
	LogConsole.Info("Scan Devices....")

	RunnningGlobalHookPre()
	ScanDevice()
	var Dev DeviceList

	ReadDevice(&Dev)
	if len(Dev.Devices) == 0 {
		LogConsole.Info("Not Device for backup")
		os.Exit(0)
	}
	LogConsole.Info(fmt.Sprintf("Devices count %d", len(Dev.Devices)))
	RotateDevice(&Dev)

	if len(Dev.Devices) == 0 {
		LogConsole.Info("All Device backups actual")
		os.Exit(0)
	}
	time.Sleep(5 * time.Second)
	RunBackups(&Dev)
	err := gitClient.CheckPush()
	if err != nil {
		LogConsole.Errorln(err)
	}
	RunnningGlobalHookPost()
}

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
func RunCommandInOS(command string) (string, error) {
	execComandi := strings.Fields(command)
	res, err := exec.Command(execComandi[0], execComandi[1:]...).Output()
	if err != nil {
		return "", err
	}
	return string(res), nil
}
