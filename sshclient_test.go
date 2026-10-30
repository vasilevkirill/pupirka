package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"testing"

	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

func TestSshClientRunWithKnownHost(t *testing.T) {
	previousEnabled := ConfigV.GetBool("ssh.knownhosts")
	previousKeyDirectory := ConfigV.GetString("path.key")
	ConfigV.Set("ssh.knownhosts", true)
	ConfigV.Set("path.key", t.TempDir())
	defer func() {
		ConfigV.Set("ssh.knownhosts", previousEnabled)
		ConfigV.Set("path.key", previousKeyDirectory)
	}()

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	serverConfig := &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if string(password) != "secret" {
			return nil, fmt.Errorf("wrong password")
		}
		return nil, nil
	}}
	serverConfig.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	serverDone := make(chan error, 2)
	go func() {
		for i := 0; i < 2; i++ {
			conn, err := listener.Accept()
			if err != nil {
				serverDone <- err
				return
			}
			serverDone <- serveBackupSSH(conn, serverConfig)
		}
	}()

	host, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	device := &Device{Address: host, PortSSH: uint16(port), Username: "backup", Password: "secret", Command: "/export", Timeout: 3, Logdevice: logger}
	for i := 0; i < 2; i++ {
		got, err := SshClientRun(device)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "router configuration\n" {
			t.Fatalf("unexpected SSH output: %q", got)
		}
		if err := <-serverDone; err != nil {
			t.Fatal(err)
		}
	}
}

func serveBackupSSH(conn net.Conn, config *ssh.ServerConfig) error {
	defer conn.Close()
	server, channels, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return err
	}
	defer server.Close()
	go ssh.DiscardRequests(requests)
	for newChannel := range channels {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "session required")
			continue
		}
		channel, channelRequests, err := newChannel.Accept()
		if err != nil {
			return err
		}
		for request := range channelRequests {
			if request.Type != "exec" {
				_ = request.Reply(false, nil)
				continue
			}
			if err := request.Reply(true, nil); err != nil {
				return err
			}
			if _, err := io.WriteString(channel, "router configuration\n"); err != nil {
				return err
			}
			status := make([]byte, 4)
			binary.BigEndian.PutUint32(status, 0)
			if _, err := channel.SendRequest("exit-status", false, status); err != nil {
				return err
			}
			return channel.Close()
		}
	}
	return nil
}
