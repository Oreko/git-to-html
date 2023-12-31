package views

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/microcosm-cc/bluemonday"
	blackfriday "github.com/russross/blackfriday/v2"
)

//go:embed templates templates/partials templates/partials/content
var templates embed.FS

type BaseData struct {
	Title       string
	Home        string
	StylePath   string
	FaviconPath string
	Nav         NavData
	Root        string
}

type NavData struct {
	Commit string
	Branch string
}

type FileMode int8

const (
	EMPTY_E FileMode = iota
	DIR_E
	REGULAR_E
	DEPRECATED_E
	EXECUTABLE_E
	SYMLINK_E
	SUBMODULE_E
)

var modeToEnum = map[filemode.FileMode]FileMode{
	filemode.Empty:      EMPTY_E,
	filemode.Dir:        DIR_E,
	filemode.Regular:    REGULAR_E,
	filemode.Deprecated: DEPRECATED_E,
	filemode.Executable: EXECUTABLE_E,
	filemode.Symlink:    SYMLINK_E,
	filemode.Submodule:  SUBMODULE_E,
}

func isSkipWrite(path string, objectTime time.Time) (bool, error) {
	if info, err := os.Stat(path); err == nil {
		return info.ModTime().After(objectTime), nil
	} else if errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else {
		return true, err
	}
}

func writeHtml(buffer *bytes.Buffer, path string) error {
	err := os.WriteFile(path, buffer.Bytes(), 0644)
	return err
}

func mdToHtml(markdownFile *object.File) (template.HTML, error) {
	reader, err := markdownFile.Reader()
	if err != nil {
		return "", err
	}
	defer reader.Close()

	buffer := new(bytes.Buffer)
	buffer.ReadFrom(reader)
	unsafe := blackfriday.Run(buffer.Bytes())
	html := bluemonday.UGCPolicy().SanitizeBytes(unsafe)

	return template.HTML(html[:]), nil
}

func prettifyBytes(size int64) string {
	if size == 0 {
		return "0 B"
	}
	if size < 0 {
		return "? B"
	}
	floated := float64(size)
	exp := math.Floor(math.Log(floated) / math.Log(1000))
	var unitPrefix string = ""
	if int(exp) > 0 {
		unitPrefix = fmt.Sprintf("%c", "kMGTPE"[int(exp)])
	}
	return fmt.Sprintf("%.1f %sB", floated/math.Pow(1000, exp), unitPrefix)
}

func relRootFromPath(path string) string {
	cleaned := filepath.Clean(path)
	split := strings.Split(cleaned, string(filepath.Separator))
	// We subtract two here because we expect a full path to file instead of a directory
	// For example, a/b/c/d/e.txt would be split into [a,b,c,d,e.txt] (of length 5)
	// and the corresponding relative path to "a" from d is ../../../ (of length 3)
	depth := max(len(split)-2, 0)
	root := strings.Repeat("../", depth)
	return root
}
