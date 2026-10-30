package main

import (
	"fmt"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"time"
)

// GitClient tracks the backup repository and whether local commits need push.
type GitClient struct {
	Folder     string
	NeedPush   bool
	RepoUrl    string
	Auth       http.BasicAuth
	Repository *git.Repository
}

var gitClient GitClient

// RunGlobalPreStart opens or clones the configured backup repository once at
// startup. Without git.remote, backup files remain local.
func RunGlobalPreStart() error {

	gitClient.Folder = ConfigV.GetString("path.backup")
	gitClient.RepoUrl = ConfigV.GetString("git.remote")
	if !gitClient.CheckRepo() {
		return nil
	}
	gitClient.Auth = struct{ Username, Password string }{Username: ConfigV.GetString("git.user"), Password: ConfigV.GetString("git.password")}
	rep, err := git.PlainClone(gitClient.Folder, false, &git.CloneOptions{
		URL:           gitClient.RepoUrl,
		Auth:          &gitClient.Auth,
		ReferenceName: plumbing.NewBranchReferenceName(ConfigV.GetString("git.branch")),
		SingleBranch:  true,
	})
	if err == git.ErrRepositoryAlreadyExists {
		rep, err := git.PlainOpen(gitClient.Folder)
		if err != nil {
			return err
		}
		gitClient.Repository = rep
		return nil
	}
	if err != nil {
		return err
	}
	gitClient.Repository = rep

	return nil
}

// AddFile stages a newly written backup and creates a commit for it.
func (c *GitClient) AddFile(filename string) error {
	if c.CheckRepo() == false {
		return nil
	}
	LogConsole.Infof("Adding File %s to repo", filename)
	w, err := c.Repository.Worktree()
	if err != nil {
		LogConsole.Errorf("Error git get WorkTree: %s", err.Error())
		return err
	}
	_, err = w.Status()
	if err != nil {
		LogConsole.Errorf("Error git get WorkTree Status: %s", err.Error())
		return err
	}
	if len(filename) > len(c.Folder) {
		if filename[:len(c.Folder)] == c.Folder {
			filename = filename[len(c.Folder)+1:]
		}
	}
	_, err = w.Add(filename)
	if err != nil {
		LogConsole.Errorf("Error git WorkTree add file: %s %s", err.Error(), filename)
		return err
	}

	commitName := fmt.Sprintf("Added File Automated %s", filename)
	commit, err := w.Commit(commitName, &git.CommitOptions{
		Author: &object.Signature{
			Name:  ConfigV.GetString("git.name"),
			Email: ConfigV.GetString("git.user"),
			When:  time.Now(),
		},
	})
	_, err = c.Repository.CommitObject(commit)
	if err != nil {
		LogConsole.Infof("Error git Commit name:%s", commitName)
		return err
	}
	if c.RepoUrl != "" {
		c.NeedPush = true
	}
	return nil
}

// RemoveFile stages the deletion of a rotated backup and commits it.
func (c *GitClient) RemoveFile(filename string) error {
	if c.CheckRepo() == false {
		return nil
	}
	LogConsole.Infof("Remove File %s to repo", filename)
	w, err := c.Repository.Worktree()
	if err != nil {
		LogConsole.Errorf("Error git get WorkTree: %s", err.Error())
		return err
	}
	_, err = w.Status()
	if err != nil {
		LogConsole.Errorf("Error git get WorkTree Status: %s", err.Error())
		return err
	}
	if len(filename) > len(c.Folder) {
		if filename[:len(c.Folder)] == c.Folder {
			filename = filename[len(c.Folder)+1:]
		}
	}
	err = w.AddWithOptions(&git.AddOptions{Path: filename, All: true})
	if err != nil {
		LogConsole.Errorf("Error git WorkTree add file: %s %s", err.Error(), filename)
		return err
	}
	commitName := fmt.Sprintf("Remove File Automated %s", filename)
	commit, err := w.Commit(commitName, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Pupirka",
			Email: ConfigV.GetString("git.user"),
			When:  time.Now(),
		},
	})
	_, err = c.Repository.CommitObject(commit)
	if err != nil {
		LogConsole.Infof("Error git Commit name:%s", commitName)
		return err
	}
	if c.RepoUrl != "" {
		c.NeedPush = true
	}

	return nil
}

// CheckPush pushes only when a previous backup operation created a commit.
func (c *GitClient) CheckPush() error {
	if c.CheckRepo() == false {
		return nil
	}
	if c.NeedPush {
		return c.Push()
	}
	return nil
}

// Push sends pending backup commits to the configured remote.
func (c *GitClient) Push() error {
	if c.CheckRepo() == false {
		return nil
	}

	err := c.Repository.Push(&git.PushOptions{
		Auth: &c.Auth,
	})
	if err == git.NoErrAlreadyUpToDate {
		return nil
	}
	if err != nil {
		LogConsole.Warn(err.Error())
		return err
	}
	LogConsole.Infof("git Push remote: %s", c.RepoUrl)
	c.NeedPush = false
	return nil
}

// SetCommit stages and commits changes to an existing backup file.
func (c *GitClient) SetCommit(filename string) error {
	if c.CheckRepo() == false {
		return nil
	}
	LogConsole.Infof("Change File %s to repo", filename)
	w, err := c.Repository.Worktree()
	if err != nil {
		LogConsole.Error(fmt.Sprintf("Error git get WorkTree: %s", err.Error()))
		return err
	}

	_, err = w.Status()
	if err != nil {
		LogConsole.Error(fmt.Sprintf("Error git get WorkTree Status: %s", err.Error()))
		return err
	}
	if len(filename) > len(c.Folder) {
		if filename[:len(c.Folder)] == c.Folder {
			filename = filename[len(c.Folder)+1:]
		}
	}
	_, err = w.Add(filename)
	if err != nil {
		LogConsole.Error(fmt.Sprintf("Error git WorkTree add file: %s %s", err.Error(), filename))
		return err
	}
	commitName := fmt.Sprintf("Change File %s", filename)
	commit, err := w.Commit(commitName, &git.CommitOptions{
		Author: &object.Signature{
			Name:  ConfigV.GetString("git.user"),
			Email: ConfigV.GetString("git.email"),
			When:  time.Now(),
		},
	})
	_, err = c.Repository.CommitObject(commit)
	if err != nil {
		LogConsole.Error(fmt.Sprintf("Error git Commit name:%s", commitName))
		return err
	}
	c.NeedPush = true
	return nil
}

// CheckRepo reports whether Git integration is enabled by git.remote.
func (c *GitClient) CheckRepo() bool {
	if c.RepoUrl == "" {
		return false
	}
	return true
}
