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

type RefType int8

const (
	BRANCH_E RefType = iota
	NOTE_E
	REMOTE_E
	TAG_E
	SYMBOLIC_E
	INVALID_E
)

type TagData struct {
	Name        string
	Target      plumbing.Hash
	IsAnnotated bool
	Head        string
	Tagger      string
	Date        time.Time
}
type TagDataSlice []TagData

func (a TagDataSlice) Len() int           { return len(a) }
func (a TagDataSlice) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a TagDataSlice) Less(i, j int) bool { return a[i].Date.Before(a[j].Date) }

type ShortRef struct {
	Name string
	Type RefType
}

type RefMap map[plumbing.Hash]ShortRef

func refToType(ref *plumbing.Reference) RefType {
	var refType RefType
	name := ref.Name()
	if name.IsBranch() {
		refType = BRANCH_E
	} else if name.IsNote() {
		refType = NOTE_E
	} else if name.IsRemote() {
		refType = REMOTE_E
	} else if name.IsTag() {
		refType = TAG_E
	} else if ref.Type() == plumbing.SymbolicReference {
		refType = SYMBOLIC_E
	} else {
		refType = INVALID_E
	}
	return refType
}

func (self *ShortRef) fromRef(ref *plumbing.Reference) {
	self.Type = refToType(ref)
	if self.Type != INVALID_E {
		self.Name = ref.Name().Short()
	}
}

func (data *TagData) fromTag(tag *object.Tag) {
	data.Name = tag.Name
	data.Target = tag.Target
	data.IsAnnotated = true
	data.Head = strings.Split(tag.Message, "\n\n")[0]
	data.Tagger = tag.Tagger.Name
	data.Date = tag.Tagger.When
}

func (data *TagData) fromReference(ref *plumbing.Reference, repo *git.Repository) error {
	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return err
	}
	data.Name = ref.Name().Short()
	data.Target = ref.Hash()
	data.IsAnnotated = false
	data.Head = ""
	data.Tagger = commit.Author.Name
	data.Date = commit.Committer.When
	return nil
}

func (data *TagData) fromRefSwitch(tag *plumbing.Reference, repo *git.Repository) error {
	obj, err := repo.TagObject(tag.Hash())
	switch err {
	case nil: // This is an annotated tag
		data.fromTag(obj)
	case plumbing.ErrObjectNotFound:
		err = data.fromReference(tag, repo)
	}
	return err
}

func generateRefs(branches *[]string, tags *TagDataSlice, data BaseData, buffer *bytes.Buffer) error {

	partialsPath := filepath.Join("templates", "partials")
	basePath := filepath.Join("templates", "base.html")
	navPath := filepath.Join(partialsPath, "nav.html")
	refsPath := filepath.Join(partialsPath, "content", "refs.html")
	footPath := filepath.Join(partialsPath, "footer.html")
	baseTempl, err := template.Must(template.Must(template.ParseFS(templates, basePath)).ParseFS(templates, navPath)).ParseFS(templates, footPath)
	if err != nil {
		return err
	}
	refsTempl, err := baseTempl.ParseFS(templates, refsPath)
	if err != nil {
		return err
	}

	err = refsTempl.Execute(buffer, struct {
		Branches []string
		Tags     TagDataSlice
		BaseData
	}{
		*branches,
		*tags,
		data,
	})
	return err
}
