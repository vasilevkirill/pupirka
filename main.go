package main

import (
	"github.com/spf13/viper"
	"log"
	"os"
)

var ConfigV = viper.New()

func init() {
	ConfigV.SetConfigType("json")
	ConfigV.SetConfigName("pupirka.config")
	ConfigV.AddConfigPath("./")
	ConfigV.SetDefault("path.backup", "./backup")
	ConfigV.SetDefault("path.key", "./key")
	ConfigV.SetDefault("path.devices", "./device")
	if err := ConfigV.ReadInConfig(); err != nil { // error read config

		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			_ = ConfigV.WriteConfig()
		}
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

}
