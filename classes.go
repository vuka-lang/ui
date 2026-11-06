package ui

import (
	"slices"
	"strings"
	"sync"
	"sync/atomic"
)

// MergeClasses joins class lists, a later one winning where two Tailwind
// utilities set the same thing: MergeClasses("px-4 py-2 text-sm", "px-2")
// is "py-2 text-sm px-2". Utilities conflict within their group — padding,
// margin, sizing, colours, display, position, typography, borders, flex and
// grid, effects — and only under the same variants: hover:bg-red-500 and
// bg-blue-500 both stay. A side refines a whole: px-2 after p-4 keeps both,
// p-4 after px-2 drops px-2. Classes that aren't Tailwind's (vk-alert,
// an app's own) are always kept.
//
// A component taking a Class merges it last, so a caller's class wins over
// the component's own.
func MergeClasses(classes ...string) string {
	var s string
	switch n := len(classes); {
	case n == 0:
		return ""
	case n == 1:
		s = classes[0]
	case n == 2 && classes[1] == "":
		s = classes[0]
	case n == 2 && classes[0] == "":
		s = classes[1]
	default:
		s = strings.Join(classes, " ")
	}
	if v, ok := merged.Load(s); ok {
		return v.(string)
	}
	out := mergeClasses(s)
	if mergedN.Add(1) > mergedMax {
		merged.Clear()
		mergedN.Store(0)
	}
	merged.Store(s, out)
	return out
}

// merged caches MergeClasses: a component's classes repeat render after
// render. It is cleared once it holds mergedMax lists.
var (
	merged    sync.Map
	mergedN   atomic.Int64
	mergedMax = int64(4096)
)

// classKey is what a class sets: its utility's group under its variants.
type classKey struct {
	mods, group string
	important   bool
}

type classTok struct {
	s   string
	key classKey // key.group is "" for a class merged with nothing
}

// groupSet is the groups taken under one set of variants.
type groupSet struct {
	mods      string
	important bool
	bits      [4]uint64
}

func (g *groupSet) has(id int) bool { return g.bits[id>>6]&(1<<(id&63)) != 0 }
func (g *groupSet) add(id int)      { g.bits[id>>6] |= 1 << (id & 63) }

func mergeClasses(s string) string {
	toks := make([]classTok, 0, 16)
	for c := range strings.FieldsSeq(s) {
		toks = append(toks, classTok{s: c, key: classKeyOf(c)})
	}
	var sets [4]groupSet
	taken := sets[:0]
	var props []classKey // arbitrary properties taken
	keep := make([]bool, len(toks))
	n := 0
	for i := len(toks) - 1; i >= 0; i-- {
		k := toks[i].key
		if id, ok := groupIDs[k.group]; !ok && k.group != "" {
			if slices.Contains(props, k) {
				continue
			}
			props = append(props, k)
		} else if ok {
			j := 0
			for j < len(taken) && (taken[j].mods != k.mods || taken[j].important != k.important) {
				j++
			}
			if j == len(taken) {
				taken = append(taken, groupSet{mods: k.mods, important: k.important})
			}
			if taken[j].has(id) {
				continue
			}
			taken[j].add(id)
			for _, c := range conflictIDs[id] {
				taken[j].add(c)
			}
		}
		keep[i], n = true, n+1
	}
	if n == len(toks) && !strings.ContainsAny(s, "\t\n\r") && !strings.Contains(s, "  ") && s == strings.TrimSpace(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i, t := range toks {
		if keep[i] {
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(t.s)
		}
	}
	return b.String()
}

// classKeyOf is what c sets: its variants (sorted where their order doesn't
// matter), whether it is important, and its utility's group, "" for a
// class that isn't one.
func classKeyOf(c string) classKey {
	start, depth, colons := 0, 0, 0
	for i := 0; i < len(c); i++ {
		switch c[i] {
		case '[', '(':
			depth++
		case ']', ')':
			depth--
		case ':':
			if depth == 0 {
				start, colons = i+1, colons+1
			}
		}
	}
	base := c[start:]
	k := classKey{mods: c[:start]}
	if strings.HasPrefix(base, "!") {
		base, k.important = base[1:], true
	} else if strings.HasSuffix(base, "!") {
		base, k.important = base[:len(base)-1], true
	}
	if k.group = utilityGroup(strings.TrimPrefix(base, "-")); k.group == "" {
		return classKey{}
	}
	if colons > 1 {
		ms := strings.Split(strings.TrimSuffix(k.mods, ":"), ":")
		sortModifiers(ms)
		k.mods = strings.Join(ms, ":")
	}
	return k
}

// sortModifiers sorts variants that commute: those between two arbitrary
// ones ([&>*]:), whose place matters.
func sortModifiers(ms []string) {
	from := 0
	for i := 0; i <= len(ms); i++ {
		if i == len(ms) || strings.HasPrefix(ms[i], "[") {
			slices.Sort(ms[from:i])
			from = i + 1
		}
	}
}

// utilityGroup is the group of a Tailwind utility (without its variants),
// "" for a class that isn't one.
func utilityGroup(base string) string {
	if base == "" {
		return ""
	}
	if g, ok := exactClasses[base]; ok {
		return g
	}
	if base[0] == '[' {
		if p, _, ok := strings.Cut(base[1:], ":"); ok && p != "" {
			return "[" + p + "]" // an arbitrary property: [mask-type:alpha]
		}
		return ""
	}
	end := len(base)
	if i := strings.IndexAny(base, "[("); i >= 0 {
		end = i
	}
	best := -1
	var classify classifier
	for i := 0; i <= end; i++ {
		if i < end && base[i] != '-' {
			continue
		}
		if f, ok := classPrefixes[base[:i]]; ok {
			best, classify = i, f
		}
	}
	if best < 0 {
		return ""
	}
	value := ""
	if best < len(base) {
		value = base[best+1:]
	}
	return classify(value)
}

type classifier func(value string) string

// group classifies every value of a prefix as g; alone says whether the
// prefix on its own (border, rounded) is a utility too.
func utility(g string, alone bool) classifier {
	groupID(g)
	return func(v string) string {
		if v == "" && !alone {
			return ""
		}
		return g
	}
}

func classSet(vs ...string) map[string]bool {
	m := make(map[string]bool, len(vs))
	for _, v := range vs {
		m[v] = true
	}
	return m
}

// isTshirt reports a size of the theme's scale: xs, sm, md, lg, xl, 2xl…
func isTshirt(v string) bool {
	switch v {
	case "xs", "sm", "md", "lg", "xl", "base":
		return true
	}
	if n := strings.TrimSuffix(v, "xl"); n != v {
		return n != "" && strings.Trim(n, "0123456789") == ""
	}
	if n := strings.TrimSuffix(v, "xs"); n != v {
		return n != "" && strings.Trim(n, "0123456789") == ""
	}
	return false
}

func isNumeral(v string) bool {
	return v != "" && strings.Trim(v, "0123456789.") == ""
}

// arbitraryValue is an arbitraryValue value's label and body: [length:2px], [2px].
func arbitraryValue(v string) (label, body string, ok bool) {
	if len(v) < 2 || (v[0] != '[' || v[len(v)-1] != ']') && (v[0] != '(' || v[len(v)-1] != ')') {
		return "", "", false
	}
	body = v[1 : len(v)-1]
	if l, b, ok := strings.Cut(body, ":"); ok && !strings.ContainsAny(l, "(#") {
		return l, b, true
	}
	return "", body, true
}

// isClassLength reports a width or length: 2, px, [3px], [length:var(--w)].
func isClassLength(v string) bool {
	if v == "px" || isNumeral(v) {
		return true
	}
	l, body, ok := arbitraryValue(v)
	if !ok {
		return false
	}
	if l != "" {
		return l == "length" || l == "size" || l == "number"
	}
	return body != "" && (body[0] >= '0' && body[0] <= '9' || body[0] == '.' || strings.HasPrefix(body, "calc(") || strings.HasPrefix(body, "min(") || strings.HasPrefix(body, "max(") || strings.HasPrefix(body, "clamp("))
}

var (
	textAligns   = classSet("left", "center", "right", "justify", "start", "end")
	textWraps    = classSet("wrap", "nowrap", "balance", "pretty")
	fontWeights  = classSet("thin", "extralight", "light", "normal", "medium", "semibold", "bold", "extrabold", "black")
	borderStyles = classSet("solid", "dashed", "dotted", "double", "hidden", "none")
	bgPositions  = classSet("bottom", "center", "left", "left-bottom", "left-top", "right", "right-bottom", "right-top", "top")
	bgRepeats    = classSet("repeat", "no-repeat", "repeat-x", "repeat-y", "repeat-round", "repeat-space")
	alignments   = classSet("normal", "start", "end", "center", "between", "around", "evenly", "stretch", "baseline", "auto")
	shadows      = classSet("2xs", "xs", "sm", "md", "lg", "xl", "2xl", "inner", "none")
)

func textGroup(v string) string {
	size, _, _ := strings.Cut(v, "/")
	switch {
	case isTshirt(size):
		return "font-size"
	case textAligns[v]:
		return "text-align"
	case textWraps[v]:
		return "text-wrap"
	case v == "ellipsis" || v == "clip":
		return "text-overflow"
	}
	if l, body, ok := arbitraryValue(size); ok && (l == "length" || l == "" && isClassLength("["+body+"]")) {
		return "font-size"
	}
	return "text-color"
}

func fontGroup(v string) string {
	if fontWeights[v] || isNumeral(v) {
		return "font-weight"
	}
	if l, _, ok := arbitraryValue(v); ok && l == "number" {
		return "font-weight"
	}
	return "font-family"
}

func bgGroup(v string) string {
	switch {
	case v == "fixed" || v == "local" || v == "scroll":
		return "bg-attachment"
	case bgPositions[v]:
		return "bg-position"
	case bgRepeats[v]:
		return "bg-repeat"
	case v == "auto" || v == "cover" || v == "contain":
		return "bg-size"
	case v == "none":
		return "bg-image"
	}
	if l, body, ok := arbitraryValue(v); ok && (l == "url" || l == "image" || strings.HasPrefix(body, "url(") || strings.Contains(body, "gradient(")) {
		return "bg-image"
	}
	return "bg-color"
}

// sidedUtility is a classifier for a property with sides — border-x-2,
// border-red-500 — of kind: width (a length, or the prefix alone), style
// or colour.
func sidedUtility(width, style, color string) classifier {
	groupID(width)
	if style != "" {
		groupID(style)
	}
	groupID(color)
	return func(v string) string {
		switch {
		case v == "" || isClassLength(v):
			return width
		case style != "" && borderStyles[v]:
			return style
		}
		return color
	}
}

func shadowGroup(name string) classifier {
	color := name + "-color"
	groupID(name)
	groupID(color)
	return func(v string) string {
		if v == "" || shadows[v] {
			return name
		}
		if _, body, ok := arbitraryValue(v); ok && strings.ContainsAny(body, "_ ") {
			return name
		}
		return color
	}
}

func flexGroup(v string) string {
	switch v {
	case "row", "row-reverse", "col", "col-reverse":
		return "flex-direction"
	case "wrap", "wrap-reverse", "nowrap":
		return "flex-wrap"
	}
	return "flex"
}

func gradientStop(name string) classifier {
	pos := name + "-pos"
	groupID(name)
	groupID(pos)
	return func(v string) string {
		if strings.HasSuffix(v, "%") {
			return pos
		}
		return name
	}
}

// classPrefixes are the utilities taking a value, by the name before it.
var classPrefixes = map[string]classifier{}

// exactClasses are the utilities written without a value, and their group.
var exactClasses = map[string]string{
	"sr-only": "sr", "not-sr-only": "sr",
	"static": "position", "fixed": "position", "absolute": "position", "relative": "position", "sticky": "position",
	"visible": "visibility", "invisible": "visibility", "collapse": "visibility",
	"italic": "font-style", "not-italic": "font-style",
	"uppercase": "text-transform", "lowercase": "text-transform", "capitalize": "text-transform", "normal-case": "text-transform",
	"underline": "text-decoration", "overline": "text-decoration", "line-through": "text-decoration", "no-underline": "text-decoration",
	"truncate":    "text-overflow",
	"antialiased": "font-smoothing", "subpixel-antialiased": "font-smoothing",
	"isolate": "isolation", "isolation-auto": "isolation",
	"box-border": "box-sizing", "box-content": "box-sizing",
	"border-collapse": "border-collapse", "border-separate": "border-collapse",
	"grow": "grow", "shrink": "shrink",
	"container":  "container",
	"ring-inset": "ring-inset",
	"transform":  "transform", "transform-gpu": "transform", "transform-cpu": "transform", "transform-none": "transform",
	"outline": "outline-style", "outline-hidden": "outline-style",
}

func init() {
	for _, d := range []string{"block", "inline-block", "inline", "flex", "inline-flex", "table", "inline-table", "table-caption", "table-cell",
		"table-column", "table-column-group", "table-footer-group", "table-header-group", "table-row-group", "table-row",
		"flow-root", "grid", "inline-grid", "contents", "list-item", "hidden"} {
		exactClasses[d] = "display"
	}
	for _, p := range []string{"p", "px", "py", "ps", "pe", "pt", "pr", "pb", "pl", "m", "mx", "my", "ms", "me", "mt", "mr", "mb", "ml",
		"gap", "gap-x", "gap-y", "space-x", "space-y", "w", "min-w", "max-w", "h", "min-h", "max-h", "size",
		"inset", "inset-x", "inset-y", "start", "end", "top", "right", "bottom", "left", "z", "order", "basis", "grow", "shrink",
		"grid-cols", "grid-rows", "col-start", "col-end", "row-start", "row-end", "grid-flow", "auto-cols", "auto-rows",
		"justify-items", "justify-self", "items", "self", "place-content", "place-items", "place-self",
		"leading", "tracking", "whitespace", "line-clamp", "indent", "align", "underline-offset", "list-image",
		"bg-clip", "bg-origin", "bg-blend", "mix-blend", "opacity", "ring-offset", "outline-offset", "overflow", "overflow-x", "overflow-y",
		"overscroll", "overscroll-x", "overscroll-y", "object", "aspect", "columns", "break-before", "break-after",
		"break-inside", "duration", "ease", "delay", "animate", "origin", "scale", "scale-x", "scale-y", "rotate",
		"translate-x", "translate-y", "skew-x", "skew-y", "cursor", "select", "pointer-events", "resize", "appearance",
		"accent", "caret", "will-change", "content", "blur", "brightness", "contrast", "grayscale", "invert", "saturate",
		"sepia", "hue-rotate", "drop-shadow", "backdrop-blur", "backdrop-brightness", "backdrop-opacity", "fill",
		"table-layout", "caption", "float", "clear", "text-shadow", "border-spacing", "touch", "snap", "scroll-m", "scroll-p"} {
		classPrefixes[p] = utility(p, false)
	}
	for _, p := range []string{"grow", "shrink", "rounded", "rounded-s", "rounded-e", "rounded-t", "rounded-r", "rounded-b", "rounded-l",
		"rounded-ss", "rounded-se", "rounded-ee", "rounded-es", "rounded-tl", "rounded-tr", "rounded-br", "rounded-bl", "transition", "blur", "drop-shadow", "grayscale", "invert", "sepia", "resize"} {
		classPrefixes[p] = utility(p, true)
	}
	classPrefixes["flex"] = flexGroup
	classPrefixes["col"] = utility("col-start-end", false)
	classPrefixes["row"] = utility("row-start-end", false)
	classPrefixes["justify"] = utility("justify-content", false)
	classPrefixes["content"] = func(v string) string {
		if alignments[v] {
			return "align-content"
		}
		return "content"
	}
	classPrefixes["object"] = func(v string) string {
		switch v {
		case "contain", "cover", "fill", "none", "scale-down":
			return "object-fit"
		}
		return "object-position"
	}
	classPrefixes["list"] = func(v string) string {
		if v == "inside" || v == "outside" {
			return "list-position"
		}
		return "list-style"
	}
	classPrefixes["break"] = func(v string) string { return "word-break" }
	classPrefixes["text"] = textGroup
	classPrefixes["font"] = fontGroup
	classPrefixes["bg"] = bgGroup
	for _, p := range []string{"bg-gradient-to", "bg-linear", "bg-radial", "bg-conic"} {
		classPrefixes[p] = utility("bg-image", false)
	}
	classPrefixes["from"], classPrefixes["via"], classPrefixes["to"] = gradientStop("from"), gradientStop("via"), gradientStop("to")
	classPrefixes["border"] = sidedUtility("border-w", "border-style", "border-color")
	for _, s := range []string{"x", "y", "s", "e", "t", "r", "b", "l"} {
		classPrefixes["border-"+s] = sidedUtility("border-w-"+s, "", "border-color-"+s)
	}
	classPrefixes["divide"] = sidedUtility("divide-style", "divide-style", "divide-color")
	classPrefixes["divide-x"] = sidedUtility("divide-x", "", "divide-x")
	classPrefixes["divide-y"] = sidedUtility("divide-y", "", "divide-y")
	classPrefixes["outline"] = sidedUtility("outline-w", "outline-style", "outline-color")
	classPrefixes["ring"] = sidedUtility("ring-w", "", "ring-color")
	classPrefixes["ring-offset"] = sidedUtility("ring-offset-w", "", "ring-offset-color")
	classPrefixes["stroke"] = sidedUtility("stroke-w", "", "stroke")
	classPrefixes["decoration"] = func(v string) string {
		switch {
		case v == "auto" || v == "from-font" || isClassLength(v):
			return "decoration-thickness"
		case v == "wavy" || borderStyles[v]:
			return "decoration-style"
		}
		return "decoration-color"
	}
	classPrefixes["shadow"] = shadowGroup("shadow")
	classPrefixes["inset-shadow"] = shadowGroup("inset-shadow")
	classPrefixes["space-x-reverse"], classPrefixes["space-y-reverse"] = utility("space-x-reverse", true), utility("space-y-reverse", true)
}

// groupIDs number the groups for mergeClasses' sets, and conflictIDs are
// classConflicts by number. An arbitrary property ([mask-type:alpha]) has
// none: mergeClasses compares those by name.
var (
	groupIDs    = map[string]int{}
	conflictIDs [][]int
)

// literalGroups are the groups classifiers return by name.
var literalGroups = []string{"align-content", "bg-attachment", "bg-color", "bg-image", "bg-position", "bg-repeat", "bg-size",
	"content", "decoration-color", "decoration-style", "decoration-thickness", "flex", "flex-direction", "flex-wrap",
	"font-family", "font-size", "font-weight", "list-position", "list-style", "object-fit", "object-position",
	"text-align", "text-color", "text-overflow", "text-wrap", "word-break"}

// groupID is g's number, given when first asked: only while the package
// initializes.
func groupID(g string) int {
	if n, ok := groupIDs[g]; ok {
		return n
	}
	groupIDs[g] = len(groupIDs)
	conflictIDs = append(conflictIDs, nil)
	return groupIDs[g]
}

func init() {
	for _, g := range literalGroups {
		groupID(g)
	}
	for _, g := range exactClasses {
		groupID(g)
	}
	for g, cs := range classConflicts {
		n := groupID(g)
		for _, c := range cs {
			conflictIDs[n] = append(conflictIDs[n], groupID(c))
		}
	}
	if len(groupIDs) > 64*len(groupSet{}.bits) {
		panic("ui: more class groups than a groupSet holds")
	}
}

// classConflicts are the groups a utility of a group overrides besides its
// own: a whole over its sides.
var classConflicts = map[string][]string{
	"overflow":   {"overflow-x", "overflow-y"},
	"overscroll": {"overscroll-x", "overscroll-y"},
	"inset":      {"inset-x", "inset-y", "start", "end", "top", "right", "bottom", "left"},
	"inset-x":    {"right", "left"},
	"inset-y":    {"top", "bottom"},
	"flex":       {"basis", "grow", "shrink"},
	"gap":        {"gap-x", "gap-y"},
	"p":          {"px", "py", "ps", "pe", "pt", "pr", "pb", "pl"},
	"px":         {"pr", "pl"},
	"py":         {"pt", "pb"},
	"m":          {"mx", "my", "ms", "me", "mt", "mr", "mb", "ml"},
	"mx":         {"mr", "ml"},
	"my":         {"mt", "mb"},
	"size":       {"w", "h"},
	"font-size":  {"leading"},
	"line-clamp": {"display", "overflow"},
	"rounded": {"rounded-s", "rounded-e", "rounded-t", "rounded-r", "rounded-b", "rounded-l",
		"rounded-ss", "rounded-se", "rounded-ee", "rounded-es", "rounded-tl", "rounded-tr", "rounded-br", "rounded-bl"},
	"rounded-s":  {"rounded-ss", "rounded-es"},
	"rounded-e":  {"rounded-se", "rounded-ee"},
	"rounded-t":  {"rounded-tl", "rounded-tr"},
	"rounded-r":  {"rounded-tr", "rounded-br"},
	"rounded-b":  {"rounded-br", "rounded-bl"},
	"rounded-l":  {"rounded-tl", "rounded-bl"},
	"border-w":   {"border-w-x", "border-w-y", "border-w-s", "border-w-e", "border-w-t", "border-w-r", "border-w-b", "border-w-l"},
	"border-w-x": {"border-w-r", "border-w-l"},
	"border-w-y": {"border-w-t", "border-w-b"},
	"border-color": {"border-color-x", "border-color-y", "border-color-s", "border-color-e",
		"border-color-t", "border-color-r", "border-color-b", "border-color-l"},
	"border-color-x": {"border-color-r", "border-color-l"},
	"border-color-y": {"border-color-t", "border-color-b"},
	"scale":          {"scale-x", "scale-y"},
}
