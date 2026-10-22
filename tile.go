package ui

import "strconv"

// TileProps is Tile, by its former name.
//
// Deprecated: use Tile: ui.Tile{…}, or <ui.Tile …> in a template.
type TileProps = Tile

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
