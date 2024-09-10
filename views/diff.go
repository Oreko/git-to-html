// This code comes from the go-git repository's unified encoder
// I've replicated it here so that I can access line types for rendering
// in HTML
// See https://github.com/go-git/go-git/blob/master/plumbing/format/diff/unified_encoder.go
package views

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing/object"
)

var (
	splitLinesRegexp = regexp.MustCompile(`[^\n]*(\n|$)`)
)

type DiffType = int

const (
	Context DiffType = iota
	Meta
	Frag
	Old
	New
)

type DiffBlock struct {
	Type DiffType
	Text string
}

type Diff = []DiffBlock

type DiffBuilder struct {
	queue []DiffBlock
}

func NewDiffBuilder() *DiffBuilder {
	builder := DiffBuilder{
		make([]DiffBlock, 0),
	}
	return &builder
}

func (self *DiffBuilder) Add(kind DiffType, text string) {
	block := DiffBlock{
		Type: kind,
		Text: text,
	}
	self.queue = append(self.queue, block)
}

func (self *DiffBuilder) Append(blocks ...DiffBlock) {
	self.queue = append(self.queue, blocks...)
}

func (self *DiffBuilder) Diff() Diff {
	diff := make([]DiffBlock, 0)
	if len(self.queue) != 0 {
		var sb strings.Builder
		var currentType DiffType = self.queue[0].Type
		for _, block := range self.queue {
			text := block.Text
			if block.Type != currentType {
				newBlock := DiffBlock{
					Type: currentType,
					Text: sb.String(),
				}
				diff = append(diff, newBlock)
				sb.Reset()
				currentType = block.Type
			}
			sb.WriteString(text)
		}
		block := DiffBlock{
			Type: currentType,
			Text: sb.String(),
		}
		diff = append(diff, block)
	}
	return diff
}

func makeDiff(patch *object.Patch) Diff {
	db := NewDiffBuilder()

	message := patch.Message()
	if message != "" {
		if !strings.HasSuffix(message, "\n") {
			message = message + "\n"
		}
		db.Add(Meta, message)
	}

	for _, filePatch := range patch.FilePatches() {
		header := makeDiffHeader(filePatch)
		db.Add(Meta, header)
		g := newHunksGenerator(filePatch.Chunks())
		for _, hunk := range g.Generate() {
			blocks := hunk.Blocks()
			db.Append(blocks...)
		}
	}

	return db.Diff()
}

func appendPathLines(sb *strings.Builder, fromPath string, toPath string, isBinary bool) {
	if isBinary {
		fmt.Fprintf(sb, "Binary files %s and %s differ\n", fromPath, toPath)
	} else {
		fmt.Fprintf(sb, "--- %s\n", fromPath)
		fmt.Fprintf(sb, "+++ %s\n", toPath)
	}
}

func makeDiffHeader(filePatch diff.FilePatch) string {
	var sb strings.Builder
	from, to := filePatch.Files()
	if from == nil && to == nil {
		return ""
	}
	isBinary := filePatch.IsBinary()

	if from == nil {
		fmt.Fprintf(&sb, "diff --git a/%s b/%s\n", to.Path(), to.Path())
		fmt.Fprintf(&sb, "new file mode %o\n", to.Mode())
		fmt.Fprintf(&sb, "index %.7s..%.7s\n", plumbing.ZeroHash, to.Hash())
		appendPathLines(&sb, "/dev/null", "b/"+to.Path(), isBinary)
	} else if to == nil {
		fmt.Fprintf(&sb, "diff --git a/%s b/%s\n", from.Path(), from.Path())
		fmt.Fprintf(&sb, "deleted file mode %o\n", from.Mode())
		fmt.Fprintf(&sb, "index %.7s..%.7s\n", from.Hash(), plumbing.ZeroHash)
		appendPathLines(&sb, "a/"+from.Path(), "/dev/null", isBinary)
	} else {
		fmt.Fprintf(&sb, "diff --git a/%s b/%s\n", from.Path(), to.Path())
		if from.Mode() != to.Mode() {
			fmt.Fprintf(&sb, "old mode %o\n", from.Mode())
			fmt.Fprintf(&sb, "new mode %o\n", to.Mode())
		}
		if from.Path() != to.Path() {
			fmt.Fprintf(&sb, "rename from %s\n", from.Path())
			fmt.Fprintf(&sb, "rename to %s\n", to.Path())
		}
		if from.Hash() != to.Hash() {
			fmt.Fprintf(&sb, "index %.7s..%.7s", from.Hash(), to.Hash())
			if from.Mode() == to.Mode() {
				fmt.Fprintf(&sb, " %o", from.Mode())
			}
			sb.WriteByte('\n')
			appendPathLines(&sb, "a/"+from.Path(), "b/"+to.Path(), isBinary)
		}
	}
	return sb.String()
}

type hunksGenerator struct {
	fromLine, toLine            int
	ctxLines                    int
	chunks                      []diff.Chunk
	current                     *hunk
	hunks                       []*hunk
	beforeContext, afterContext []string
}

func newHunksGenerator(chunks []diff.Chunk) *hunksGenerator {
	return &hunksGenerator{
		chunks:   chunks,
		ctxLines: 3,
	}
}

func (g *hunksGenerator) Generate() []*hunk {
	for i, chunk := range g.chunks {
		lines := splitLines(chunk.Content())
		nLines := len(lines)

		switch chunk.Type() {
		case diff.Equal:
			g.fromLine += nLines
			g.toLine += nLines
			g.processEqualsLines(lines, i)
		case diff.Delete:
			if nLines != 0 {
				g.fromLine++
			}
			g.processHunk(i, chunk.Type())
			g.fromLine += nLines - 1
			g.current.AddOp(chunk.Type(), lines...)
		case diff.Add:
			if nLines != 0 {
				g.toLine++
			}
			g.processHunk(i, chunk.Type())
			g.toLine += nLines - 1
			g.current.AddOp(chunk.Type(), lines...)
		}

		if i == len(g.chunks)-1 && g.current != nil {
			g.hunks = append(g.hunks, g.current)
		}
	}

	return g.hunks
}

func (g *hunksGenerator) processHunk(i int, op diff.Operation) {
	if g.current != nil {
		return
	}

	var ctxPrefix string
	linesBefore := len(g.beforeContext)
	if linesBefore > g.ctxLines {
		ctxPrefix = g.beforeContext[linesBefore-g.ctxLines-1]
		g.beforeContext = g.beforeContext[linesBefore-g.ctxLines:]
		linesBefore = g.ctxLines
	}

	g.current = &hunk{ctxPrefix: strings.TrimSuffix(ctxPrefix, "\n")}
	g.current.AddOp(diff.Equal, g.beforeContext...)

	switch op {
	case diff.Delete:
		g.current.fromLine, g.current.toLine =
			g.addLineNumbers(g.fromLine, g.toLine, linesBefore, i, diff.Add)
	case diff.Add:
		g.current.toLine, g.current.fromLine =
			g.addLineNumbers(g.toLine, g.fromLine, linesBefore, i, diff.Delete)
	}

	g.beforeContext = nil
}

// addLineNumbers obtains the line numbers in a new chunk.
func (g *hunksGenerator) addLineNumbers(la, lb int, linesBefore int, i int, op diff.Operation) (cla, clb int) {
	cla = la - linesBefore
	// we need to search for a reference for the next diff
	switch {
	case linesBefore != 0 && g.ctxLines != 0:
		if lb > g.ctxLines {
			clb = lb - g.ctxLines + 1
		} else {
			clb = 1
		}
	case g.ctxLines == 0:
		clb = lb
	case i != len(g.chunks)-1:
		next := g.chunks[i+1]
		if next.Type() == op || next.Type() == diff.Equal {
			// this diff will be into this chunk
			clb = lb + 1
		}
	}

	return
}

func (g *hunksGenerator) processEqualsLines(ls []string, i int) {
	if g.current == nil {
		g.beforeContext = append(g.beforeContext, ls...)
		return
	}

	g.afterContext = append(g.afterContext, ls...)
	if len(g.afterContext) <= g.ctxLines*2 && i != len(g.chunks)-1 {
		g.current.AddOp(diff.Equal, g.afterContext...)
		g.afterContext = nil
	} else {
		ctxLines := g.ctxLines
		if ctxLines > len(g.afterContext) {
			ctxLines = len(g.afterContext)
		}
		g.current.AddOp(diff.Equal, g.afterContext[:ctxLines]...)
		g.hunks = append(g.hunks, g.current)

		g.current = nil
		g.beforeContext = g.afterContext[ctxLines:]
		g.afterContext = nil
	}
}

func splitLines(s string) []string {
	out := splitLinesRegexp.FindAllString(s, -1)
	if out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

type hunk struct {
	fromLine int
	toLine   int

	fromCount int
	toCount   int

	ctxPrefix string
	ops       []*op
}

func (h *hunk) Blocks() []DiffBlock {
	var sb strings.Builder
	Blocks := make([]DiffBlock, 0)

	sb.WriteString("@@ -")
	if h.fromCount == 1 {
		fmt.Fprintf(&sb, "%d", h.fromLine)
	} else {
		fmt.Fprintf(&sb, "%d,%d", h.fromLine, h.fromCount)
	}

	sb.WriteString(" +")

	if h.toCount == 1 {
		fmt.Fprintf(&sb, "%d", h.toLine)
	} else {
		fmt.Fprintf(&sb, "%d,%d", h.toLine, h.toCount)
	}
	sb.WriteString(" @@")

	Blocks = append(Blocks, DiffBlock{
		Type: Frag,
		Text: sb.String(),
	})

	sb.Reset()
	if h.ctxPrefix != "" {
		sb.WriteByte(' ')
		sb.WriteString(h.ctxPrefix)
	}

	Blocks = append(Blocks, DiffBlock{
		Type: Meta,
		Text: sb.String() + "\n",
	})

	for _, op := range h.ops {
		block := op.Block()
		Blocks = append(Blocks, block)
	}
	return Blocks
}

func (h *hunk) AddOp(t diff.Operation, ss ...string) {
	n := len(ss)
	switch t {
	case diff.Add:
		h.toCount += n
	case diff.Delete:
		h.fromCount += n
	case diff.Equal:
		h.toCount += n
		h.fromCount += n
	}

	for _, s := range ss {
		h.ops = append(h.ops, &op{s, t})
	}
}

type op struct {
	text string
	t    diff.Operation
}

func (o *op) Block() DiffBlock {
	var sb strings.Builder
	var block DiffBlock

	switch o.t {
	case diff.Add:
		sb.WriteByte('+')
		block.Type = New
	case diff.Delete:
		sb.WriteByte('-')
		block.Type = Old
	case diff.Equal:
		sb.WriteByte(' ')
		block.Type = Context
	}

	sb.WriteString(o.text)

	if strings.HasSuffix(o.text, "\n") == false {
		sb.WriteString("\n\\ No newline at end of file\n")
	}

	block.Text = sb.String()
	return block
}
