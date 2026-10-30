package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/sirupsen/logrus"
)

func TestSaveBackupFileReplacesShorterContent(t *testing.T) {
	previousClient := gitClient
	gitClient = GitClient{}
	defer func() { gitClient = previousClient }()

	dir := t.TempDir()
	previousRoot := ConfigV.GetString("path.backup")
	ConfigV.Set("path.backup", dir)
	defer ConfigV.Set("path.backup", previousRoot)
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	device := &Device{
		Name:           "router",
		Dirbackup:      filepath.Join(dir, "router"),
		BackupFileName: "config.rsc",
		Logdevice:      logger,
	}
	for _, content := range []string{"configuration with a long old suffix", "short"} {
		if err := SaveBackupFile(device, []byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(filepath.Join(dir, device.Name, device.BackupFileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "short" {
		t.Fatalf("backup contains stale data: %q", got)
	}
}

func TestGitClientAddChangeRemoveAndPush(t *testing.T) {
	root := t.TempDir()
	remotePath := filepath.Join(root, "remote.git")
	if _, err := git.PlainInit(remotePath, true); err != nil {
		t.Fatal(err)
	}

	seedPath := filepath.Join(root, "seed")
	seed, err := git.PlainInit(seedPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seedPath, "initial.txt"), []byte("initial"), 0600); err != nil {
		t.Fatal(err)
	}
	seedWorktree, err := seed.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seedWorktree.Add("initial.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := seedWorktree.Commit("Initial commit", &git.CommitOptions{Author: &object.Signature{
		Name: "Test", Email: "test@example.com", When: time.Now(),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := seed.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{remotePath}}); err != nil {
		t.Fatal(err)
	}
	if err := seed.Push(&git.PushOptions{}); err != nil {
		t.Fatal(err)
	}

	clonePath := filepath.Join(root, "clone")
	clone, err := git.PlainClone(clonePath, false, &git.CloneOptions{URL: remotePath})
	if err != nil {
		t.Fatal(err)
	}
	client := GitClient{Folder: clonePath, RepoUrl: remotePath, Repository: clone}
	nested := filepath.Join("team", "site", "router")
	if err := os.MkdirAll(filepath.Join(clonePath, nested), 0700); err != nil {
		t.Fatal(err)
	}
	added := filepath.Join(clonePath, nested, "added.txt")
	removed := filepath.Join(clonePath, nested, "removed.txt")
	if err := os.WriteFile(added, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := client.AddFile(added); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(removed, []byte("remove me"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := client.AddFile(removed); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(added, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := client.SetCommit(added); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(removed); err != nil {
		t.Fatal(err)
	}
	if err := client.RemoveFile(removed); err != nil {
		t.Fatal(err)
	}
	if err := client.CheckPush(); err != nil {
		t.Fatal(err)
	}

	verifyPath := filepath.Join(root, "verify")
	if _, err := git.PlainClone(verifyPath, false, &git.CloneOptions{URL: remotePath}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(verifyPath, nested, "added.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "changed" {
		t.Fatalf("remote has %q, want changed", got)
	}
	if _, err := os.Stat(filepath.Join(verifyPath, nested, "removed.txt")); !os.IsNotExist(err) {
		t.Fatalf("removed file is still on remote: %v", err)
	}
}
