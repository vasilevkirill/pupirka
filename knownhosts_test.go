package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"golang.org/x/crypto/ssh"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testHostKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return signer.PublicKey()
}

func TestCheckOrAddHostKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys", "known_hosts")
	hostname := "[router.example]:2222"
	remote := &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 2222}
	firstKey := testHostKey(t)

	if err := checkOrAddHostKey(path, hostname, remote, firstKey); err != nil {
		t.Fatalf("save new host key: %v", err)
	}
	if err := checkOrAddHostKey(path, hostname, remote, firstKey); err != nil {
		t.Fatalf("verify saved host key: %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(contents), "\n"); got != 1 {
		t.Fatalf("expected one saved host key, got %d", got)
	}

	if err := checkOrAddHostKey(path, hostname, remote, testHostKey(t)); err == nil {
		t.Fatal("changed host key was accepted")
	}
}

func TestForwardedHostKeepsOriginalIdentity(t *testing.T) {
	keyDirectory := t.TempDir()
	previousEnabled := ConfigV.GetBool("ssh.knownhosts")
	previousKeyDirectory := ConfigV.GetString("path.key")
	ConfigV.Set("ssh.knownhosts", true)
	ConfigV.Set("path.key", keyDirectory)
	defer func() {
		ConfigV.Set("ssh.knownhosts", previousEnabled)
		ConfigV.Set("path.key", previousKeyDirectory)
	}()

	firstKey := testHostKey(t)
	remote := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 40000}
	if err := SshHostKeyCallback("child.example:22")("127.0.0.1:40000", remote, firstKey); err != nil {
		t.Fatalf("save forwarded host key: %v", err)
	}
	remote.Port = 40001
	if err := SshHostKeyCallback("child.example:22")("127.0.0.1:40001", remote, firstKey); err != nil {
		t.Fatalf("verify forwarded host through a new local port: %v", err)
	}
	if err := SshHostKeyCallback("child.example:22")("127.0.0.1:40001", remote, testHostKey(t)); err == nil {
		t.Fatal("changed forwarded host key was accepted")
	}
	contents, err := os.ReadFile(filepath.Join(keyDirectory, "known_hosts"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(contents), "child.example ") {
		t.Fatalf("host key was saved under the tunnel address: %q", contents)
	}
}

func TestDisabledKnownHostsAcceptsUnrecordedKey(t *testing.T) {
	previousEnabled := ConfigV.GetBool("ssh.knownhosts")
	ConfigV.Set("ssh.knownhosts", false)
	defer ConfigV.Set("ssh.knownhosts", previousEnabled)

	remote := &net.TCPAddr{IP: net.ParseIP("192.0.2.2"), Port: 22}
	if err := SshHostKeyCallback("")("unrecorded.example:22", remote, testHostKey(t)); err != nil {
		t.Fatalf("disabled host key checking rejected a key: %v", err)
	}
}
