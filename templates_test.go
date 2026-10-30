package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

type templateResponse struct {
	text string
	exit uint32
	hang bool
}

func templateTestServer(t *testing.T, responses map[string]templateResponse) (string, uint16, <-chan string) {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if string(password) != "secret" {
			return nil, fmt.Errorf("invalid password")
		}
		return nil, nil
	}}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	commands := make(chan string, 20)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		server, channels, requests, err := ssh.NewServerConn(conn, config)
		if err != nil {
			return
		}
		defer server.Close()
		go ssh.DiscardRequests(requests)
		for next := range channels {
			channel, channelRequests, err := next.Accept()
			if err != nil {
				return
			}
			for request := range channelRequests {
				if request.Type != "exec" {
					_ = request.Reply(false, nil)
					continue
				}
				var payload struct{ Command string }
				if ssh.Unmarshal(request.Payload, &payload) != nil {
					_ = request.Reply(false, nil)
					break
				}
				commands <- payload.Command
				_ = request.Reply(true, nil)
				response := responses[payload.Command]
				if response.hang {
					<-time.After(2 * time.Second)
					_ = channel.Close()
					break
				}
				_, _ = io.WriteString(channel, response.text)
				status := make([]byte, 4)
				binary.BigEndian.PutUint32(status, response.exit)
				_, _ = channel.SendRequest("exit-status", false, status)
				_ = channel.Close()
				break
			}
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
	return host, uint16(port), commands
}

func testTemplateDevice(host string, port uint16, name string) *Device {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return &Device{Name: "router", Address: host, PortSSH: port, Username: "backup", Password: "secret", Timeout: 1, TemplateName: name, Logdevice: logger}
}

func setTemplateTestPath(t *testing.T, path string) {
	t.Helper()
	old := ConfigV.GetString("path.templates")
	ConfigV.Set("path.templates", path)
	t.Cleanup(func() { ConfigV.Set("path.templates", old) })
}

func TestTemplateExecAndLocalOverride(t *testing.T) {
	dir := t.TempDir()
	setTemplateTestPath(t, dir)
	source := `{{- $a := exec "first" -}}{{- $b := exec "second" -}}{{- comment "# " (keepLines "^keep" $a) -}}{{- $b | regexReplace "(?m)^drop.*\\n?" "" | stripANSI | trimLineRight -}}`
	if err := os.WriteFile(filepath.Join(dir, "routeros.gotmpl"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	host, port, commands := templateTestServer(t, map[string]templateResponse{
		"first":  {text: "keep one\nignore\n"},
		"second": {text: "drop me\n\x1b[31mresult\x1b[0m  \n"},
	})
	device := testTemplateDevice(host, port, "routeros")
	got, err := SshClientRunTemplate(device)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# keep one\nresult\n" {
		t.Fatalf("unexpected template result %q", got)
	}
	if order := []string{<-commands, <-commands}; !reflect.DeepEqual(order, []string{"first", "second"}) {
		t.Fatalf("command order: %v", order)
	}
}

func TestRouterOSTemplate(t *testing.T) {
	for _, tc := range []struct {
		name, version, export string
		vars                  map[string]any
	}{
		{"v7", "7.18", "/export show-sensitive", nil},
		{"v6", "6.49", "/export", nil},
		{"hidden", "7.18", "/export hide-sensitive", map[string]any{"remove_secret": true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setTemplateTestPath(t, t.TempDir())
			host, port, commands := templateTestServer(t, map[string]templateResponse{
				"/system resource print":               {text: "version: " + tc.version + "   \nboard-name: test\n"},
				"/system package update print":         {text: "installed-version: " + tc.version + "\n"},
				"/system history print without-paging": {text: "changed by admin\n"},
				tc.export:                              {text: "# 2026-10-27 12:00:00 by RouterOS\n/interface add name=ether1  \n"},
			})
			device := testTemplateDevice(host, port, "routeros")
			device.Vars = tc.vars
			got, err := SshClientRunTemplate(device)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(got), "# board-name: test\n") || !strings.Contains(string(got), "/interface add name=ether1\n") || strings.Contains(string(got), "2026-10-27") {
				t.Fatalf("unexpected RouterOS output: %q", got)
			}
			order := []string{<-commands, <-commands, <-commands, <-commands}
			if order[3] != tc.export {
				t.Fatalf("export command: %v", order)
			}
		})
	}
}

func TestTemplateFailureDoesNotWritePartialBackup(t *testing.T) {
	root := t.TempDir()
	setTemplateTestPath(t, t.TempDir())
	oldRoot := ConfigV.GetString("path.backup")
	ConfigV.Set("path.backup", root)
	t.Cleanup(func() { ConfigV.Set("path.backup", oldRoot) })
	if err := os.WriteFile(filepath.Join(ConfigV.GetString("path.templates"), "bad.gotmpl"), []byte(`partial{{exec "fail"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	host, port, _ := templateTestServer(t, map[string]templateResponse{"fail": {exit: 1}})
	device := testTemplateDevice(host, port, "bad")
	device.BackupFileName = "backup.rsc"
	backup(device)
	if device.StatusJob != "error" {
		t.Fatalf("status = %q", device.StatusJob)
	}
	if _, err := os.Stat(filepath.Join(root, "router", "backup.rsc")); !os.IsNotExist(err) {
		t.Fatalf("partial backup exists: %v", err)
	}
}

func TestTemplateCommandTimeout(t *testing.T) {
	setTemplateTestPath(t, t.TempDir())
	if err := os.WriteFile(filepath.Join(ConfigV.GetString("path.templates"), "slow.gotmpl"), []byte(`{{exec "hang"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	host, port, _ := templateTestServer(t, map[string]templateResponse{"hang": {hang: true}})
	device := testTemplateDevice(host, port, "slow")
	start := time.Now()
	_, err := SshClientRunTemplate(device)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout, got %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("command timeout took too long")
	}
}

func TestTemplateDefaultsAndSafeLoading(t *testing.T) {
	dir := t.TempDir()
	setTemplateTestPath(t, dir)
	oldDefault := ConfigV.GetString("devicedefault.template")
	ConfigV.Set("devicedefault.template", "routeros")
	t.Cleanup(func() { ConfigV.Set("devicedefault.template", oldDefault) })
	inherited := testBackupDevice("inherited", nil)
	if err := SetDefaultParameter(inherited); err != nil || inherited.TemplateName != "routeros" {
		t.Fatalf("inherited template: %q, %v", inherited.TemplateName, err)
	}
	empty := testBackupDevice("legacy", nil)
	empty.Template = stringPointer("")
	if err := SetDefaultParameter(empty); err != nil || empty.TemplateName != "" {
		t.Fatalf("explicit empty template: %q, %v", empty.TemplateName, err)
	}
	for _, name := range []string{"../etc", "/etc", "nested/model", "..", "routeros.gotmpl", "CON"} {
		if err := validateTemplateName(name); err == nil {
			t.Errorf("accepted template name %q", name)
		}
	}
	invalid := testBackupDevice("badtemplate", nil)
	invalid.Template = stringPointer("../outside")
	if err := SetDefaultParameter(invalid); err == nil {
		t.Fatal("accepted invalid device template")
	}
	outside := filepath.Join(t.TempDir(), "outside.gotmpl")
	if err := os.WriteFile(outside, []byte("outside marker"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "routeros.gotmpl")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := loadBackupTemplate("routeros"); err == nil {
		t.Fatal("loaded symlink instead of rejecting it")
	}
	if contents, err := os.ReadFile(outside); err != nil || string(contents) != "outside marker" {
		t.Fatalf("outside marker changed: %q, %v", contents, err)
	}
}

func TestEmptyTemplateUsesLegacyCommand(t *testing.T) {
	host, port, commands := templateTestServer(t, map[string]templateResponse{
		"/export": {text: "legacy configuration\n"},
	})
	device := testTemplateDevice(host, port, "")
	device.Command = "/export"
	output, err := runDeviceSSH(device)
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "legacy configuration\n" || <-commands != "/export" {
		t.Fatalf("legacy command output: %q", output)
	}
}

func TestInvalidTemplateFailsBeforeConnecting(t *testing.T) {
	dir := t.TempDir()
	setTemplateTestPath(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "broken.gotmpl"), []byte(`{{exec`), 0600); err != nil {
		t.Fatal(err)
	}
	device := testTemplateDevice("127.0.0.1", 1, "broken")
	if _, err := SshClientRunTemplate(device); err == nil || !strings.Contains(err.Error(), "parse template") {
		t.Fatalf("expected parse error, got %v", err)
	}
}
