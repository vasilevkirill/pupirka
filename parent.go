package main

import (
	"errors"
	"fmt"
	"golang.org/x/crypto/ssh"
	"io"
	"math/rand"
	"net"
	"time"
)

var MDeviceList = make(map[string]Device)
var MLocalPort = make(map[uint16]string)

// SshForwardNewDevice starts a local tunnel through parent and returns a copy
// of child configured to dial that tunnel. Its original address is retained
// for SSH host-key verification.
func SshForwardNewDevice(parent Device, child Device) Device {
	child.LogDebug("SshForwardNewDevice: Get new address", child.Name)
	lport := SshLocalGeneratePort()
	go SshClientRunForward(&parent, child, lport)
	child.KnownHostAddress = SshAddressFormat(&child)
	child.Address = "localhost"
	child.PortSSH = lport
	child.LogDebug(fmt.Sprintf("SshForwardNewDevice: new address (%s), new port (%d)", child.Address, child.PortSSH))
	return child

}

// SshClientRunForward accepts local connections and forwards each one through
// the parent device to the child's original address.
func SshClientRunForward(parent *Device, child Device, lport uint16) {
	auth, _ := SshClientDeviceAuth(parent)

	config := &ssh.ClientConfig{
		Config: ssh.Config{
			KeyExchanges: sshkkeysAlgo,
			Ciphers:      sshChippers,
		},
		User:            parent.Username,
		Auth:            auth,
		HostKeyCallback: SshHostKeyCallback(""),
		Timeout:         time.Duration(parent.Timeout) * time.Second,
	}

	localAddrString := fmt.Sprintf("localhost:%d", lport)
	localListener, err := net.Listen("tcp", localAddrString)
	if err != nil {
		child.LogError(fmt.Sprintf("SshClientRunForward net.Listen failed: %s", err.Error()))
		return
	}

	for {
		// Each incoming connection gets its own SSH connection to the parent.
		localConn, err := localListener.Accept()
		if err != nil {
			child.LogError(fmt.Sprintf("SshClientRunForward listen.Accept failed: %s", err.Error()))
			return
		}
		go forward(localConn, config, SshAddressFormat(parent), SshAddressFormat(&child))
	}
}

// forward copies traffic in both directions between a local connection and a
// connection opened through the parent device.
func forward(localConn net.Conn, config *ssh.ClientConfig, parentaddress string, childaddress string) {
	sshClientConn, err := ssh.Dial("tcp", parentaddress, config)
	if err != nil {
		LogConsole.Warnf("ssh.Dial failed %s ... Child %s, Parent %s", err, childaddress, parentaddress)
		_ = localConn.Close()
		return
	}
	// Dial the child from inside the parent SSH connection.
	sshConn, err := sshClientConn.Dial("tcp", childaddress)
	if err != nil {
		LogConsole.Warnf("SSH forwarding failed %s ... Child %s, Parent %s", err, childaddress, parentaddress)
		_ = localConn.Close()
		_ = sshClientConn.Close()
		return
	}

	// Forward requests to the child.
	go func() {
		_, copyErr := io.Copy(sshConn, localConn)
		if copyErr != nil {
			LogConsole.Info(fmt.Sprintf("io.Copy failed %s ... Child %s, Parent %s", copyErr.Error(), childaddress, parentaddress))
			_ = sshConn.Close()
			_ = sshClientConn.Close()
			return
		}
	}()

	// Forward responses to the local client.
	go func() {
		_, copyErr := io.Copy(localConn, sshConn)
		if copyErr != nil {
			LogConsole.Info(fmt.Sprintf("io.Copy failed %s ... Child %s, Parent %s", copyErr.Error(), childaddress, parentaddress))
			_ = sshConn.Close()
			_ = sshClientConn.Close()
			return
		}
	}()

}

// SshLocalGeneratePort reserves a candidate port for a local forwarding
// listener. The listener still reports bind failures separately.
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

// SshNeedForward resolves the parent named by a child device.
func SshNeedForward(device *Device) (Device, Device, error) {

	if _, ok := MDeviceList[device.Parent]; !ok {
		return Device{}, Device{}, errors.New("SshNeedForward: no isset parent device")
	}
	parent := MDeviceList[device.Parent]
	dev := *device
	return parent, dev, nil
}
