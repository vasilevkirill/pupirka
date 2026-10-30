package main

import (
	"github.com/sirupsen/logrus"
)

// Device holds the JSON settings and the resolved runtime state for one
// configured network device. Fields tagged json:"-" are computed per pass.
type Device struct {
	Name             string         `json:"-"`
	Address          string         `json:"address"`
	PortSSH          uint16         `json:"portssh"`
	Username         string         `json:"username"`
	Password         string         `json:"password"`
	Key              string         `json:"key"`
	Timeout          int            `json:"timeout"`
	Every            int            `json:"every"`
	Rotate           int            `json:"rotate"`
	Command          string         `json:"command"`
	Template         *string        `json:"template"`
	TemplateName     string         `json:"-"`
	Vars             map[string]any `json:"vars"`
	Parent           string         `json:"parent"`
	Project          *string        `json:"project"`
	ProjectPath      string         `json:"-"`
	Prefix           string         `json:"prefix"`
	TimeFormat       string         `json:"timeformat"`
	FileNameFormat   string         `json:"filenameformat"`
	Clearstring      string         `json:"clearstring"`
	DeviceHooks      DeviceHook     `json:"hook"`
	Dirbackup        string         `json:"-"`
	Lastbackup       string         `json:"-"`
	Authkey          bool           `json:"-" default:"false"`
	BackupFileName   string         `json:"-"`
	KnownHostAddress string         `json:"-"`
	StatusJob        string         `json:"-"`
	Logdevice        *logrus.Logger `json:"-"`
}

// DeviceList is the set of valid devices scheduled in one backup pass.
type DeviceList struct {
	Devices []Device
}

// DeviceHook stores local commands for each possible backup outcome.
type DeviceHook struct {
	Backup string `json:"backup"`
	Skip   string `json:"skip"`
	Error  string `json:"error"`
}
