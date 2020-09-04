package main

type Device struct {
	Name             string `json:"-"`
	NameBackupPrefix string `json:"prefix"`
	Address          string `json:"address"`
	PortSSH          uint16 `json:"portssh"`
	Username         string `json:"username"`
	Password         string `json:"password"`
	Key              string `json:"key"`
	Timeout          int    `json:"timeout"`
	Every            int    `json:"every"`
	Rotate           int    `json:"rotate"`
	Dirbackup        string `json:"-"`
	Command          string `json:"command"`
	Lastbackup       string `json:"-"`
	Authkey          bool   `json:"-" default:"false"`
	Parent           string `json:"parent"`
}
type DeviceList struct {
	Devices []Device
}
