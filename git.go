package main

import (
	"fmt"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/http"
	"time"
)

type GitClient struct {
	Folder     string
	Repo       string
	Token      string
	NeedPush   bool
	RepoUrl    string
	Auth       http.BasicAuth
	Branch     plumbing.ReferenceName
	Repository *git.Repository
}

var gitClient GitClient

func RunGlobalPreStart() error {

	gitClient.Folder = ConfigV.GetString("path.backup")
	gitClient.RepoUrl = ConfigV.GetString("git.remote")
	if gitClient.RepoUrl == "" {
		rep, err := git.PlainOpen(gitClient.Folder)
		if err != nil {
			return err
		}
		gitClient.Repository = rep
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

func (c *GitClient) AddFile(filename string) error {
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
func (c *GitClient) RemoveFile(filename string) error {
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
	_, err = w.Remove(filename)
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

func (c *GitClient) CheckPush() error {
	if c.NeedPush {
		return c.Push()
	}
	return nil
}

func (c *GitClient) Push() error {

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

func (c *GitClient) SetCommit(filename string) error {
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
			Name:  "Pupirka",
			Email: ConfigV.GetString("git.user"),
			When:  time.Now(),
		},
	})
	_, err = c.Repository.CommitObject(commit)
	if err != nil {
		LogConsole.Error(fmt.Sprintf("Error git Commit name:%s", commitName))
		return err
	}
	go c.Push()
	return nil
}
