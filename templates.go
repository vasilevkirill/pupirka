package main

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"text/template"
	"time"

	"golang.org/x/crypto/ssh"
)

// builtinTemplates contains models available without a local templates folder.
//
//go:embed models/routeros.gotmpl
var builtinTemplates embed.FS

var templateNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

// validateTemplateName permits a portable single-component model identifier.
// The .gotmpl extension is added only when the template is loaded.
func validateTemplateName(name string) error {
	if !templateNamePattern.MatchString(name) {
		return fmt.Errorf("invalid template name %q", name)
	}
	return validateBackupName(name)
}

// loadBackupTemplate reads a local model first, then falls back to an embedded
// model. Local symlinks and other non-regular files are never accepted.
func loadBackupTemplate(name string) ([]byte, error) {
	if err := validateTemplateName(name); err != nil {
		return nil, err
	}
	filename := name + ".gotmpl"
	root, err := os.OpenRoot(ConfigV.GetString("path.templates"))
	if err == nil {
		defer root.Close()
		info, statErr := root.Lstat(filename)
		if statErr == nil {
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("template %q is not a regular file", filename)
			}
			file, openErr := root.Open(filename)
			if openErr != nil {
				return nil, openErr
			}
			defer file.Close()
			opened, statErr := file.Stat()
			if statErr != nil || !os.SameFile(info, opened) {
				return nil, fmt.Errorf("template %q changed while opening", filename)
			}
			return io.ReadAll(file)
		}
		if !os.IsNotExist(statErr) {
			return nil, statErr
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	data, err := builtinTemplates.ReadFile("models/" + filename)
	if err != nil {
		return nil, fmt.Errorf("template %q not found: %w", name, err)
	}
	return data, nil
}

// runDeviceSSH keeps the legacy single-command path for devices without a model.
func runDeviceSSH(device *Device) ([]byte, error) {
	if device.TemplateName == "" {
		return SshClientRun(device)
	}
	return SshClientRunTemplate(device)
}

// SshClientRunTemplate renders one model into one backup. Parsing happens before
// connecting, and no partial rendered output is returned when a command fails.
func SshClientRunTemplate(device *Device) ([]byte, error) {
	source, err := loadBackupTemplate(device.TemplateName)
	if err != nil {
		return nil, err
	}
	var client *ssh.Client
	functions := template.FuncMap{
		"exec": func(command string) (string, error) {
			if client == nil {
				return "", errors.New("SSH connection is not ready")
			}
			return runTemplateCommand(client, device.Timeout, command)
		},
		"regexFind":     regexFind,
		"regexReplace":  regexReplace,
		"keepLines":     keepLines,
		"dropLines":     dropLines,
		"stripANSI":     func(value string) string { return ansiEscapePattern.ReplaceAllString(value, "") },
		"trimLineRight": trimLineRight,
		"comment":       commentLines,
	}
	tmpl, err := template.New(device.TemplateName).Funcs(functions).Option("missingkey=error").Parse(string(source))
	if err != nil {
		return nil, fmt.Errorf("parse template %q: %w", device.TemplateName, err)
	}
	auth, err := SshClientDeviceAuth(device)
	if err != nil {
		return nil, err
	}
	config := &ssh.ClientConfig{
		Config: ssh.Config{KeyExchanges: sshkkeysAlgo, Ciphers: sshChippers},
		User:   device.Username, Auth: auth,
		HostKeyCallback: SshHostKeyCallback(device.KnownHostAddress),
		Timeout:         time.Duration(device.Timeout) * time.Second,
	}
	client, err = ssh.Dial("tcp", SshAddressFormat(device), config)
	if err != nil {
		return nil, fmt.Errorf("template SSH dial: %w", err)
	}
	defer client.Close()
	// Expose only fields needed for rendering; credentials stay out of templates.
	data := templateDeviceData{
		Name: device.Name, Address: device.Address,
		Project: device.ProjectPath, Vars: device.Vars,
	}
	var output bytes.Buffer
	if err := tmpl.Execute(&output, data); err != nil {
		return nil, fmt.Errorf("execute template %q: %w", device.TemplateName, err)
	}
	if output.Len() == 0 {
		return nil, fmt.Errorf("template %q produced no backup data", device.TemplateName)
	}
	return output.Bytes(), nil
}

// templateDeviceData limits the fields visible to user-authored templates.
type templateDeviceData struct {
	Name    string
	Address string
	Project string
	Vars    map[string]any
}

// runTemplateCommand opens a fresh SSH session on the verified connection.
// A timeout closes that connection so the blocked session can unwind.
func runTemplateCommand(client *ssh.Client, seconds int, command string) (string, error) {
	if strings.TrimSpace(command) == "" {
		return "", errors.New("empty template command")
	}
	if seconds <= 0 {
		return "", errors.New("template command timeout must be positive")
	}
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	type result struct {
		output []byte
		err    error
	}
	done := make(chan result, 1)
	go func() {
		output, err := session.Output(command)
		done <- result{output, err}
	}()
	timer := time.NewTimer(time.Duration(seconds) * time.Second)
	defer timer.Stop()
	select {
	case res := <-done:
		if res.err != nil {
			return "", fmt.Errorf("command %q: %w", command, res.err)
		}
		return string(res.output), nil
	case <-timer.C:
		_ = client.Close()
		return "", fmt.Errorf("command %q timed out after %d seconds", command, seconds)
	}
}

// regexFind returns the first capture group, or the full match if the pattern
// has no groups. A non-match returns an empty string.
func regexFind(pattern, value string) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", err
	}
	match := re.FindStringSubmatch(value)
	if len(match) > 1 {
		return match[1], nil
	}
	if len(match) == 1 {
		return match[0], nil
	}
	return "", nil
}

// regexReplace applies a regular expression to a command's output.
func regexReplace(pattern, replacement, value string) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", err
	}
	return re.ReplaceAllString(value, replacement), nil
}

// filterLines keeps or removes complete lines that match pattern, preserving
// their original line endings.
func filterLines(pattern, value string, keep bool) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", err
	}
	var output strings.Builder
	for _, line := range strings.SplitAfter(value, "\n") {
		if line != "" && re.MatchString(strings.TrimSuffix(line, "\n")) == keep {
			output.WriteString(line)
		}
	}
	return output.String(), nil
}

// keepLines returns only lines matching pattern.
func keepLines(pattern, value string) (string, error) { return filterLines(pattern, value, true) }

// dropLines removes lines matching pattern.
func dropLines(pattern, value string) (string, error) { return filterLines(pattern, value, false) }

// trimLineRight removes trailing spaces and CR characters from every line.
func trimLineRight(value string) string {
	var output strings.Builder
	for _, line := range strings.SplitAfter(value, "\n") {
		if line == "" {
			continue
		}
		endsNewline := strings.HasSuffix(line, "\n")
		line = strings.TrimRight(strings.TrimSuffix(line, "\n"), " \t\r")
		output.WriteString(line)
		if endsNewline {
			output.WriteByte('\n')
		}
	}
	return output.String()
}

// commentLines prefixes each line and terminates a final unterminated line.
func commentLines(prefix, value string) string {
	var output strings.Builder
	for _, line := range strings.SplitAfter(value, "\n") {
		if line == "" {
			continue
		}
		output.WriteString(prefix)
		output.WriteString(line)
		if !strings.HasSuffix(line, "\n") {
			output.WriteByte('\n')
		}
	}
	return output.String()
}
