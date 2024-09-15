package main

import (
	"flag"
	"fmt"
	"os"

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

func internalMain(repositoryPath string, repositoryName string, config views.Config) int {
	repository, err := git.PlainOpen(repositoryPath)
	if res := checkIfError(err); res != 0 {
		return res
	}

	baseDir := "public"
	err = os.MkdirAll(baseDir, 0755)
	if res := checkIfError(err); res != 0 {
		return res
	}

	err = views.WriteCommits(repository, repositoryName, baseDir, config)
	if res := checkIfError(err); res != 0 {
		return res
	}

	branchIter, err := repository.Branches()
	checkIfError(err)
	defer branchIter.Close()
	err = branchIter.ForEach(func(branch *plumbing.Reference) error {
		return views.WriteBranch(branch, repository, repositoryName, baseDir, config)
	})
	if res := checkIfError(err); res != 0 {
		return res
	}

	err = views.WriteRefs(repository, repositoryName, baseDir, config)
	if res := checkIfError(err); res != 0 {
		return res
	}

	return 0
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] repository_path repository_name\n", os.Args[0])
		flag.PrintDefaults()
	}
	var logLimit = flag.Uint("l", 0, "Limit on the number of commits to render in the log with 0 giving no limit (default 0)")
	var stylePath = flag.String("s", "../static/styles.css", "Relative path from public to look for styles")
	flag.Parse()

	config := views.Config{*logLimit, *stylePath}

	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(1)
	}
	args := flag.Args()
	os.Exit(internalMain(args[0], args[1], config))
}
