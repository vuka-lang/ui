package gallery

import (
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/gallery/examples"
)

//go:generate go run ./internal/propsgen

// Prop is one of a component's props: its Go field, the attribute markup
// sets it with, its type, its default and its doc comment. Every prop is
// optional: its zero value is a working default.
type Prop struct {
	Name, Attr, Type, Default, Doc string
}

type typeDoc struct {
	Doc    string
	Fields []Prop
}

// Example is one example of a component: its .vuka.html source and what it
// renders.
type Example struct {
	// Section names the example's section of the component's page; "" for
	// the page's main example.
	Section string
	// Heading is one sentence saying what the section shows.
	Heading string
	// File is the example's file, and Source its text: the source compiled
	// into what the example renders.
	File, Source string
	// Height is the preview's height in pixels for an overlay; 0 sizes the
	// preview to its content.
	Height int
	Node   ui.Node
}

// Component is one of the kit's components, as its gallery page shows it.
type Component struct {
	// Name is its Go name: Button, DataTable, or Icon for a function.
	Name string
	// Slug names its page: button, data-table.
	Slug string
	// Title is its name in words: Data Table.
	Title string
	// Description is one sentence from its doc comment.
	Description string
	// Doc is its whole doc comment.
	Doc string
	// Examples are its examples, the main one first.
	Examples []Example
	// Props are its fields; none for a component written as a function.
	Props []Prop
	// Related are slugs of components used with it.
	Related []string
}

// families group the components a page lists as related.
var families = [][]string{
	{"Button", "Submit", "Actions", "Spinner", "Icon"},
	{"Form", "Field", "Input", "Password", "Textarea", "Select", "Checkbox"},
	{"Modal", "Dialog", "Drawer", "Popover", "Notice", "ConfirmBox", "Toast"},
	{"Table", "DataTable", "Tabs", "Tile"},
	{"Alert", "Badge", "Card", "Skeleton", "Image"},
}

// Components are the kit's components with their examples, ordered by
// title as a catalogue is.
func Components() []Component {
	byName := map[string]*Component{}
	var out []*Component
	for _, e := range examples.All {
		c := byName[e.Component]
		if c == nil {
			c = &Component{Name: e.Component, Slug: slug(e.Component), Title: title(e.Component)}
			if td, ok := typeDocs[e.Component]; ok {
				c.Doc, c.Props = td.Doc, td.Fields
			} else {
				c.Doc = funcDocs[e.Component]
			}
			c.Description = describe(c.Name, c.Doc)
			byName[e.Component] = c
			out = append(out, c)
		}
		c.Examples = append(c.Examples, Example{Section: e.Section, Heading: e.Heading, File: e.File, Source: examples.Source(e.File), Height: e.Height, Node: e.Node})
	}
	for _, c := range out {
		for _, fam := range families {
			if slices.Contains(fam, c.Name) {
				for _, n := range fam {
					if n != c.Name && byName[n] != nil {
						c.Related = append(c.Related, slug(n))
					}
				}
			}
		}
	}
	slices.SortFunc(out, func(a, b *Component) int { return strings.Compare(a.Title, b.Title) })
	res := make([]Component, len(out))
	for i, c := range out {
		res[i] = *c
	}
	return res
}

// KitTypes are the kit's components written as types (not functions):
// every one has examples (a test checks).
func KitTypes() []string {
	var out []string
	for name := range typeDocs {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

// RenderExample renders an example as a page shows it.
func RenderExample(e Example) (string, error) {
	return Render(Case{Name: e.File, Node: func() ui.Node { return e.Node }})
}

// slug is a Go name as a page's name: DataTable → data-table.
func slug(name string) string { return strings.ToLower(strings.ReplaceAll(title(name), " ", "-")) }

// title is a Go name in words: ConfirmBox → Confirm Box.
func title(name string) string {
	var b strings.Builder
	for i, r := range name {
		if i > 0 && unicode.IsUpper(r) {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

var leadIn = regexp.MustCompile(`^(\w+) (?:is|renders|draws|shows) (?:an? |the )?`)

// describe is a doc comment's first sentence, without the name it starts
// with: "Badge is a small status pill" → "A small status pill."
func describe(name, doc string) string {
	first := strings.Join(strings.Fields(doc), " ")
	if i := strings.Index(first, ". "); i >= 0 {
		first = first[:i+1]
	}
	if i := strings.Index(first, ":"); i >= 0 && !strings.HasSuffix(first, ".") {
		first = first[:i]
	}
	first = strings.TrimSuffix(strings.TrimSuffix(first, ":"), ".")
	if m := leadIn.FindStringSubmatchIndex(first); m != nil && first[m[2]:m[3]] == name {
		first = first[m[1]:]
	}
	if first == "" {
		return ""
	}
	r := []rune(first)
	r[0] = unicode.ToUpper(r[0])
	return string(r) + "."
}
