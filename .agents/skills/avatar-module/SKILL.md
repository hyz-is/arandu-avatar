---
name: avatar-module
description: Change what Arandu Avatar registers -- its routes, handlers, configuration, response shape, schema or the pipeline a photo goes through. Use when the request is to "add a route", "add a handler", "add a config option", "change the prefix", "add a column", "write a migration", "add a generated style", "change the sizes", "change how the photo is cut", "regenerate the query", "return 404", or when a change touches module.go, config.go, handlers.go, model.go, service.go, AvatarQuery.go, internal/art or internal/imaging. Covers foundation.Module, thin handlers, RequireAuth and ctx.User, returning errors for the router to answer, the generated query and aru model:build, the route table, the migration name that carries the order, and where the drawings and the image pipeline live.
license: MIT
---

# The module contract

`foundation.Module` is `Name()` and `Routes()`, and nothing else; this package also
implements `foundation.Migratable`. Keep the compile-time proof at the top of
`module.go`:

```go
var (
	_ foundation.Module     = (*Module)(nil)
	_ foundation.Migratable = (*Module)(nil)
)
```

## Adding a route

Routes are a table, `routeTable(Config)` in `config.go`: name, method, path, side,
kind and action. `validateRoutes` registers the same table on a throwaway mux, so a
pattern that does not parse or two that collide is the error `New` returns. Add the
line there, then the case to `handler` in `handlers.go`, then the address to
`everyRoute` in `tests/Feature/routes_test.go` -- it must answer the sign-in
redirect with no session. The prefix is `cfg.Prefix` and never a literal, and no
route lives under `/_arandu`.

`register` mounts `RequireAuth` once, on the group, before the first route.
`TestHandlersLeaveTheSubjectAndTheStatusToTheFramework` fails otherwise.

## A handler is thin

Read the input and who is asking, ask the service, answer. `who, _ := ctx.User()`;
no handler loads a session, holds the database or starts a query (`Avatars(...)` in a
handler fails the structure test). Return the error and the router answers it:
`validation.Errors` goes back with the messages, `ErrNotFound` is 404, a policy
refusal 403. The one place that looks at an error is `answer`, because an htmx
request cannot follow a redirect into a fragment: it answers the editor again with
the messages in it.

## Changing the Model

`Avatar` embeds `model.Model`; `avatarTable` declares the table once. After changing
the struct or the table run `aru model:build` and commit what it rewrote
(`AvatarQuery.go` and `internal/platform/AvatarQuery.go`); `aru model:build --check`
exits 1 when either is stale. Rows are pointers after `New`, `First`, `Get`: a copy
keeps the model of the row it came from and `Save` refuses it.

The rules of the entity (`SetUpload`, `ClearGravatar`...) are pure methods in the
custom block. `internal/platform` is the same table, `Global`, read only after
`TeamPolicy` answered; keep its columns in step with the entity's and never let its
rows leave `team.go`.

## Adding a migration

Append to `Migrations()`, with a name that sorts after the last one, and never change
the name of one that shipped. Types that spell the same in SQLite, PostgreSQL and
MySQL; no database defaults on timestamps; a new column is nullable or has a default,
because the previous binary runs during a rollout. Migrations do not run at boot.

## A configuration field

A doc comment on the field; a rule in `Validate` for every value it cannot be (a
value out of range is refused, never clamped); a default in `withDefaults` as a
constant beside the others. Add the bad value to `TestTheConfigurationRefusesWhatCannotWork`.

## A drawing

`internal/art` draws deterministically from `Seed`: every number comes from the
stream of `drawing.next`, so the same seed is the same picture, and no style reads a
clock, a map order or the name (except `initials`). Rules that keep it safe: only
validated colours are written into the markup; the name goes through
`html.EscapeString`; no `style` attribute, no script, no `href`; a gradient id is
derived from the seed. Add the style to `Styles()`, to `Render`, to both catalogues
(`style.<name>`) and the tests iterate it for you.

## The image pipeline

`internal/imaging` is the whole of it: bytes in, variants out. Keep it standard
library plus `golang.org/x/image`. Never add cgo or a package that links C
(`TestNothingLinksCOrLeavesTheBinaryDynamic`), and never decode before the header was
held against `MaxPixels`. Whatever it writes is built from pixels alone, which is
what strips the metadata; do not copy a segment through.
