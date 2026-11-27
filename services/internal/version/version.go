// Package version reports the build of a binary. Commit and BuiltAt are set at link time:
//
//	go build -ldflags "-X github.com/buzandanalexandru/eps-prototype/services/internal/version.Commit=$SHA"
package version

// Commit is the git commit the binary was built from ("dev" for local builds).
var Commit = "dev"

// BuiltAt is the build time in RFC 3339 ("" for local builds).
var BuiltAt = ""
