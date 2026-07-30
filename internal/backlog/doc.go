// Package backlog holds repository-hygiene checks for the three ranked backlog
// documents at the repo root: bugs.md, improvements.md and
// improvements_paywall.md.
//
// It has no runtime code and is never imported. Everything here is a test,
// deliberately placed inside a package that "go test ./..." walks, because the
// defect these checks exist to prevent is precisely a claim that every session
// believed it had verified and none actually had. A script nobody runs would
// reproduce that bug rather than fix it.
package backlog
