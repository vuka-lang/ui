package ui

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var cssComments = regexp.MustCompile(`(?s)/\*.*?\*/`)
var cssLeafBlock = regexp.MustCompile(`([^{}]+)\{([^{}]*)\}`)
var cssDeclaration = regexp.MustCompile(`([\w-]+)\s*:\s*([^;{}]+)`)
var cssLiteralColour = regexp.MustCompile(`(?i)#[0-9a-f]{3,8}\b|\b(?:rgba?|hsla?|oklch)\(\s*[0-9.+-][^)]*\)`)

// colourViolations inspects declaration values, not selectors (#abc is a
// valid ID). Only --ui-* declarations in ui.css's root/dark token section
// may contain literal colours. hsl(var(--background)) is an alias, not a
// literal colour, and remains valid in the legacy shadcn preset.
func colourViolations(file, css string) []string {
	clean := cssComments.ReplaceAllStringFunc(css, func(s string) string {
		return strings.Map(func(r rune) rune {
			if r == '\n' {
				return r
			}
			return ' '
		}, s)
	})
	start, end := strings.Index(css, "/* ui-tokens:start */"), strings.Index(css, "/* ui-tokens:end */")
	var out []string
	for _, block := range cssLeafBlock.FindAllStringSubmatchIndex(clean, -1) {
		selector := strings.TrimSpace(clean[block[2]:block[3]])
		body := clean[block[4]:block[5]]
		for _, decl := range cssDeclaration.FindAllStringSubmatchIndex(body, -1) {
			name := body[decl[2]:decl[3]]
			value := body[decl[4]:decl[5]]
			offset := block[4] + decl[4]
			token := file == "ui.css" && start >= 0 && end > start && offset > start && offset < end && strings.HasPrefix(name, "--ui-") && (strings.Contains(selector, ":root") || selector == ".dark")
			if token {
				continue
			}
			for _, match := range cssLiteralColour.FindAllStringIndex(value, -1) {
				line := 1 + strings.Count(clean[:offset+match[0]], "\n")
				out = append(out, file+":"+strconv.Itoa(line)+": "+value[match[0]:match[1]])
			}
		}
	}
	return out
}

func TestCSSColoursUseTokens(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(path, ".css") {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, v := range colourViolations(filepath.ToSlash(path), string(b)) {
				t.Error(v)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestColourScanner(t *testing.T) {
	css := "/* ui-tokens:start */\n:root{--ui-bg:#fff}\n.dark{--ui-bg:oklch(.2 0 0)}\n/* ui-tokens:end */\n#abc{color:#123456;background:rgb(1 2 3);border-color:hsl(10 20% 30%);outline-color:rgba(1,2,3,.5)}\n.x{color:hsl(var(--background))}"
	if got := colourViolations("ui.css", css); len(got) != 4 {
		t.Fatalf("want 4 violations, got %v", got)
	}
	if got := colourViolations("other.css", css); len(got) != 6 {
		t.Fatalf("only ui.css may define literal tokens: %v", got)
	}
}

func TestStylesMatchCSSSources(t *testing.T) {
	for file, want := range map[string]string{"ui.css": Styles, "preset-shadcn.css": ShadcnStyles} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != want {
			t.Errorf("%s differs from its Go constant: GOWORK=off go generate ./...", file)
		}
	}
}

func TestUITokenReferencesDefined(t *testing.T) {
	defs := regexp.MustCompile(`(--ui-[\w-]+)\s*:`)
	refs := regexp.MustCompile(`var\((--ui-[\w-]+)`)
	defined := map[string]bool{}
	for _, m := range defs.FindAllStringSubmatch(Styles, -1) {
		defined[m[1]] = true
	}
	for _, m := range refs.FindAllStringSubmatch(Styles+ShadcnStyles, -1) {
		if !defined[m[1]] {
			t.Errorf("undefined kit token %s", m[1])
		}
	}
}
