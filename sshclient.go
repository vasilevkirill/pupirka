package main

import (
	"bytes"
	"errors"
	"fmt"
	"golang.org/x/crypto/ssh"
	"io/ioutil"
	"time"
)

var sshkkeysAlgo = []string{
	"diffie-hellman-group1-sha1",
	"diffie-hellman-group14-sha1",
	"ecdh-sha2-nistp256",
	"ecdh-sha2-nistp384",
	"ecdh-sha2-nistp521",
	"diffie-hellman-group-exchange-sha1",
	"diffie-hellman-group-exchange-sha256",
	"curve25519-sha256@libssh.org",
}

//connect and run command, return []byte
func SshClientRun(device *Device) ([]byte, error) {

	auth, err := SshClientDeviceAuth(device)
	if err != nil {
		return []byte{}, err
	}
	device.LogDebug(fmt.Sprintf("SshClientRun: Get auth (%t)", auth))

	config := &ssh.ClientConfig{
		Config: ssh.Config{
			KeyExchanges: sshkkeysAlgo,
		},
		User:            device.Username,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         time.Duration(device.Timeout) * time.Second,
	}
	address := SshAddressFormat(device)
	client, err := ssh.Dial("tcp", address, config)
	if err != nil {
		return nil, errors.New("SshClientRun: DialSSH error:" + err.Error())
	}
	defer client.Close()
	device.LogDebug(fmt.Sprintf("SshClientRun: ssh Dial running %s", device.Name))
	session, err := client.NewSession()
	if err != nil {
		return nil, errors.New("SshClientRun: NewSession error:" + err.Error())
	}
	defer session.Close()
	device.LogDebug(fmt.Sprintf("SshClientRun: ssh session running %s", device.Name))
	var b bytes.Buffer
	session.Stdout = &b
	if err := session.Run(device.Command); err != nil {
		return nil, errors.New("SshClientRun: Run command error:" + err.Error())
	}
	device.LogDebug(fmt.Sprintf("SshClientRun: Read bytes good %s", device.Name))
	return b.Bytes(), nil
}

//prepare auth method for client
func SshClientDeviceAuth(device *Device) ([]ssh.AuthMethod, error) {
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

func SshAddressFormat(device *Device) string {
	str := fmt.Sprintf("%s:%d", device.Address, device.PortSSH)
	device.LogDebug(fmt.Sprintf("SshAddressFormat: addres (%s)", str))
	return str
}
