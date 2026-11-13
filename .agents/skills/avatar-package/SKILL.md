---
name: avatar-package
description: Install, wire and use Arandu Avatar (Go, Arandu) in an application. Use when the request is to "install arandu-avatar", "add avatars", "add the profile picture", "let people upload a photo", "show the avatar in the menu", "put the company logo", "go get github.com/hyz-is/arandu-avatar", "wire it into bootstrap/app.go", "the avatar editor looks unstyled", "no such table avatars", "403 from /avatars", "the photo does not show", "Gravatar", or when a project's go.mod already requires github.com/hyz-is/arandu-avatar. Covers the lines of wiring and where each goes, the Directory the application implements, the one line for the layout, drawing an avatar and the editor, the migration step that is not optional, and the symptoms.
license: MIT
metadata:
  audience: app
---

# Using Arandu Avatar

The avatar of the people of an application, and the logo of its companies'
spaces: a photo they send, their Gravatar if they switch it on, or a drawing made
from their name. It is registered by hand: there is no service provider, no
container and no discovery, so if a line is not written in the application, it does
not happen.

## Install

```bash
go get github.com/hyz-is/arandu-avatar@latest   # check gh release list -R hyz-is/arandu-avatar first
aru migrate
```

## The wiring, and where each part goes

All of it is in `bootstrap/app.go`. `aru make:module` does not edit it.

The imports. The components are imported under another name, because the project
already imports Kyse's `components`:

```go
import (
	avatar "github.com/hyz-is/arandu-avatar"
	avatarui "github.com/hyz-is/arandu-avatar/components"
)
```

The construction, in `Build`, after the session store, the private disk and the
activity log exist and before `k.Register`:

```go
	avatarModule, err := avatar.New(avatar.Config{
		TeamTenant: cfg.Auth.Tenant,
		Directory:  accountDirectory{db: db},
		Files:      disk,
		Activity:   activityModule.Logger(),
		Render:     avatarui.Render, // components.Render
		Middleware: []func(http.Handler) http.Handler{permissionModule.Roles()},
	}, db, sessions)
	if err != nil {
		return App{}, err
	}
```

And the registration, inside the `k.Register(...)` call already there:

```go
		avatarModule,
```

`New` refuses a configuration that cannot work, a nil database and a nil session
store, so a wiring mistake costs one restart.

- **`Directory`** is `avatar.Directory`: `Owner(ctx, g, ref)` answers a user or an
  organization **in the tenant of `g`**, or `avatar.ErrUnknownOwner`. Look the owner
  up with the Grant, never with an identifier from the request.
- **`Files`** is the application's `Disk`; in production the private R2 bucket
  (`FILESYSTEM_DISK=r2`). Nil turns uploads and Gravatar off and the drawing still
  works. Never point it at a public bucket address.
- **`Render`** is `components.Render`: how a change answers an htmx request.
- Mount `arandu-permission`'s `Roles()` in `Middleware`, so the subject carries the
  actions it resolves. Declare `avatar.Actions()` and `avatar.TeamActions()` in the
  permission catalogue; give the team's only to the team's groups.
- `Prefix` and `Paths` (`avatar.PortuguesePaths`) choose the addresses; `Locale`
  `pt-BR` is the default and `en` ships.

## One line in the layout

In the `<head>`, once:

```go
{!! avatar.Assets() !!}
```

It writes the stylesheet and the script from the application's own origin, which the
strict CSP allows. Without it an avatar is drawn but unstyled, and the editor has no
cropper. `AvatarEditor` writes them itself unless `NoAssets`; `AvatarImage` only with
`WithAssets`.

## Draw an avatar

Ask for every person the page draws in one call, and draw them:

```go
pictures, err := avatarModule.Resolve(ctx.Ctx(), who,
	avatar.Owner{Ref: avatar.User(u.ID), Name: u.Name, Email: u.Email})
```

```go
{!! avatarui.AvatarImage(avatarui.AvatarImageProps{Picture: .Pictures.Of(avatar.User(u.ID)), Size: "sm"}) !!}
```

Sizes `xs sm md lg xl 2xl`. `Alt: "-"` when the name is written beside it. For just an
address, `avatarModule.URL(ctx, who, owner, 64)`.

## The editor

```go
editor, err := avatarModule.Editor(ctx.Ctx(), who, avatar.User(who.ID))
```

```go
{!! avatarui.AvatarEditor(avatarui.AvatarEditorProps{Page: .Page, Editor: .AvatarEditor}) !!}
```

`Dialog: true` draws a button with the avatar that opens it in a dialog. With
`avatar.Organization(id)` it is the logo's editor, and it shows nothing to press to
a subject that cannot change it.

## The policy is not open by guesswork

- A person changes **their own** avatar with no group. Anybody signed in to the space
  reads the space's avatars. A guest has none.
- The logo of an organization needs `avatar.manage_organization`. The platform team
  needs `avatar.view_all` and `avatar.manage_all`, in `TeamTenant`.
- The tenant is always the session's Grant. A request for an owner of another space
  is a `404`, the same as one that does not exist.

## Symptoms

| you see | it is |
| --- | --- |
| `no such table`, `relation "avatars" does not exist` | `aru migrate` was not run. It is never run at boot |
| every route redirects to `/auth/login` | no session: every route sits behind `RequireAuth` |
| `403` on the logo | the subject does not hold `avatar.manage_organization`; check `Roles()` is in `Config.Middleware` |
| `404` on a person's editor | `Directory.Owner` answered `ErrUnknownOwner`, or the owner is in another tenant |
| the editor is unstyled, no cropper | `{!! avatar.Assets() !!}` is not in the layout's head, or `ui.js` loads after the script |
| the photo does not appear, the drawing does | the picture route answered an error: check the `Disk` and that the session is valid on this origin |
| the photo shows for the owner and not for others | other members are in another tenant: avatars are scoped to the space |
| "Gravatar não está ligado" | `Config.Chain` has no `avatar.SourceGravatar`, or `Files` is nil |
| the CDN does not keep the pictures | the default is `private`; set `Config.PublicCache` once the CDN is in front |
| a change redirects instead of answering the editor | `Config.Render` is nil |
