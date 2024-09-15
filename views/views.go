package views

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"golang.org/x/sync/errgroup"
)

func WriteCommits(repository *git.Repository, repositoryName string, baseDir string, config Config) error {
	commitDir := filepath.Join(baseDir, "c")
	err := os.MkdirAll(commitDir, 0755)
	if err != nil {
		return err
	}

	commitIter, err := repository.Log(&git.LogOptions{
		All:   true,
		Order: git.LogOrderCommitterTime,
	})
	if err != nil {
		return err
	}
	defer commitIter.Close()

	// This iterator is over the notes "branches" e.g., refs/notes/commits
	noteIter, err := repository.Notes()
	if err != nil {
		return err
	}
	defer noteIter.Close()

	var noteMap NoteMap = make(NoteMap)
	err = noteIter.ForEach(func(note *plumbing.Reference) error {
		commit, err := repository.CommitObject(note.Hash())
		if err != nil { // plumbing.ErrObjectNotFound is included here
			return err
		}

		// Notes consist of a blob
		fileIter, err := commit.Files()
		if err != nil {
			return err
		}
		_ = fileIter.ForEach(func(file *object.File) error {
			var blobData BlobData
			blobData.fromFile(file)
			noteData := NoteData{
				Reference: string(note.Name()),
				Hash:      commit.Hash,
				Time:      commit.Committer.When,
				Blob:      blobData,
			}

			commitHash := file.Name
			if val, ok := noteMap[commitHash]; ok {
				noteMap[commitHash] = append(val, noteData)
			} else {
				noteMap[commitHash] = []NoteData{noteData}
			}
			return nil
		})

		return nil
	})
	if err != nil {
		return err
	}

	err = commitIter.ForEach(func(commit *object.Commit) error {
		fileName := fmt.Sprintf("%s.html", commit.Hash)
		commitPath := filepath.Join(commitDir, fileName)

		notes := noteMap[fmt.Sprintf("%s", commit.Hash)]
		noteTime := recentNoteTime(notes)
		var modTime time.Time
		if noteTime.After(commit.Committer.When) {
			modTime = noteTime
		} else {
			modTime = commit.Committer.When
		}

		skip, err := isSkipWrite(commitPath, modTime)
		if err != nil {
			return err
		}
		if skip {
			return nil
		}
		var buffer bytes.Buffer
		root := relRootFromPath(commitPath)
		commitBase := BaseData{
			Title:     fmt.Sprintf("%s", commit.Hash),
			StylePath: root + config.StylePath,
			Home:      repositoryName,
			Root:      root,
			Nav: NavData{
				Commit: "",
				Branch: "",
			},
		}
		// PERFORMANCE: Calling stats for every commit is expensive.
		err = generateCommit(commit, notes, commitBase, &buffer)
		if err != nil {
			return err
		}
		err = writeHtml(&buffer, commitPath)
		return err
	})

	return err
}

func WriteIndex(branch *object.Commit, repository *git.Repository, repositoryName string, hash plumbing.Hash, branchDir string, branchName string, treePrefix string, config Config) error {
	var branchBuffer bytes.Buffer
	branchPath := filepath.Join(branchDir, "index.html")

	root := relRootFromPath(branchPath)
	branchBase := BaseData{
		Title:     branchName,
		StylePath: root + config.StylePath,
		Home:      repositoryName,
		Root:      root,
		Nav: NavData{
			Commit: fmt.Sprintf("%s", hash),
			Branch: branchName,
		},
	}
	submoduleMap, err := getSubmoduleNameUrlMap(branch, repository)
	if err != nil {
		return err
	}

	err = generateIndex(branch, submoduleMap, treePrefix, branchBase, &branchBuffer)
	if err != nil {
		return err
	}

	err = writeHtml(&branchBuffer, branchPath)
	if err != nil {
		return err
	}

	return nil
}

func WriteLog(branch *object.Commit, repository *git.Repository, repositoryName string, hash plumbing.Hash, branchDir string, branchName string, config Config) error {
	var logBuffer bytes.Buffer
	logPath := filepath.Join(branchDir, "log.html")
	skip, err := isSkipWrite(logPath, branch.Committer.When)
	if err != nil {
		return err
	}
	if skip {
		return nil
	}

	root := relRootFromPath(logPath)
	logBase := BaseData{
		Title:     fmt.Sprintf("%s - log", branchName),
		StylePath: root + config.StylePath,
		Home:      repositoryName,
		Root:      root,
		Nav: NavData{
			Commit: fmt.Sprintf("%s", hash),
			Branch: branchName,
		},
	}

	var refs = make(map[plumbing.Hash][]ShortRef)
	refIter, err := repository.References()
	if err != nil {
		return err
	}
	err = refIter.ForEach(func(ref *plumbing.Reference) error {
		var shortRef ShortRef
		shortRef.fromRef(ref)
		if (shortRef.Type != INVALID_E) && (shortRef.Type != SYMBOLIC_E) && (shortRef.Type != NOTE_E) {
			var hash plumbing.Hash = ref.Hash()
			if ref.Name().IsTag() {
				obj, err := repository.TagObject(hash)
				switch err {
				case nil: // This is an annotated tag
					hash = obj.Target
				case plumbing.ErrObjectNotFound:
				default:
					return err
				}
			}
			if val, ok := refs[hash]; ok {
				refs[hash] = append(val, shortRef)
			} else {
				refs[hash] = []ShortRef{shortRef}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	err = generateLog(branch, refs, logBase, &logBuffer, config.LogLimit)
	if err != nil {
		return err
	}

	err = writeHtml(&logBuffer, logPath)
	if err != nil {
		return err
	}

	return nil
}

func WriteTree(branch *object.Commit, repository *git.Repository, repositoryName string, treeDir string, branchName string, config Config) error {
	// Generate the pages for each file/dir in the branch
	tree, err := branch.Tree()
	if err != nil {
		return err
	}
	walker := object.NewTreeWalker(tree, true, nil)
	defer walker.Close()
	submoduleMap, err := getSubmoduleNameUrlMap(branch, repository)
	if err != nil {
		return err
	}

	threadGroup := new(errgroup.Group)
	for {
		name, entry, err := walker.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}

		switch entry.Mode {
		case filemode.Dir:
			treeName := filepath.Base(string(name))
			subTree, err := walker.Tree().Tree(treeName)
			if err != nil {
				return err
			}

			folderPath := filepath.Join(treeDir, name)
			htmlPath := folderPath + ".html"

			err = os.MkdirAll(folderPath, 0755)
			if err != nil {
				return err
			}

			root := relRootFromPath(folderPath)
			var treeBuffer bytes.Buffer
			treeBase := BaseData{
				Title:     name,
				StylePath: root + config.StylePath,
				Home:      repositoryName,
				Root:      root,
				Nav: NavData{
					Commit: "",
					Branch: branchName,
				},
			}

			err = generateTree(subTree, submoduleMap, treeName, treeBase, &treeBuffer)
			if err != nil {
				return err
			}

			err = writeHtml(&treeBuffer, htmlPath)
			if err != nil {
				return err
			}
		case filemode.Submodule:
			// No files need to be generated for a submodule since it will be rendered as a link to the submodule's repository
			continue
		default:
			file, err := tree.TreeEntryFile(&entry)
			if err != nil {
				return err
			}

			threadGroup.Go(func() error {
				var fileBuffer bytes.Buffer

				path := filepath.Join(treeDir, name+".html")
				root := relRootFromPath(path)
				fileBase := BaseData{
					Title:     name,
					StylePath: root + config.StylePath,
					Home:      repositoryName,
					Root:      root,
					Nav: NavData{
						Commit: "",
						Branch: branchName,
					},
				}
				err = generateBlob(file, fileBase, &fileBuffer)
				if err != nil {
					return err
				}

				err = writeHtml(&fileBuffer, path)
				return err
			})
		}

	}

	return threadGroup.Wait()
}

func WriteBranch(branch *plumbing.Reference, repository *git.Repository, repositoryName string, baseDir string, config Config) error {
	const treePrefix = "t"

	branchName := filepath.Base(string(branch.Name()))
	branchDir := filepath.Join(baseDir, branchName)
	treeDir := filepath.Join(branchDir, treePrefix)
	err := os.MkdirAll(treeDir, 0755)
	if err != nil {
		return err
	}

	commit, err := repository.CommitObject(branch.Hash())
	if err != nil {
		return err
	}

	err = WriteIndex(commit, repository, repositoryName, branch.Hash(), branchDir, branchName, treePrefix, config)
	if err != nil {
		return err
	}

	err = WriteLog(commit, repository, repositoryName, branch.Hash(), branchDir, branchName, config)
	if err != nil {
		return err
	}

	err = WriteTree(commit, repository, repositoryName, treeDir, branchName, config)
	if err != nil {
		return err
	}

	return nil
}

func WriteRefs(repository *git.Repository, repositoryName string, baseDir string, config Config) error {
	refsPath := filepath.Join(baseDir, "refs.html")

	branchIter, err := repository.Branches()
	if err != nil {
		return err
	}
	defer branchIter.Close()

	branches := make([]string, 0)
	_ = branchIter.ForEach(func(branch *plumbing.Reference) error {
		branches = append(branches, branch.Name().Short())
		return nil
	})

	tagIter, err := repository.Tags()
	if err != nil {
		return err
	}
	defer tagIter.Close()

	tags := make(TagDataSlice, 0)
	err = tagIter.ForEach(func(tag *plumbing.Reference) error {
		var data TagData
		data.fromRefSwitch(tag, repository)
		tags = append(tags, data)
		return err
	})
	if err != nil {
		return err
	}
	sort.Sort(sort.Reverse(tags))

	root := relRootFromPath(refsPath)
	refBase := BaseData{
		Title:     "References",
		StylePath: root + config.StylePath,
		Home:      repositoryName,
		Root:      root,
		Nav: NavData{
			Commit: "",
			Branch: "",
		},
	}

	var refsBuffer bytes.Buffer
	err = generateRefs(&branches, &tags, refBase, &refsBuffer)
	if err != nil {
		return err
	}

	err = writeHtml(&refsBuffer, refsPath)
	return err
}
