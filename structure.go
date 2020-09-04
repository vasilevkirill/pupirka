package main

type Device struct {
	Name           string `json:"-"`
	Address        string `json:"address"`
	PortSSH        uint16 `json:"portssh"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	Key            string `json:"key"`
	Timeout        int    `json:"timeout"`
	Every          int    `json:"every"`
	Rotate         int    `json:"rotate"`
	Command        string `json:"command"`
	Parent         string `json:"parent"`
	Prefix         string `json:"prefix"`
	TimeFormat     string `json:"timeformat"`
	FileNameFormat string `json:"filenameformat"`
	Clearstring    string `json:"clearstring"`
	Dirbackup      string `json:"-"`
	Lastbackup     string `json:"-"`
	Authkey        bool   `json:"-" default:"false"`
	BackupFileName string `json:"-"`
}
type DeviceList struct {
	Devices []Device
}
