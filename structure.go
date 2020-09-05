package main

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

type Device struct {
	Name           string         `json:"-"`
	Address        string         `json:"address"`
	PortSSH        uint16         `json:"portssh"`
	Username       string         `json:"username"`
	Password       string         `json:"password"`
	Key            string         `json:"key"`
	Timeout        int            `json:"timeout"`
	Every          int            `json:"every"`
	Rotate         int            `json:"rotate"`
	Command        string         `json:"command"`
	Parent         string         `json:"parent"`
	Prefix         string         `json:"prefix"`
	TimeFormat     string         `json:"timeformat"`
	FileNameFormat string         `json:"filenameformat"`
	Clearstring    string         `json:"clearstring"`
	Dirbackup      string         `json:"-"`
	Lastbackup     string         `json:"-"`
	Authkey        bool           `json:"-" default:"false"`
	BackupFileName string         `json:"-"`
	Logdevice      *logrus.Logger `json:"-"`
}
type DeviceList struct {
	Devices []Device
}

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

func (device *Device) LogError(s ...interface{}) {
	device.Logdevice.Errorln(s...)
	device.Logdevice.Infoln(s...)
	device.Logdevice.Debugln(s...)
	device.Logdevice.Warnln(s...)
	LogConsole.Errorln(s...)
	LogGlobal.Errorln(s...)
}
func (device *Device) LogInfo(s ...interface{}) {
	device.Logdevice.Infoln(s...)
	device.Logdevice.Debugln(s...)
}
func (device *Device) LogWarn(s ...interface{}) {
	device.Logdevice.Warnln(s...)
	device.Logdevice.Debugln(s...)
	LogGlobal.Warnln(s...)
}
func (device *Device) LogDebug(s ...interface{}) {
	device.Logdevice.Debug(s...)
	LogGlobal.Debugln(s...)
	LogConsole.Debugln(s...)
}
