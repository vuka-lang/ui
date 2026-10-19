package ui

import "strconv"

// Tile is a dashboard stat card: an icon badge beside a big number and its
// label:
//
//	<ui.Tile label="Applications" icon="file-text" value={int64(12840)}/>
//
// Lay a row of them out with TileGrid.
type Tile struct {
	// Label is the caption under the value.
	Label string
	// Icon is the named icon shown in the tile's badge, e.g. "briefcase".
	Icon string
	// Value is the figure. A whole number is shown thousands-separated; any
	// other value is shown as is.
	Value any
	// Variant tints the icon badge; Primary by default.
	Variant Variant
	Class   string
}

// TileProps is Tile, by its former name.
//
// Deprecated: use Tile: ui.Tile{…}, or <ui.Tile …> in a template.
type TileProps = Tile

func (t Tile) badgeClass() string {
	variant := t.Variant
	if variant == "" {
		variant = Primary
	}
	return "vk-tile__icon vk-tile__icon--" + string(variant)
}

// TileGrid lays tiles out in a responsive grid (four across on a wide screen,
// collapsing to two then one):
//
//	ui.TileGrid(tileA, tileB, tileC, tileD)
func TileGrid(tiles ...Node) Node {
	return El("div", []Attr{{"class", "vk-tile-grid"}}, tiles...)
}

// tileValue formats a tile's value: a whole number thousands-separated, any
// other value through Text's formatting.
func tileValue(v any) string {
	switch n := v.(type) {
	case int:
		return intComma(int64(n))
	case int64:
		return intComma(n)
	case int32:
		return intComma(int64(n))
	case uint:
		return intComma(int64(n))
	case uint64:
		return intComma(int64(n))
	case uint32:
		return intComma(int64(n))
	}
	return toString(v)
}

// intComma groups n into thousands with commas: 12840 → "12,840".
func intComma(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := false
	if len(s) > 0 && s[0] == '-' {
		neg, s = true, s[1:]
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if neg {
		return "-" + s
	}
	return s
}
