package views

import (
	"bytes"
	"html/template"
	"path/filepath"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type AuthorData struct {
	Name  string
	Email string
	Date  time.Time
}

type CommitData struct {
	Head      string
	Message   string
	Note      string
	Author    AuthorData
	Committer AuthorData
	Parents   []plumbing.Hash
	Notes     []NoteData
	Hash      plumbing.Hash
	Stats     object.FileStats
	Lines     Diff
}

type NoteData struct {
	Reference string
	Blob      BlobData
}

type NoteMap = map[string][]NoteData

func (data *AuthorData) fromSignature(signature *object.Signature) {
	data.Name = signature.Name
	data.Email = signature.Email
	data.Date = signature.When
}

func (data *CommitData) fromCommit(commit *object.Commit) error {
	data.Parents = commit.ParentHashes
	data.Author.fromSignature(&commit.Author)
	data.Committer.fromSignature(&commit.Committer)
	data.Hash = commit.Hash
	data.Head = strings.Split(commit.Message, "\n\n")[0]
	data.Message = commit.Message

	parent, err := commit.Parent(0)

	// TODO: We should combine diffs for a merge. How should this be done?
	var pTree *object.Tree = nil
	if err == nil {
		pTree, err = parent.Tree()
	} else if err != object.ErrParentNotFound {
		return err
	}
	cTree, err := commit.Tree()
	if err != nil {
		return err
	}
	changes, err := pTree.Diff(cTree)
	if err != nil {
		return err
	}
	patch, err := changes.Patch()
	if err != nil {
		return err
	}
	data.Stats = patch.Stats()
	data.Lines = makeDiff(patch)

	return nil
}

func latestCommit(path *string, repository *git.Repository, branch plumbing.Hash) (plumbing.Hash, time.Time, error) {
	cIter, err := repository.Log(&git.LogOptions{
		Order:    git.LogOrderCommitterTime,
		From:     branch,
		FileName: path,
	})
	if err != nil {
		return plumbing.Hash{}, time.Time{}, err
	}
	commit, err := cIter.Next()
	if err != nil {
		return plumbing.Hash{}, time.Time{}, err
	}
	return commit.Hash, commit.Committer.When, nil
}

func generateCommit(commit *object.Commit, notes []NoteData, base BaseData, buffer *bytes.Buffer) error {
	var data CommitData
	err := data.fromCommit(commit)
	data.Notes = notes

	basePath := filepath.Join("templates", "base.html")
	navPath := filepath.Join("templates", "nav.html")
	commitPath := filepath.Join("templates", "commit.html")
	blobPath := filepath.Join("templates", "blob.html") // Notes are blobs
	footPath := filepath.Join("templates", "footer.html")
	baseTempl, err := template.Must(template.Must(template.ParseFS(templates, basePath)).ParseFS(templates, navPath)).ParseFS(templates, footPath)
	if err != nil {
		return nil
	}
	commitTempl, err := template.Must(baseTempl.ParseFS(templates, commitPath)).ParseFS(templates, blobPath)
	if err != nil {
		return nil
	}

	err = commitTempl.Execute(buffer, struct {
		Commit CommitData
		BaseData
	}{
		data,
		base,
	})
	return err
}
