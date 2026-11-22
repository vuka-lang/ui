module github.com/vuka-lang/ui/v2

go 1.27.3

require (
	github.com/a-h/templ v0.3.1020
	github.com/vuka-lang/orm/v2 v2.18.0
	github.com/vuka-lang/vuka/v2 v2.24.1-0.20261121193906-3d211f061ff8
	golang.org/x/net v0.61.0
)

require (
	github.com/a-h/parse v0.0.0-20250122154542-74294addb73e // indirect
	github.com/andybalholm/brotli v1.2.7 // indirect
	github.com/evanw/esbuild v0.28.2 // indirect
	github.com/vuka-lang/vuka/cmd/vuka/v2 v2.24.1-0.20261121193932-97941e6e7339 // indirect
	github.com/vuka-lang/vuka/cmd/vukavet/v2 v2.23.0 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/tools v0.52.0 // indirect
)

tool (
	github.com/vuka-lang/vuka/cmd/vuka/v2
	github.com/vuka-lang/vuka/cmd/vukavet/v2
)
