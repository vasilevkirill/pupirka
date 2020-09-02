package main

import (
	"bytes"
	"errors"
	"fmt"
	"golang.org/x/crypto/ssh"
	"io/ioutil"
	"time"
)

//connect and run command, return []byte
func SshClientRun(device Device) ([]byte, error) {

	auth, err := SshClientDeviceAuth(device)
	if err != nil {
		return []byte{}, err
	}
	config := &ssh.ClientConfig{
		Config:          ssh.Config{},
		User:            device.Username,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         time.Duration(device.Timeout) * time.Second,
	}
	address := fmt.Sprintf("%s:%d", device.Address, device.PortSSH)
	client, err := ssh.Dial("tcp", address, config)
	if err != nil {
		return nil, errors.New("SshClientRun: DialSSH error:" + err.Error())
	}
	session, err := client.NewSession()
	if err != nil {
		return nil, errors.New("SshClientRun: NewSession error:" + err.Error())
	}
	defer session.Close()
	var b bytes.Buffer
	session.Stdout = &b
	if err := session.Run(device.Command); err != nil {
		return nil, errors.New("SshClientRun: Run command error:" + err.Error())
	}

	return b.Bytes(), nil
}

//prepare auth method for client
func SshClientDeviceAuth(device Device) ([]ssh.AuthMethod, error) {
	var auth []ssh.AuthMethod
	if device.Authkey == false {
		if device.Password == "" {
			return nil, errors.New("SshClientDeviceAuth: Password empty")
		}
		auth = append(auth, ssh.Password(device.Password))
		return auth, nil
	}

	flp := fmt.Sprintf("%s/%s", ConfigV.GetString("path.key"), device.Key)
	key, err := ioutil.ReadFile(flp)
	if err != nil {
		return nil, errors.New("SshClientDeviceAuth: read key file:" + err.Error())
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, errors.New("SshClientDeviceAuth: parse private file:" + err.Error())
	}
	auth = append(auth, ssh.PublicKeys(signer))
	return auth, nil
}
