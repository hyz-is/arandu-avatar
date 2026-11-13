# Working on Arandu Avatar

This is an Arandu package: the avatar of the people of a multi-tenant application
and the logo of its companies' spaces. One entity with an embedded Hesape Model,
two policies that decide about it (a member's and the platform team's), two
services that own the database handle, the routes that reach them, and two Kyse
components with the script and the stylesheet they need.
It is a Go module somebody `go get`s and registers by hand in their own
`bootstrap/app.go`, which is the whole difference from working in an
application. There is no service provider, no container and no discovery — if a
line of wiring is not written in the installer's repository, it does not happen.

Read `.agents/skills/` before writing code. Each skill is a procedure, and the
one you need is named by the situation you are in.

## The gates

Nothing is finished until all of these exit zero.

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

`aru view:build` compiles `components/*.kyse.go` into the `.go` beside them, which is
committed, because `go get` has no build step: a change to a `.kyse.go` without the
rebuild ships the old component. The last line is the proof the binary stays static,
which the applications that install this package depend on (a distroless image built
with `-tags nodynamic`). If the linker of the Mac fails,
`SDKROOT=$(xcrun --sdk macosx --show-sdk-path)`.

`GOWORK=off` is not borrowed from somewhere else, and here it is not a
preference either. This checkout may sit beside a Go workspace that lists the
framework repositories and does not list this one; when it does, every command
above fails before it compiles anything:

```
pattern ./...: directory prefix . does not contain modules listed in go.work
or their selected dependencies
```

With the workspace off, the module resolves the framework version in `go.mod` —
which is what CI compiles against, and what somebody's `go get` will get.

Both filters on `gofmt` are load-bearing in the toolchain even where this
repository has nothing for them to skip: `gofmt` is the only tool in the chain
that ignores build tags, and `testdata/` is where a fixture is allowed to be
invalid on purpose.

`aru doctor` is not one of the gates, and running it here costs a minute and
answers nothing:

```
this is not an Arandu project: no go.mod, main.go and arandu.toml together.
Run it from inside a project, or create one with `aru new`
```

It exits 1. It reads applications, and this is a library.

It does not read this one after it is installed either, and that is why
`tests/Unit/audit_test.go` exists. The doctor walks the application's own tree,
skips `vendor/`, and opens the one `arandu.mod.toml` at its root; it never loads
a dependency. So `tenant-from-request`, `system-grant-without-tenant` and
`permission-not-declared` never see a line of an installed package, and whatever
this one must prove about itself it proves in its own suite or nowhere.

## What this repository holds

| | measured with |
| --- | --- |
| 18 Go files in one package at the root, one of them the generated query | `ls *.go \| wc -l` |
| 2 Kyse components, one `.kyse.go` source each, compiled beside it | `ls components/*.kyse.go` |
| 1 script and 1 stylesheet, embedded and registered with `view.RegisterAsset` | `ls assets/` |
| 87 tests in 12 files, with a fake Gravatar, a real SQLite and a local Disk | `go test -count=1 ./... -v \| grep -c '^--- PASS'` |
| 16 routes: 12 of a member, 4 of the team | `everyRoute` in `tests/Feature/routes_test.go`, each walked by `TestAVisitorWithNoSessionIsSentToSignIn` |
| 5 actions the policies answer about | `grep -cE '^\t[A-Za-z]+ +security.Action = ' policy.go` |
| 1 table, `avatars`, and its Global twin in `internal/platform` | `migrations.go` |
| 2 internal packages, `art` (eleven SVG styles) and `imaging` (the photo pipeline) | `ls internal` |
| 1 third-party direct dependency, `golang.org/x/image` | `go list -m -f '{{if and (not .Indirect) (not .Main)}}{{.Path}} {{.Version}}{{end}}' all` |

The layout is by role rather than by layer, so the package reads top to bottom:

```
module.go      registration, routes and the contract with the framework
config.go      what the application passes in, and the route table
contracts.go   what the application implements: the directory of owners
model.go       the entity, its own rules, and the owner and source types
picture.go     how an owner is drawn, and the addresses of the photo
policy.go      who may do what, for a member of a space and for the team
service.go     the members' use cases, and Model access after authorization
team.go        the platform team's use cases
editor.go      what the editor is handed
files.go       keeping and removing the photos on the application's Disk
gravatar.go    the one look-up of a Gravatar, made when a person asks for it
grants.go      the one place a Grant is issued without a signed-in member
activity.go    what is recorded in arandu-activitylog
handlers.go    the controllers of both sides
assets.go      the script and the stylesheet
migrations.go  the schema
components/    AvatarImage, AvatarEditor and Render
internal/art, internal/imaging, internal/platform
```

`avatarTable`, in `model.go`, declares the table once. `Avatars(db)` is generated
from it into `AvatarQuery.go`, and its Global twin into
`internal/platform/AvatarQuery.go`; neither is ever edited by hand, and
`aru model:build --check` says when one is stale. Terminals return `*Avatar`; keep
those pointers intact, because copying an embedded Model leaves its `Entity`
pointer aimed at the original allocation.

## What does not exist here

Reaching for one of these is the most common way to write a change that is rejected
in review. None of them is missing by accident.

| A model reaches for | What is here instead |
| --- | --- |
| `gen2brain/heic`, libvips, ImageMagick, cgo, anything that makes the binary dynamic | the standard library for JPEG and PNG and `golang.org/x/image` for WebP and scaling. `TestNothingLinksCOrLeavesTheBinaryDynamic` fails on the others, and the last gate builds with `CGO_ENABLED=0 -tags nodynamic` |
| `<img src="https://gravatar.com/...">` in the page | one server-side request when the person asks, a copy kept and served from here: no visitor's address reaches Gravatar and the CSP needs no change |
| a public URL of the bucket | the route, which asks the policy. Keys are written by the server |
| deciding the type of a file by its name or `Content-Type` | `imaging.Detect`, on the bytes |
| decoding first and checking the size after | `image.DecodeConfig` against `MaxPixels` before `image.Decode` |
| copying the EXIF segment through, or keeping the original | nothing but pixels is encoded, which is what removes the metadata |
| an `<img src="blob:...">` for the preview | `createImageBitmap` onto a canvas: the policy has no `blob:` and needs none |
| an inline `style`, `onclick`, `x-data`, a `<style>` block | a class, a data attribute and the two files served from `self` |
| a second stylesheet cascade | `arandu-avatar.css` is namespaced under `.avatar[data-avatar]` and `.avatar-*`, written against Basecoat's tokens |
| a local table of roles or admins | `arandu-permission` fills the subject's actions; the policy reads them |
| a tenant from the path, the query or a header | `data.Tenant(g)`, from the Grant |
| a permit-all branch in a policy | nothing. A policy denies, and an action is opened by writing the rule that opens it |
| a service provider, a container, a global registry | `New(cfg, db, sessions)`, called by hand in `bootstrap/app.go` |
| a CRUD Repository beside the Model | `Avatars(db)`, reached only by the services after `security.Authorize` |
| a `panic` on bad wiring | an `error` from `New` |
| a second dependency | an argument, first. This module is imported into other people's builds |

## The properties

A change that breaks one of them is not merged, whatever else it improves.

1. **The policies deny by default.** `tests/Unit/policy_test.go` walks a matrix
   of subjects and actions, and a subject with nothing is refused every action.
2. **Every service method authorizes before it reaches the Model.**
   `TestEveryServiceMethodAuthorizesBeforeTheModel` reads the source of
   `AvatarService` and `TeamService`; `reach` and `Resolve` are the deciders it
   trusts, and `TestTheHelpersThatDecideActuallyDecide` holds that they call
   `Authorize`.
3. **The tenant comes from `data.Tenant(g)`**, on every path.
   `TestNoTenantIsReadOutOfTheRequest` and `TestTheTenantIsWrittenOnlyFromTheGrant`
   hold both halves, and `TestTheSpacesAreIsolatedFromEachOther` proves it against
   a real database.
4. **Only the team's service reads across tenants**, through `internal/platform`,
   after `TeamPolicy` answered, and writes through `scopedGrant`, the one system
   Grant of the package (`TestAGrantWithoutAMemberIsIssuedInGrantsGoAlone`).
5. **A photo is what its bytes say, bounded and clean.** Fake bytes, a bomb, an
   oversized file and a photo with GPS are the cases in `imaging_test.go` and
   `avatar_test.go`.
6. **`arandu.mod.toml` matches the code.** `network = true` for the one Gravatar
   request, `migrations = true`, the rest `false`;
   `TestTheDeclaredCapabilitiesAreWhatTheCodeDoes` compares them with what the code
   calls.
7. **No inline script or style, in the markup or in a drawing.**
   `TestTheEditorHasNoScriptNoStyleAndNoHandler` and
   `TestEveryStyleIsWellFormedSVGWithNothingACSPWouldBlock`.

What the audit does not reach is written at the top of the file it lives in. It
reads syntax, so dynamic dispatch, reflection and wrappers around the named seams
are invisible to it. A green run means no such thing was found written down, not
that none exists.

## Writing code

Everything in the source is in English: identifiers, doc comments, internal
comments, error messages, log messages, and the names and messages of tests.
`pkg.go.dev` publishes the doc comments and its readers are users of this
package.

Every exported symbol carries a doc comment, and the comment documents the
symbol and nothing else. Why a signature is what it is belongs there when it is
a fact about the code — *"the value is held because Go does not build a type
from a string"* stays. A date, an issue number, a version in progress or the
name of another repository does not.

Tests go under `tests/`, in a capitalized category directory declaring a
lowercase external package: `tests/Unit` holds `package unit_test`,
`tests/Feature` holds `package feature_test`. Both import the package by its
module path, which is what makes them see exactly what a caller sees. A test
that genuinely needs something unexported goes beside the code as
`*_internal_test.go`, and the suffix is how it says so.
