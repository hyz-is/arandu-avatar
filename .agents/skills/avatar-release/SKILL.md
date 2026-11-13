---
name: avatar-release
description: Get Arandu Avatar green, declared and published. Use when the request is to "run the gates", "check it builds", "the tests fail", "build static", "update arandu.mod.toml", "add a dependency", "bump the framework", "tag a release", "publish it", "write the changelog", or before opening a pull request. Covers the gates and why each filter is there, the static build, what the manifest declares and who checks it, the immutability of a published module version, and what breaks somebody else's build.
license: MIT
---

# Getting it green and out

## The gates

```sh
export GOWORK=off GOTOOLCHAIN=auto
aru view:build
aru model:build --check
gofmt -l $(find . -name '*.go' -not -path '*/testdata/*' -not -name '*.kyse.go')
go build ./...
go vet ./...
go test -race ./...
CGO_ENABLED=0 go build -tags nodynamic ./...
```

`GOWORK=off` because the checkout may sit beside a workspace that does not list it,
and then every command fails before it compiles anything. `aru view:build` compiles
`components/*.kyse.go` into the `.go` beside them, which is committed: `go get` has
no build step, so a change to a `.kyse.go` without the rebuild ships the old
component. If the Mac's linker fails, `SDKROOT=$(xcrun --sdk macosx --show-sdk-path)`.

The last line is the proof the binary stays static: the applications that install this
package ship on a distroless image built with `-tags nodynamic`.

`aru doctor` is not a gate: it reads applications and exits 1 here.

## What the manifest declares, and who reads it

```toml
name = "hyz-is/avatar"
framework = ">= 0.67"
profiles = ["conventional", "performance"]

[permissions]
network = true      # one request to Gravatar, when a person asks
filesystem = false  # photos go to the application's Disk
exec = false
migrations = true   # the avatars table
```

`TestTheDeclaredCapabilitiesAreWhatTheCodeDoes` compares the four with what the code
*calls*, by syntax. The `framework` line is a floor and a second declaration of what
`go.mod` says: move both together.

## Adding a dependency

Do not, without an argument. The only third-party module is `golang.org/x/image`,
for WebP decoding and scaling, and it is pure Go. Nothing that links C, nothing that
makes the binary dynamic.

## Publishing

A published module version is immutable; a mistake is corrected by another release,
never by moving a tag. Before tagging: the gates pass, `CHANGELOG.md` has the entry
under the version (the actions in backticks and the migration by name), `UPGRADE.md`
has the notes, and `arandu.mod.toml` matches the code. While the version starts with
`v0.`, breaking is allowed; breaking quietly is not. A route name, a migration name
and a default are the three changes that break an installer without touching a
signature: say each in the changelog.

Branches follow the other `hyz-is` modules: `main` is released, and the tag `vX.Y.Z`
on it triggers `release.yml`, which publishes the changelog entry.
