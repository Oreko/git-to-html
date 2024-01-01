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
	Time      time.Time
	Hash      plumbing.Hash
	Blob      BlobData
}

type NoteMap = map[string][]NoteData

func recentNoteTime(notes []NoteData) time.Time {
	var recent time.Time
	for _, note := range notes {
		if note.Time.After(recent) {
			recent = note.Time
		}
	}
	return recent
}

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
	splitHeadAndBody := strings.Split(commit.Message, "\n\n")
	data.Head = splitHeadAndBody[0]
	if len(data.Head) > 50 {
		data.Message = strings.Join(splitHeadAndBody[:], "\n\n")
	} else {
		data.Message = strings.Join(splitHeadAndBody[1:], "\n\n")
	}

	// TODO: We should combine diffs for a merge. How should this be done?
	patch, err := patchFromCommit(commit)
	if err != nil {
		return err
	}
	data.Stats = patch.Stats()
	data.Lines = makeDiff(patch)

	return nil
}

func patchFromCommit(commit *object.Commit) (*object.Patch, error) {
	var pTree *object.Tree = nil
	parent, err := commit.Parent(0)
	if err == nil {
		pTree, err = parent.Tree()
	} else if err != object.ErrParentNotFound {
		return nil, err
	}
	cTree, err := commit.Tree()
	if err != nil {
		return nil, err
	}
	changes, err := pTree.Diff(cTree)
	if err != nil {
		return nil, err
	}
	return changes.Patch()
}

func latestCommit(path *string, repository *git.Repository, branch plumbing.Hash) (plumbing.Hash, time.Time, error) {
	// This code is currently too slow due to an issue on the go-git repo causing Log to run for too long
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

	partialsPath := filepath.Join("templates", "partials")
	basePath := filepath.Join("templates", "base.html")
	navPath := filepath.Join(partialsPath, "nav.html")
	commitPath := filepath.Join(partialsPath, "content", "commit.html")
	blobPath := filepath.Join(partialsPath, "blob.html") // Notes are blobs
	footPath := filepath.Join(partialsPath, "footer.html")
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
