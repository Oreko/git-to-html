package main

import (
	"fmt"
	"os"
	"path/filepath"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/oreko/git-to-html/views"
)

func checkIfError(err error) int {
	if err == nil {
		return 0
	}

	fmt.Fprintf(os.Stderr, "\x1b[31;1m%s\x1b[0m\n", fmt.Sprintf("error: %s", err.Error()))
	return 1
}

func internalMain(repositoryPath string, repositoryName string) int {
	repository, err := git.PlainOpen(repositoryPath)
	if res := checkIfError(err); res != 0 {
		return res
	}

	baseDir := "public"
	err = os.MkdirAll(baseDir, 0755)
	if res := checkIfError(err); res != 0 {
		return res
	}

	err = views.WriteCommits(repository, repositoryName, baseDir)
	if res := checkIfError(err); res != 0 {
		return res
	}

	branchIter, err := repository.Branches()
	checkIfError(err)
	defer branchIter.Close()
	err = branchIter.ForEach(func(branch *plumbing.Reference) error {
		return views.WriteBranch(branch, repository, repositoryName, baseDir)
	})
	if res := checkIfError(err); res != 0 {
		return res
	}

	err = views.WriteRefs(repository, repositoryName, baseDir)
	if res := checkIfError(err); res != 0 {
		return res
	}

	return 0
}

func main() {
	if len(os.Args) != 3 {
		execPath, err := os.Executable()
		checkIfError(err)
		fmt.Fprintf(os.Stdout, "Usage: %s repository_path repository_name\n", filepath.Base(execPath))
		os.Exit(1)
	}
	os.Exit(internalMain(os.Args[1], os.Args[2]))
}
