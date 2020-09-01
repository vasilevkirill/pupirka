package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"golang.org/x/crypto/ssh"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strconv"
	"time"
)

func ScanDevice() {
	dir := ConfigV.GetString("path.devices")

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Println(err.Error())
		log.Fatalf("Program exit")
	}

	for _, f := range files {
		fr := regexp.MustCompile(`\.json$`)
		if frr := string(fr.Find([]byte(f.Name()))); frr == "" {
			log.Printf("File %s skipped, not valid file extension", f.Name())
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
			log.Printf("Error file Open %s, Error:%s, Skip file", f, err.Error())
			continue
		}
		defer jsonFile.Close()

		byteValueFromFile, err := ioutil.ReadAll(jsonFile)
		if err != nil {
			log.Printf("Error file Read %s, Error:%s, Skip file", f, err.Error())
			jsonFile.Close()
			continue
		}
		var d Device
		err = json.Unmarshal(byteValueFromFile, &d)
		if err != nil {
			log.Printf("Error Read file json  %s, Error:%s, Skip file", f, err.Error())
			jsonFile.Close()
			continue
		}
		d.Name = f[:len(f)-5]
		if d.Timeout == 0 {
			d.Timeout = ConfigV.GetInt("devicedefault.timeout")
		}
		if d.Every == 0 {
			d.Every = ConfigV.GetInt("devicedefault.every")
		}
		if d.Rotate == 0 {
			d.Rotate = ConfigV.GetInt("devicedefault.rotate")
		}
		if d.Command == "" {
			d.Command = ConfigV.GetString("devicedefault.command")
		}
		if d.PortSSH == 0 {
			p, err := strconv.Atoi(ConfigV.GetString("devicedefault.portshh"))
			if err != nil {
				d.PortSSH = 22
			}

			if err == nil || p < 655535 || p > 0 {
				d.PortSSH = uint16(p)
			} else {
				d.PortSSH = 22
			}

		}
		d.Dirbackup = fmt.Sprintf("%s/%s", ConfigV.GetString("path.backup"), d.Name)
		Dev.Devices = append(Dev.Devices, d)
	}
}

func RotateDevice(Dev *DeviceList) {
	for i, d := range Dev.Devices {
		if _, err := os.Stat(d.Dirbackup); os.IsNotExist(err) {
			_ = os.Mkdir(d.Dirbackup, os.ModePerm)
			log.Printf("Create Folder %s for backup ", d.Dirbackup)
			continue
		}
		files, err := ioutil.ReadDir(d.Dirbackup)
		if err != nil {
			log.Printf("Error read folder backup %s, Error:%s", d.Dirbackup, err.Error())
			Dev.Devices = RemoveIndexFromDevice(Dev.Devices, i)
			continue
		}

		for _, f := range files {
			now := time.Now()
			fdifftimesecond := now.Sub(f.ModTime()).Seconds()
			diffday := fdifftimesecond / 60 / 24
			if int(diffday) > d.Rotate {
				if len(files) > 5 {
					err := os.Remove(f.Name())
					if err != nil {
						log.Printf("Error Remove file %s, Error:%s", f.Name(), err.Error())
						continue
					}
				}

			}
			if int(fdifftimesecond) < d.Every {
				Dev.Devices = RemoveIndexFromDevice(Dev.Devices, i)
				break
			}

		}

	}

}
func RemoveIndexFromDevice(s []Device, index int) []Device {
	if len(s) == 1 {
		return []Device{}
	}
	return append(s[:index], s[index+1:]...)
}

func RunBackups(Dev *DeviceList) {
	log.Printf("Backup Start ---->")
	for _, d := range Dev.Devices {
		backup(d)
	}

	log.Printf("Backup Finish <----")
}

func backup(d Device) {
	//var hostKey ssh.PublicKey
	config := &ssh.ClientConfig{
		Config: ssh.Config{},
		User:   d.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(d.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         time.Duration(d.Timeout) * time.Second,
	}
	adr := fmt.Sprintf("%s:%d", d.Address, d.PortSSH)
	client, err := ssh.Dial("tcp", adr, config)
	if err != nil {
		log.Printf("Fatal error Device:%s connect to ssh %s, Error:%s", d.Name, adr, err.Error())
		return
	}
	session, err := client.NewSession()
	if err != nil {
		log.Printf("Fatal error Device:%s session to ssh %s, Error:%s", d.Name, adr, err.Error())
		return
	}
	defer session.Close()

	var b bytes.Buffer
	session.Stdout = &b
	if err := session.Run(d.Command); err != nil {
		log.Printf("Failed Run command:\"%s\", Error:%s", d.Command, err.Error())
	}
	dt := time.Now().Format("20060102T1504")
	backupfile := fmt.Sprintf("%s/%s.rsc", d.Dirbackup, dt)
	fn, err := os.Create(backupfile)
	if err != nil {
		log.Printf("Fatal error Device:%s create file %s, Error:%s", d.Name, backupfile, err.Error())
		return
	}
	_, err = fn.Write(b.Bytes())
	if err != nil {
		log.Printf("Fatal error Device:%s write to file %s, Error:%s", d.Name, backupfile, err.Error())
		return
	}

	_ = fn.Close()
}
