# Pupirka

(Simple Backup over SSH)
---

Author @vasilevkirill [My Web Site](https://vasilevkirill.ru)

Pupirka is a Application for simple execute command in ssh server and response save in file, supported SSH LocalPort Forward.

 - Pupirka can run once or stay active as a service with `-daemon`
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
      - [Backup templates](#backup-templates)
      - [Device Hook](#device-hook)
    - [Config Pupirka](#puprik-config)
      - [Device Default Parameter](#devicedefault)
      - [Logging](#log)
      - [Path](#path)
      - [SSH host keys](#ssh-host-keys)
      - [Proccess](#proccess)
      - [Global Hooks](#global-hooks)
    - [Time Format](#time-format)
    - [Code structure](#code-structure)
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
go mod download
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

Running command `/export` (Default) saves the response in `./backup/mydevice-01/20200903T2210.rsc`.

By default, `./pupirka` makes one pass and exits; use cron or a system scheduler to repeat it. Run `./pupirka -daemon` to keep the process active. It backs up each device according to its `every` interval (in seconds), checks device JSON files again between passes, and stops cleanly on SIGINT or SIGTERM. Run it under your system service manager if you want it to start automatically after a reboot. The process checks for configuration changes at least every 30 seconds; restart it after changing `pupirka.config.json`.

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
  "project": "team/site",
  "template": "routeros",
  "vars": {"remove_secret": false},
  "parent": "mydevice-02",
  "timeformat": "20060102T1504",
  "prefix": "rootR1-",
  "filenameformat": "%p%t.rcs",
  "key":"blabla.key",
  "clearstring":"",
  "hook": {
      "backup": "",
      "error": "",
      "skip": ""
    }
}
```

All supported parameter in `json` file for device

 - `portssh` - if your device running or access not 22 (default) port ssh, set needed tcp port.
 - `timeout` - if your device very slow response, set needed time wait in second.
 - `every` - how often to backup, check the time of the last file and if the difference with the current time is greater than the time of the file, a backup will be created.
 - `rotate` - delete old files backups older than days. (in order not to accidentally delete all backups, the deletion will occur if ONLY the count of backup files is more than (5) five files.)
 - `project` - optional folder path under `path.backup`, using `/` between names. For example, `team/site` stores backups in `backup/team/site/<device>/`. Absolute paths, `.` and `..` are rejected. An explicit empty string disables `devicedefault.project` for this device. Existing backups are not moved. `path.backup` remains the trusted root and may be configured separately.
 - `command` - execute command on remote device, if you need backup only firewall in mikrotik (`/ip firewall export`)
 - `template` - use a Go backup template instead of `command`; an explicit empty value selects the legacy command even when `devicedefault.template` is set.
 - `vars` - optional values for the selected template, such as `remove_secret` for RouterOS.
 - `parent` - if your devices are behind the device and are not directly accessible, you can use this parameter. Specify the name of the device through which you want to establish a connection.
 - `timeformat` - as generate time string in file backup. ([Read documentation and example](#time-format))
 - `prefix` - adding prefix in file name backups.
 - `filename` - position `prefix` and `time` in file name. `%p` replaced `prefix` and `%t` replaced current time, %n replaced device name
 - `key` - Private Key for used SSH Authorized, need saved private key in folder `./keys`. (**if need used SSH keys, need `password` filed delete or set `""`**)
 - `clearstring` - Specify the character to start with the line to delete.
 - `hook` - Execute command in os.

Many defaults can be changed globally in the config file `pupirka.config.json`.

### Backup templates

Set `"template": "routeros"` in device JSON to use the built-in RouterOS model. It collects resource, package and history information, then selects `/export`, `/export show-sensitive` for RouterOS 7+, or `/export hide-sensitive` when `"vars": {"remove_secret": true}`. The model is adapted from [Oxidized's RouterOS model](https://github.com/ytti/oxidized/blob/master/lib/oxidized/model/routeros.rb); its attribution and Apache-2.0 license are in `models/`.

Put custom Go `text/template` files in `path.templates` (default `./templates`) and refer to them by name without the `.gotmpl` extension. Local files override a built-in model of the same name. For example, `templates/example.gotmpl` runs two commands. The template's rendered text becomes one backup file, then the usual `clearstring`, rotation, Git and hook steps apply. An empty result or any template or command error prevents the file from being written.

Templates receive `.Name`, `.Address`, `.Project` and `.Vars`; passwords and private keys are not exposed. Available functions are `exec` (SSH command), `regexFind` (first capture group, or full match), `regexReplace`, `keepLines`, `dropLines`, `stripANSI`, `trimLineRight` and `comment`. For example:

```gotemplate
{{- $version := exec "show version" | trimLineRight -}}
{{- comment "# " $version -}}
{{- exec "show running-config" | stripANSI | trimLineRight -}}
```

`exec` uses one SSH connection and a new session for each command, in template order. Each command must finish within `timeout` seconds. This first version supports SSH exec commands; Oxidized models requiring an interactive shell, Telnet or an API need separate implementation stages.

### Devece Hook

A device can have three (3) states.

 - `skip` if no need backup, if time last backup less than `every` (timefilelastbackup < every)
 - `error` if when doing backup device return error (connected ssh, connected parent, execute command, create file and etc...)
 - `backup` Good job, backup successfully

this string can be used next arguments
  - `%name` - name device (from filename).
  - `%parent`- Parent if connected via ssh ssh port forward.
  - `%filename` - filename backup
  - `%address` - Address Device
  - `%portssh` - Port ssh Device

Device hooks start without waiting for them to finish.

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
    "template": "",
    "portshh": 22,
    "prefix": "",
    "project": "",
    "rotate": 730,
    "timeformat": "20060102T1504",
    "timeout": 10,
    "hook": {
        "backup": "",
        "error": "",
        "skip": ""
      }
  },
  "git": {
      "branch": "master",
      "password": "",
      "user":"Pupirka",
      "email": "vk@mikrotik.me",
      "remote": ""
   },
  "log": {
    "format": "text",
    "maxday": 1
  },
  "path": {
    "backup": "./backup",
    "devices": "./device",
    "key": "./keys",
    "log": "./log",
    "templates": "./templates"
  },
  "ssh": {
    "knownhosts": false
  },
  "process": {
    "max": 10
  },
  "global": {
    "hook": {
      "post": "",
      "pre": ""
    },
    "onlyone": false
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

### git
 Clone current remote repo, Commit and Push remote

  - `branch` - need isset
  - `remote` - url to remote repo

### log
  - `format` - the logging format can be `json` or `text`.
  - `maxday` - how many days can be recorded in one file.

### Path

Pupirka created all folder if not found.

 - `backup` - folder for save backup
 - `devices` - folder used for find your devices. Your needed create json file devices in this folder.
 - `key` -  Pupirka find in this folder Private Keys for Auth devices.
 - `log` -  Folder for save all log.
 - `templates` - folder for custom `.gotmpl` backup templates.

### SSH host keys

Set `ssh.knownhosts` to `true` to save new SSH host keys in `path.key/known_hosts` and verify them on later connections. A changed or revoked key produces a warning and the connection fails. The setting also applies when connecting through a parent device. With `false` (the default), host keys are ignored as before.

### Process
  - `max` - in order not to overload the CPU, Pupirka it will back up in groups of the specified count.

### Global
 other parameter
 - `onlyone` true or false. Running only one application pupirka

#### Global Hooks
   Running and wait execute custom command.
   - `pre` running before backups
   - `post` running after all backups    

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

## Code structure

`main.go` loads configuration and runs either one backup pass or the daemon loop. Each pass scans device JSON files, applies defaults, rotates old backups, and runs due jobs in a worker pool (`pupirka.go`). Devices with invalid settings are logged and skipped.

`sshclient.go` runs the legacy single command. `templates.go` loads a local or embedded Go template, opens one SSH connection, and executes each template command in a separate session. `parent.go` handles SSH forwarding through a parent device. The resulting bytes reach `SaveBackupFile` only after collection succeeds.

`backuppath.go` validates project, device, and filename components. File operations use `os.Root` under the configured `path.backup`; rotation considers only regular backup files for that device. `git.go` records added, changed, and removed files when a remote repository is configured. Local SSH and filesystem tests exercise these paths without network devices.


---
RUS - https://vasilevkirill.ru/articles/pupirka/
