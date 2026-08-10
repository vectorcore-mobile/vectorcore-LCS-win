// Package version holds the app's build version. Version is a var, not
// a const, so build.ps1 can stamp a real value via -ldflags -X; the
// literal below is only what a plain `go build` without that flag falls
// back to.
package version

var Version = "0.0.1d"
