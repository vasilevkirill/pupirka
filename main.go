package main

import (
	"github.com/spf13/viper"
	"log"
	"os"
)

var ConfigV = viper.New()
var DeviceFiles []string

func init() {
	ConfigV.SetConfigType("json")
	ConfigV.SetConfigName("pupirka.config")
	ConfigV.AddConfigPath("./")
	ConfigV.SetDefault("path.backup", "./backup")
	//ConfigV.SetDefault("path.key", "./key")
	ConfigV.SetDefault("path.devices", "./device")
	ConfigV.SetDefault("devicedefault.portshh", 22)
	ConfigV.SetDefault("devicedefault.timeout", 10)
	ConfigV.SetDefault("devicedefault.every", 3600)
	ConfigV.SetDefault("devicedefault.rotate", 730)
	ConfigV.SetDefault("devicedefault.command", "/export")
	if err := ConfigV.ReadInConfig(); err != nil { // error read config
		log.Println(err)
		ConfigV.SafeWriteConfig()
		//todo need try other exception
	}

	for _, path := range ConfigV.GetStringMapString("path") {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			_ = os.Mkdir(path, os.ModePerm)
			log.Printf("Create Folder %s", path)
		}
	}
}

func main() {
	ScanDevice()
	var Dev DeviceList
	ReadDevice(&Dev)
	if len(Dev.Devices) == 0 {
		log.Println("Not Device for backup")
		os.Exit(0)
	}
	RotateDevice(&Dev)
	if len(Dev.Devices) == 0 {
		log.Println("All Device backups actual")
		os.Exit(0)
	}
	RunBackups(&Dev)
}
