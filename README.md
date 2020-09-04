# Pupirka

(Simple Backup over SSH)
---

Author @vasilevkirill [My Web Site](https://mikrotik.me/vasilevkirill.html)

Pupirka is a Application for simple execute command in ssh server and response save in file, supported SSH LocalPort Forward.



 - Pupirka - this not service
 - Pupirka - this not used external library
 - Pupirka - running one binary and save backup from all MikroTik or other devices
 - Pupirka - Supported all Operating System if supported Golang

## Contents

- [Pupirka](#pupirka)
    - [Contents](#Contents)
    - [Installation](#instalation)
        - [Download](#download)
        - [Build From source](#build-from-source-code)
        - [First setup](#first-setup)
    - [Quick Start](#quick-start)
    - [Device Settings](#device-all-parameter)
    - [Config Pupirka](#puprik-config)
      - [Device Default Parameter](#devicedefault)
      - [Logging](#log)
      - [Path](#path)
      - [Proccess](#proccess)
    - [Time Format](#time-format)
---

## Installation
### Download

For download your need used this [link](https://github.com/vasilevkirill/pupirka/releases)
Save binary for your Operating System

### Build from source code
1. Get source

You need use git and clone repository in your system

```bash
git clone https://github.com/vasilevkirill/pupirka.git
```
2. Compily from source

Your need installed go and running command
```bash
cd pupirka
go get -d ./...
go build -ldflags "-s" -o pupirka .
```

your binary set name pupirka if your used nix*like set execute permission `chmod +x pupirka`


## First setup

If your first run pupirka, needed running once

```bash
./pupirka
```

Pupirka automated Created required directories and config file `pupirka.config.json`.

Needed added `json` file in folder `device`. File name is name device. (Example `mydevice-01.json` Pupirka used file name as device name `mydevice-01`)

```json
{
  "address": "111.222.223.222",
  "username": "backupuserssh",
  "password": "asdlmhas,o9duajsp9odj"
}
```

Paprika connected to SSH server in address and auth used credential from `json` file.

Running command `/export` (Default) and saved response in file `./backup/mydevice-01/20200903T2210.rsc` if you need automated backup used cron in unix or scheduler in Windows, running `pupirka`.

## Device all parameter

```json
{
  "address": "111.222.223.222",
  "username": "backupuserssh",
  "password": "asdlmhas,o9duajsp9odj",
  "portssh": 22,
  "timeout": 10,
  "every": 3600,
  "rotate": 730,
  "parent": "mydevice-02",
  "timeformat": "20060102T1504",
  "prefix": "rootR1-",
  "filenameformat": "%p%t.rcs",
  "key":"blabla.key",
  "clearstring":""
}
```

All supported parameter in `json` file for device

 - `portssh` - if your device running or access not 22 (default) port ssh, set needed tcp port.
 - `timeout` - if your device very slow response, set needed time wait in second.
 - `every` - how often to backup, check the time of the last file and if the difference with the current time is greater than the time of the file, a backup will be created.
 - `rotate` - delete old files backups older than days. (in order not to accidentally delete all backups, the deletion will occur if ONLY the count of backup files is more than (5) five files.)
 - `command` - execute command on remote device, if you need backup only firewall in mikrotik (`/ip firewall export`)
 - `parent` - if your devices are behind the device and are not directly accessible, you can use this parameter. Specify the name of the device through which you want to establish a connection.
 - `timeformat` - as generate time string in file backup. ([Read documentation and example](#time-format))
 - `prefix` - adding prefix in file name backups.
 - `filename` - position `prefix` and `time` in file name. `%p` replaced `prefix` and `%t` replaced current time.
 - `key` - Private Key for used SSH Authorized, need saved private key in folder `./keys`. (**if need used SSH keys, need `password` filed delete or set `""`**)
 - `clearstring` - Specify the character to start with the line to delete.
Many defaults can be changed globally in the config file `pupirka.config.json`.

## Config Pupirka

If pupirka execute and file `pupirka.config.json` not found, pupirka created config and set all default parameter

format: json

```json
{
  "devicedefault": {
    "clearstring":"",
    "command": "/export",
    "every": 3600,
    "filenameformat": "%p%t.rcs",
    "key": "",
    "portshh": 22,
    "prefix": "",
    "rotate": 730,
    "timeformat": "20060102T1504",
    "timeout": 10
  },
  "log": {
    "format": "text",
    "maxday": 1
  },
  "path": {
    "backup": "./backup",
    "devices": "./device",
    "key": "./keys",
    "log": "./log"
  },
  "process": {
    "max": 10
  }
}
```

Sub Categories:
  - `devicedefault` - default parameter used if not set in devices json file config
  - `log` - global parameter for logging
  - `path` - directories
  - `process` - limits for pupirka

### devicedefault
  When Pupirka reads device parameters, if in devices json files not set parameter Pupirka used default value from config. Even if you did not set in the global config, these parameters and values are stored in the code Pupirka.

### log
  - `format` - the logging format can be `json` or `text`.
  - `maxday` - how many days can be recorded in one file.

### Path

Pupirka created all folder if not found.

 - `backup` - folder for save backup
 - `devices` - folder used for find your devices. Your needed create json file devices in this folder.
 - `key` -  Pupirka find in this folder Private Keys for Auth devices.
 - `log` -  Folder for save all log.

### Process
  - `max` - in order not to overload the CPU, Pupirka it will back up in groups of the specified count.
## Time Format

Used specified Golang format

 - Year - `2006` = result `2020`, `06` = result `20`
 - Mount - `01` = result `05`, `1` = result `5`
 - Day - `02` = result `07`, `2` = result `7`
 - Hours - `03` = result `05`, `3` = result `5`, `15` = result `17`
 - Minutes - `04` = result `09`, `4` = result `9`
 - Seconds - `05` = result `07`, `5` = result `7`

Examples: datetime 2020-09-04:22:15:01

```json
`timefomat` = "20060102T1504" #result 20200904T2215 this default
 ```

```json
`timefomat` = "2006y01m02T15h04m" #result 2020y09m04T22h15m
 ```

```json
`timefomat` = "2006-01-02 150405" #result 2020-09-04 221501
```

```json
`timefomat` = "2006-01-2 150405" #result 2020-09-4 221501
```

```json
`timefomat` = "2006-01-2 30405" #result 2020-09-4 101501
```
Please read documentation golang for time format loyuot


---
RUS - https://mikrotik.me/blog-pupirka-ru.html


Eng - https://mikrotik.me/blog-pupirka-en.html
