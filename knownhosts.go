package main

import (
	"fmt"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"net"
	"os"
	"path/filepath"
	"sync"
)

var knownHostsMu sync.Mutex

// SshHostKeyCallback trusts a host on its first connection and checks its
// recorded key on every later connection when ssh.knownhosts is enabled.
// hostAddress identifies a device reached through a local SSH tunnel.
func SshHostKeyCallback(hostAddress string) ssh.HostKeyCallback {
	if !ConfigV.GetBool("ssh.knownhosts") {
		return ssh.InsecureIgnoreHostKey()
	}

	path := filepath.Join(ConfigV.GetString("path.key"), "known_hosts")
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		if hostAddress != "" {
			hostname = hostAddress
		}
		knownHostsMu.Lock()
		defer knownHostsMu.Unlock()

		err := checkOrAddHostKey(path, hostname, remote, key)
		if err != nil {
			LogConsole.Warnf("SSH host key verification failed for %s: %v", hostname, err)
		}
		return err
	}
}

// checkOrAddHostKey records an unseen key and rejects a changed key. The caller
// holds knownHostsMu so concurrent SSH connections cannot race on the file.
func checkOrAddHostKey(path, hostname string, remote net.Addr, key ssh.PublicKey) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create known_hosts directory: %w", err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open known_hosts: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close known_hosts: %w", err)
	}

	check, err := knownhosts.New(path)
	if err != nil {
		return fmt.Errorf("read known_hosts: %w", err)
	}
	if err := check(hostname, remote, key); err == nil {
		return nil
	} else if keyErr, ok := err.(*knownhosts.KeyError); !ok || len(keyErr.Want) != 0 {
		return err
	}

	file, err = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open known_hosts for update: %w", err)
	}
	_, writeErr := fmt.Fprintln(file, knownhosts.Line([]string{hostname}, key))
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("save SSH host key: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close known_hosts after update: %w", closeErr)
	}
	return nil
}
