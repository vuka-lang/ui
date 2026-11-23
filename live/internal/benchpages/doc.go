// Package benchpages is the pages the live package's render benchmarks run,
// compiled by the Vuka compiler: a users table of row components, a grid of
// small components, a form in a dialog, and a feed taking broadcasts. The
// *_html.go files are generated: vuka gen -inplace ./live/internal/benchpages
package benchpages

import (
	"strconv"
)

// User is a row of the users table.
type User struct {
	ID    int
	Name  string
	Email string
	Role  string
}

// Users is n users, ids from 1.
func Users(n int) []User {
	roles := []string{"admin", "editor", "viewer"}
	out := make([]User, n)
	for i := range out {
		s := strconv.Itoa(i + 1)
		out[i] = User{ID: i + 1, Name: "User " + s, Email: "user" + s + "@example.com", Role: roles[i%3]}
	}
	return out
}

// RoleChanged is the broadcast the feed takes: a user's new role.
type RoleChanged struct {
	ID   int
	Role string
}

// NextRole is the role a promotion gives.
func NextRole(r string) string {
	switch r {
	case "viewer":
		return "editor"
	case "editor":
		return "admin"
	}
	return "viewer"
}

func variantOf(role string) string {
	switch role {
	case "admin":
		return "danger"
	case "editor":
		return "primary"
	}
	return "default"
}

func itoa(n int) string { return strconv.Itoa(n) }
