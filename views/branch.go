package views

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"path/filepath"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/utils/ioutil"
)

type File struct {
	Mode FileMode
	Size string
	Link string
}

type TreeData struct {
	Readme   template.HTML
	Tree     map[string]File
	TreeName string
}

type BlobData struct {
	Lines     []string
	LineCount []int
	IsBinary  bool
	Markdown  template.HTML
}

type LogCommit struct {
	Hash    plumbing.Hash
	Author  string
	Date    time.Time
	Message string
	Refs    []ShortRef
	Stats   LogStats
}

type LogStats struct {
	Files     int
	Additions int
	Deletions int
}

type LogData struct {
	Commits []LogCommit
}

// This global is treated as a constant and should only be read
// It acts as a string mapping for each file enum for the templates
var fileFuncMap = template.FuncMap{
	"Empty":      func() FileMode { return EMPTY_E },
	"Dir":        func() FileMode { return DIR_E },
	"Regular":    func() FileMode { return REGULAR_E },
	"Deprecated": func() FileMode { return DEPRECATED_E },
	"Executable": func() FileMode { return EXECUTABLE_E },
	"Symlink":    func() FileMode { return SYMLINK_E },
	"Submodule":  func() FileMode { return SUBMODULE_E },
}

// This global is treated as a constant and should only be read
// It acts as a string mapping for each reference enum for the templates
var refFuncMap = template.FuncMap{
	"RefEnumToString": func(enum RefType) string {
		var representation string = ""
		switch enum {
		case BRANCH_E:
			representation = "branchType"
		case NOTE_E:
			representation = "noteType"
		case REMOTE_E:
			representation = "remoteType"
		case TAG_E:
			representation = "tagType"
		case SYMBOLIC_E:
			representation = "symbolicType"
		case INVALID_E:
			representation = "invalidType"
		}
		return representation
	},
}

func (data *TreeData) fromTreeAndSubmodules(tree *object.Tree, submoduleMap map[string]string) error {
	data.Tree = make(map[string]File, 0)

	for _, entry := range tree.Entries {
		var prettySize string = ""
		var link string = ""
		name := entry.Name
		mode := entry.Mode
		if mode == filemode.Submodule {
			link = submoduleMap[name]
		} else if mode != filemode.Symlink && mode != filemode.Dir {
			file, err := tree.TreeEntryFile(&entry)
			if err != nil {
				return err
			}
			switch strings.ToLower(name) {
			// If the markdown conversion fails, just don't render it
			case "readme":
				if data.Readme == "" {
					data.Readme, _ = mdToHtml(file)
				}
			case "readme.md":
				data.Readme, _ = mdToHtml(file)
			}

			isBinary, err := file.IsBinary()
			if err != nil {
				return err
			}

			if isBinary {
				size, err := tree.Size(name)
				if err != nil {
					return err
				}
				prettySize = prettifyBytes(size)
			} else {
				lines, err := file.Lines()
				if err != nil {
					return err
				}
				prettySize = fmt.Sprintf("%dL", len(lines))
			}
		}

		data.Tree[name] = File{
			Mode: modeToEnum[mode],
			Size: prettySize,
			Link: link,
		}
	}
	return nil
}

func (data *BlobData) fromFile(file *object.File) error {
	bin, err := file.IsBinary()
	if err != nil {
		return err
	}
	data.IsBinary = bin

	if bin == false {
		lines, err := file.Lines()
		if err != nil {
			return err
		}

		data.Lines = lines
		data.LineCount = make([]int, len(lines))
		for idx := range data.LineCount {
			data.LineCount[idx] = idx + 1
		}

		if strings.HasSuffix(strings.ToLower(file.Name), ".md") {
			data.Markdown, err = mdToHtml(file)
			if err != nil {
				// If the markdown conversion fails, just don't render it
				data.Markdown = ""
			}
		}
	}

	return nil
}

func (data *LogData) fromBranchAndRefs(top *object.Commit, refs map[plumbing.Hash][]ShortRef) error {
	commitIter := object.NewCommitIterCTime(top, nil, nil)
	defer commitIter.Close()

	// We just point to the commits here since we will generate all the commit pages at the repo level.
	err := commitIter.ForEach(func(commit *object.Commit) error {
		message := strings.Split(commit.Message, "\n\n")[0]
		logEntry := LogCommit{
			Hash:    commit.Hash,
			Author:  commit.Author.Name,
			Date:    commit.Author.When,
			Message: message,
			Refs:    refs[commit.Hash],
			Stats:   LogStats{0, 0, 0},
		}
		patch, err := patchFromCommit(commit)
		if err != nil {
			return err
		}
		stats := patch.Stats()
		logEntry.Stats.Files = len(stats)
		for _, stat := range stats {
			logEntry.Stats.Additions += stat.Addition
			logEntry.Stats.Deletions += stat.Deletion
		}
		data.Commits = append(data.Commits, logEntry)
		return nil
	})
	return err
}

func getSubmoduleNameUrlMap(branch *object.Commit, repository *git.Repository) (map[string]string, error) {
	var mapping map[string]string = make(map[string]string)
	subModFile, err := branch.File(".gitmodules")
	if err == object.ErrFileNotFound {
		return mapping, nil
	} else if err != nil {
		return mapping, err
	}
	if subModFile.Mode == filemode.Symlink {
		return mapping, errors.New(".gitmodules is a symlink")
	}

	reader, err := subModFile.Reader()
	if err != nil {
		return mapping, err
	}
	defer ioutil.CheckClose(reader, &err)
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(reader)
	if err != nil {
		return mapping, err
	}

	m := config.NewModules()
	err = m.Unmarshal(buf.Bytes())
	if err != nil {
		return mapping, err
	}
	for _, submodule := range m.Submodules {
		mapping[submodule.Path] = submodule.URL
	}
	return mapping, nil
}

func generateTree(subTree *object.Tree, submoduleMap map[string]string, treeName string, base BaseData, buffer *bytes.Buffer) error {
	var treeData TreeData
	err := treeData.fromTreeAndSubmodules(subTree, submoduleMap)
	if err != nil {
		return err
	}
	treeData.TreeName = treeName

	partialsPath := filepath.Join("templates", "partials")
	basePath := filepath.Join("templates", "base.html")
	navPath := filepath.Join(partialsPath, "nav.html")
	footPath := filepath.Join(partialsPath, "footer.html")
	dirPath := filepath.Join(partialsPath, "content", "directory.html")
	treePath := filepath.Join(partialsPath, "tree.html")
	baseTempl, err := template.Must(template.Must(template.ParseFS(templates, basePath)).ParseFS(templates, navPath)).ParseFS(templates, footPath)
	if err != nil {
		return err
	}
	treeTempl, err := template.Must(baseTempl.ParseFS(templates, dirPath)).Funcs(fileFuncMap).ParseFS(templates, treePath)
	if err != nil {
		return err
	}

	err = treeTempl.Execute(buffer, struct {
		Tree TreeData
		BaseData
	}{
		treeData,
		base,
	})
	return err
}

func generateBlob(file *object.File, base BaseData, buffer *bytes.Buffer) error {
	var blobData BlobData
	blobData.fromFile(file)

	partialsPath := filepath.Join("templates", "partials")
	basePath := filepath.Join("templates", "base.html")
	navPath := filepath.Join(partialsPath, "nav.html")
	filePath := filepath.Join(partialsPath, "content", "file.html")
	blobPath := filepath.Join(partialsPath, "blob.html")
	footPath := filepath.Join(partialsPath, "footer.html")
	baseTempl, err := template.Must(template.Must(template.ParseFS(templates, basePath)).ParseFS(templates, navPath)).ParseFS(templates, footPath)
	if err != nil {
		return err
	}
	blobTempl, err := template.Must(baseTempl.ParseFS(templates, filePath)).ParseFS(templates, blobPath)
	if err != nil {
		return err
	}

	err = blobTempl.Execute(buffer, struct {
		Blob BlobData
		BaseData
	}{
		blobData,
		base,
	})
	return err
}

func generateIndex(branch *object.Commit, submoduleMap map[string]string, treePrefix string, base BaseData, buffer *bytes.Buffer) error {
	var treeData TreeData
	tree, err := branch.Tree()
	if err != nil {
		return err
	}
	err = treeData.fromTreeAndSubmodules(tree, submoduleMap)
	if err != nil {
		return err
	}
	treeData.TreeName = treePrefix

	partialsPath := filepath.Join("templates", "partials")
	basePath := filepath.Join("templates", "base.html")
	navPath := filepath.Join(partialsPath, "nav.html")
	branchPath := filepath.Join(partialsPath, "content", "branch.html")
	treePath := filepath.Join(partialsPath, "tree.html")
	footPath := filepath.Join(partialsPath, "footer.html")
	baseTempl, err := template.Must(template.Must(template.ParseFS(templates, basePath)).ParseFS(templates, navPath)).ParseFS(templates, footPath)
	if err != nil {
		return err
	}
	treeTempl, err := template.Must(baseTempl.ParseFS(templates, branchPath)).Funcs(fileFuncMap).ParseFS(templates, treePath)
	if err != nil {
		return err
	}

	err = treeTempl.Execute(buffer, struct {
		Tree TreeData
		BaseData
	}{
		treeData,
		base,
	})
	return err
}

func generateLog(branch *object.Commit, refs map[plumbing.Hash][]ShortRef, base BaseData, buffer *bytes.Buffer) error {
	var logData LogData
	logData.fromBranchAndRefs(branch, refs)

	partialsPath := filepath.Join("templates", "partials")
	basePath := filepath.Join("templates", "base.html")
	navPath := filepath.Join(partialsPath, "nav.html")
	logPath := filepath.Join(partialsPath, "content", "log.html")
	footPath := filepath.Join(partialsPath, "footer.html")
	baseTempl, err := template.Must(template.Must(template.ParseFS(templates, basePath)).ParseFS(templates, navPath)).ParseFS(templates, footPath)
	if err != nil {
		return err
	}
	logTempl, err := baseTempl.Funcs(refFuncMap).ParseFS(templates, logPath)
	if err != nil {
		return err
	}

	err = logTempl.Execute(buffer, struct {
		Log LogData
		BaseData
	}{
		logData,
		base,
	})
	return err
}
