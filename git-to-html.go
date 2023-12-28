package main

import (
	"fmt"
	"os"
	"path/filepath"

	git "github.com/go-git/go-git/v5"

	"github.com/oreko/git-to-html/views"
)

func checkIfError(err error) {
	if err == nil {
		return
	}

	fmt.Fprintf(os.Stderr, "\x1b[31;1m%s\x1b[0m\n", fmt.Sprintf("error: %s", err))
	os.Exit(1)
}

func main() {
	if len(os.Args) != 2 {
		execPath, err := os.Executable()
		checkIfError(err)
		fmt.Printf("Usage: %s repository_path\n", filepath.Base(execPath))
		os.Exit(1)
	}

	repositoryPath := os.Args[1]
	repository, err := git.PlainOpen(repositoryPath)
	checkIfError(err)

	baseDir := "public"
	err = os.MkdirAll(baseDir, 0755)
	checkIfError(err)

	err = views.WriteCommits(repository, baseDir)
	checkIfError(err)

	err = views.WriteBranches(repository, baseDir)
	checkIfError(err)

	err = views.WriteRefs(repository, baseDir)
	checkIfError(err)

	os.Exit(0)
}
