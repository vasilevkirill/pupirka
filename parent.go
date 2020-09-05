package main

import (
	"errors"
	"fmt"
	"golang.org/x/crypto/ssh"
	"io"
	"log"
	"math/rand"
	"net"
	"time"
)

var MDeviceList = make(map[string]Device)
var MLocalPort = make(map[uint16]string)

func SshForwardNewDevice(parent Device, child Device) Device {
	child.LogDebug("SshForwardNewDevice: Get new address", child.Name)
	lport := SshLocalGeneratePort()
	go SshClientRunForward(parent, child, lport)
	child.Address = "localhost"
	child.PortSSH = lport
	child.LogDebug(fmt.Sprintf("SshForwardNewDevice: new address (%s), new port (%d)", child.Address, child.PortSSH))
	return child

}

func SshClientRunForward(parent Device, child Device, lport uint16) {
	auth, _ := SshClientDeviceAuth(parent)

	config := &ssh.ClientConfig{
		Config:          ssh.Config{},
		User:            parent.Username,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         time.Duration(parent.Timeout) * time.Second,
	}

	localAddrString := fmt.Sprintf("localhost:%d", lport)
	localListener, err := net.Listen("tcp", localAddrString)
	if err != nil {
		log.Fatalf("SshClientRunForward net.Listen failed: %v", err)
	}

	for {
		// Setup localConn (type net.Conn)
		localConn, err := localListener.Accept()
		if err != nil {
			log.Fatalf("SshClientRunForward listen.Accept failed: %v", err)
		}
		go forward(localConn, config, SshAddressFormat(&parent), SshAddressFormat(&child))
	}
}

func forward(localConn net.Conn, config *ssh.ClientConfig, parentaddress string, childaddress string) {
	// Setup sshClientConn (type *ssh.ClientConn)
	sshClientConn, err := ssh.Dial("tcp", parentaddress, config)
	if err != nil {
		LogConsole.Info(fmt.Sprintf("ssh.Dial failed %s ... Child %s, Parent %s", err.Error(), childaddress, parentaddress))
		_ = sshClientConn.Close()
		return
	}
	// Setup sshConn (type net.Conn)
	sshConn, err := sshClientConn.Dial("tcp", childaddress)

	// Copy localConn.Reader to sshConn.Writer
	go func() {
		_, err = io.Copy(sshConn, localConn)
		if err != nil {
			LogConsole.Info(fmt.Sprintf("io.Copy failed %s ... Child %s, Parent %s", err.Error(), childaddress, parentaddress))
			_ = sshConn.Close()
			_ = sshClientConn.Close()
			return
		}
	}()

	// Copy sshConn.Reader to localConn.Writer
	go func() {
		_, err = io.Copy(localConn, sshConn)
		if err != nil {
			LogConsole.Info(fmt.Sprintf("io.Copy failed %s ... Child %s, Parent %s", err.Error(), childaddress, parentaddress))
			_ = sshConn.Close()
			_ = sshClientConn.Close()
			return
		}
	}()

}

func SshLocalGeneratePort() uint16 {
	min := 40000
	max := 50000

	for {
		rand.Seed(time.Now().UnixNano())
		r := rand.Intn(max-min+1) + min
		if _, ok := MLocalPort[uint16(r)]; ok {
			continue
		}
		MLocalPort[uint16(r)] = ""
		return uint16(r)
	}
}

func SshNeedForward(device Device) (Device, Device, error) {

	if _, ok := MDeviceList[device.Parent]; !ok {
		return Device{}, Device{}, errors.New("SshNeedForward: no isset parent device")
	}
	parent := MDeviceList[device.Parent]
	return parent, device, nil
}
