package live

// SetWholeTrees makes new sessions render every instance without its
// frames, as components without render tracking render.
func SetWholeTrees(on bool) { wholeTrees = on }
