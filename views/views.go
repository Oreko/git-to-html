package views

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"golang.org/x/sync/errgroup"
)

func WriteCommits(repository *git.Repository, baseDir string) error {
	commitDir := filepath.Join(baseDir, "c")
	err := os.MkdirAll(commitDir, 0755)
	if err != nil {
		return err
	}

	commitIter, err := repository.Log(&git.LogOptions{
		All: true,
	})
	if err != nil {
		return err
	}
	defer commitIter.Close()

	threadGroup := new(errgroup.Group)

	_ = commitIter.ForEach(func(commit *object.Commit) error {
		threadGroup.Go(func() error {
			var buffer bytes.Buffer
			fileName := fmt.Sprintf("%s.html", commit.Hash)
			commitPath := filepath.Join(commitDir, fileName)
			root := relRootFromPath(commitPath)
			commitBase := BaseData{
				Title:     fmt.Sprintf("%s", commit.Hash),
				StylePath: root + "..",
				Nav: NavData{
					Root:   root,
					Commit: "",
				},
			}
			// TODO: Need to handle notes (see refs in other sections)
			// I think notes are stored as trees, so we'll need to do some extra work
			err := generateCommit(commit, commitBase, &buffer)
			if err != nil {
				return err
			}
			err = writeHtml(&buffer, commitPath)
			return err
		})
		return nil
	})

	err = threadGroup.Wait()
	return err
}

// TODO: Function is doing too much. Candidate for splitting.
func WriteBranches(repository *git.Repository, baseDir string) error {
	const treePrefix = "t"
	branchIter, err := repository.Branches()
	if err != nil {
		return err
	}
	defer branchIter.Close()

	threadGroup := new(errgroup.Group)
	err = branchIter.ForEach(func(branch *plumbing.Reference) error {
		branchName := filepath.Base(string(branch.Name()))
		branchDir := filepath.Join(baseDir, branchName)
		treeDir := filepath.Join(branchDir, treePrefix)
		err = os.MkdirAll(treeDir, 0755)
		if err != nil {
			return err
		}

		commit, err := repository.CommitObject(branch.Hash())
		if err != nil {
			return err
		}

		// Generate the branch's index
		var branchBuffer bytes.Buffer
		branchPath := filepath.Join(branchDir, "index.html")
		if err != nil {
			return err
		}

		root := relRootFromPath(branchPath)
		branchBase := BaseData{
			Title:     branchName,
			StylePath: root + "..",
			Nav: NavData{
				Root:   root,
				Commit: "",
				Branch: branchName,
			},
		}
		err = generateBranch(commit, treePrefix, branchBase, &branchBuffer)
		if err != nil {
			return err
		}
		err = writeHtml(&branchBuffer, branchPath)
		if err != nil {
			return err
		}

		// Generate the branch's log
		var logBuffer bytes.Buffer
		logPath := filepath.Join(branchDir, "log.html")
		root = relRootFromPath(logPath)
		logBase := BaseData{
			Title:     fmt.Sprintf("%s - log", branchName),
			StylePath: root + "..",
			Nav: NavData{
				Root:   root,
				Commit: fmt.Sprintf("%s", branch.Hash()),
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
		err = generateLog(commit, refs, logBase, &logBuffer)
		if err != nil {
			return err
		}
		err = writeHtml(&logBuffer, logPath)
		if err != nil {
			return err
		}

		// Generate the pages for each file/dir in the branch
		tree, err := commit.Tree()
		if err != nil {
			return err
		}
		walker := object.NewTreeWalker(tree, true, nil)
		defer walker.Close()
		for {
			name, entry, err := walker.Next()
			if err == io.EOF {
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

				path := filepath.Join(treeDir, name)
				err = os.MkdirAll(path, 0755)
				if err != nil {
					return err
				}

				root := relRootFromPath(path)
				threadGroup.Go(func() error {
					var treeBuffer bytes.Buffer

					treeBase := BaseData{
						Title:     name,
						StylePath: root + "..",
						Nav: NavData{
							Root:   root,
							Commit: fmt.Sprintf("%s", branch.Hash()),
							Branch: branchName,
						},
					}

					err = generateTree(subTree, treeName, treeBase, &treeBuffer)
					if err != nil {
						return err
					}

					err = writeHtml(&treeBuffer, path+".html")
					return err
				})
			case filemode.Submodule:
				// No files need to be generated for a submodule since it will be rendered as a link to the submodule's repository
				continue
			default:
				threadGroup.Go(func() error {
					var fileBuffer bytes.Buffer
					file, err := tree.TreeEntryFile(&entry)
					if err != nil {
						return err
					}
					path := filepath.Join(treeDir, name+".html")

					root := relRootFromPath(path)
					fileBase := BaseData{
						Title:     name,
						StylePath: root + "..",
						Nav: NavData{
							Root:   root,
							Commit: fmt.Sprintf("%s", branch.Hash()),
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
		return nil
	})
	if err != nil {
		return err
	}

	err = threadGroup.Wait()
	return err
}

func WriteRefs(repository *git.Repository, baseDir string) error {
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

	tags := make([]TagData, 0)
	err = tagIter.ForEach(func(tag *plumbing.Reference) error {
		var data TagData
		obj, err := repository.TagObject(tag.Hash())
		switch err {
		case nil:
			data.fromTag(obj)
		case plumbing.ErrObjectNotFound:
			data.fromReference(tag)
			err = nil
		default:
			return err
		}
		tags = append(tags, data)
		return err
	})
	if err != nil {
		return err
	}

	root := relRootFromPath(refsPath)
	refBase := BaseData{
		Title:     "References",
		StylePath: root + "..",
		Nav: NavData{
			Root:   root,
			Commit: "",
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
