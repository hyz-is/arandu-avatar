<p align="center">
  <img src=".github/logo.svg" alt="Arandu" width="180">
</p>

# Arandu Avatar

The avatar of the people of a multi-tenant [Arandu](https://github.com/arandu-io/framework)
application, and the logo of its companies' spaces.

A person has a picture the way they have a name. They send a photo and frame the
square that shows, or switch on their Gravatar, or choose a drawing made from
their name -- initials, a block pattern, a face, marble, pixels, rings, a horizon,
Bauhaus shapes, a polygon, a silhouette -- and the application draws it anywhere:
the side menu, the top bar, a list of people, with one call. The photo is checked
by its bytes, cut, scaled and encoded again without a byte of metadata, kept on
the application's private disk and served by a route of this package, at an
address that changes when the photo does, so the CDN in front of the application
keeps it.

```sh
go get github.com/hyz-is/arandu-avatar@latest
aru migrate
```

## What it does

| | |
| --- | --- |
| **Three sources, in a chain you set** | the photo the person sent, then their Gravatar when they switched it on, then a drawing. The drawing is the one source that cannot fail, so an avatar is never empty |
| **A photo** | up to 5 MB; JPEG, PNG or WebP by their bytes, never by the name or the announced type; a square the person framed, with preview; scaled to three sizes; encoded again, which removes EXIF and GPS; kept on the application's `Disk` under keys the server generates |
| **Gravatar** | only when the person switches it on; the SHA-256 of the e-mail is the only thing that leaves; `d=404`, so an e-mail with no Gravatar is not a stand-in picture kept as if it were theirs; the copy is kept here and served from here |
| **Eleven drawings** | deterministic SVG made from the owner's identifier: no request, no stored file, no layout shift |
| **Two kinds of owner** | a person (their identity) and an organization (the logo of a company's space) |
| **A policy and an audit trail** | the person changes their own; the people who run a space change its logo; the platform team changes any. Every change and removal goes to `arandu-activitylog` |
| **Two Kyse components** | `AvatarImage` to show it, `AvatarEditor` to change it, in a section or in a dialog, with the keyboard, under a strict CSP |
| **Static** | pure Go: the standard library and `golang.org/x/image`. No cgo, nothing that makes the binary dynamic, so `-tags nodynamic` on a distroless image still builds |

## Wire it

An Arandu application registers a module explicitly. There is no service
provider, no container and no discovery, so these are the lines to paste into
`bootstrap/app.go`, and there are no others.

The imports, with the other module imports. The components live in their own
package (`components.Render` and the two components below), which is imported
under another name because a project already imports Kyse's `components`:

```go
import (
	avatar "github.com/hyz-is/arandu-avatar"
	avatarui "github.com/hyz-is/arandu-avatar/components"
)
```

The construction, in `Build`, after the session store, the disk and the
activity log exist and before `k.Register`:

```go
	avatarModule, err := avatar.New(avatar.Config{
		TeamTenant: cfg.Auth.Tenant,           // the platform team's realm; empty turns the team's side off
		Directory:  accountDirectory{db: db},  // who a user and an organization are (below)
		Files:      disk,                      // the application's private R2 disk
		Activity:   activityModule.Logger(),   // every change goes to the log
		Render:     avatarui.Render,           // components.Render: how a change answers an htmx request
		Prefix:     "/perfil/avatar",
		Paths:      avatar.PortuguesePaths,    // /pessoas/{id}/foto, /espacos/{id}/remocao
		Locale:     "pt-BR",
		Middleware: []func(http.Handler) http.Handler{permissionModule.Roles()},
	}, db, sessions)
	if err != nil {
		return App{}, err
	}
```

The registration, inside the `k.Register(...)` call already there:

```go
		avatarModule,
```

Run the migration. It is not optional and not automatic: the package owns one
table, which is why `arandu.mod.toml` declares `migrations = true`.

```sh
aru migrate
```

And one line in the layout, once, in the `<head>`, which loads the stylesheet
and the script from your own origin:

```go
{!! avatar.Assets() !!}
```

`avatar.Assets()` is `avatar.Stylesheet()` and `avatar.Script()`, which a layout
may place apart (the script after `ui.js`). `AvatarEditor` writes them itself
unless told `NoAssets`; `AvatarImage` writes them only when told `WithAssets`,
because a list of fifty people would otherwise write them fifty times.

If your application mounts the team's routes inside its own operator console,
register the two sides apart, each on the group its guards are on:

```go
avatarModule.MemberRoutes(panel)   // under Prefix
avatarModule.TeamRoutes(console)   // under TeamPrefix
```

### The Directory

The package does not own users or organizations; the application does. It
implements `avatar.Directory` to tell the package who an owner is, in the tenant
of the Grant:

```go
type accountDirectory struct{ db *data.DB }

func (d accountDirectory) Owner(ctx context.Context, g security.Grant, ref avatar.Ref) (avatar.Owner, error) {
	switch ref.Kind {
	case avatar.KindUser:
		u, err := Users(d.db).WhereKey(ref.ID).First(ctx, g) // g carries the tenant: a user of another space is not found
		if err != nil || u == nil {
			return avatar.Owner{}, avatar.ErrUnknownOwner
		}
		return avatar.Owner{Name: u.Name, Email: u.Email}, nil
	case avatar.KindOrganization:
		o, err := Organizations(d.db).WhereKey(ref.ID).First(ctx, g)
		if err != nil || o == nil {
			return avatar.Owner{}, avatar.ErrUnknownOwner
		}
		return avatar.Owner{Name: o.Name}, nil
	}
	return avatar.Owner{}, avatar.ErrUnknownOwner
}
```

The `Email` is read for one thing, and only once the person switched Gravatar on:
the digest Gravatar is asked for. It is never stored, logged or sent anywhere
else. With no Directory the editor is limited to the signed-in person's own
avatar, drawn with no name and no e-mail.

## Draw it

Ask for the avatars a page draws, in one query however many there are, and draw
them:

```go
// in the controller
pictures, err := avatarModule.Resolve(ctx.Ctx(), who,
	avatar.Owner{Ref: avatar.User(u.ID), Name: u.Name, Email: u.Email},
	// ... one per person on the page
)
```

```go
// in the view
{!! avatarui.AvatarImage(avatarui.AvatarImageProps{Picture: .Pictures.Of(avatar.User(u.ID)), Size: "sm"}) !!}
```

Sizes are `xs`, `sm`, `md`, `lg`, `xl` and `2xl`. The component is Basecoat's
`avatar` -- the same class, the same ring -- with the drawing always written into
the circle and the photo laid over it: a photo that is slow, or fails, shows the
drawing, and nothing shifts. The photo is offered at every stored size through
`srcset`, loaded lazily unless `Eager`, and read aloud by the owner's name, or by
nothing when `Alt: "-"` says it sits beside the name.

When all that is wanted is an address -- for an `<img src>`, an e-mail, a
stylesheet -- `avatarModule.URL(ctx, who, owner, 64)` answers the photo's address,
or the drawing as a `data:` address when there is no photo. `Picture` also offers
`SVG()`, `DataURI()`, `URL(size)`, `Srcset()`, `Initials()` and `Source`.

## Change it

On the profile page, in place:

```go
// in the controller
editor, err := avatarModule.Editor(ctx.Ctx(), who, avatar.User(who.ID))
```

```go
// in the view
{!! avatarui.AvatarEditor(avatarui.AvatarEditorProps{Page: .Page, Editor: .AvatarEditor}) !!}
```

Or as a button that shows the avatar and opens the same editor in a dialog, for
the top bar or the account menu:

```go
{!! avatarui.AvatarEditor(avatarui.AvatarEditorProps{Page: .Page, Editor: .AvatarEditor, Dialog: true}) !!}
```

The same call with `avatar.Organization(id)` is the editor of a company's logo.

What the person sees: the picture as it stands and where it comes from; a drop
area that is the file field itself, so a click, the keyboard, a screen reader and
a phone's picker all work; after a photo is chosen, a frame to drag, zoom and move
with the arrow keys, and the result at three sizes before saving; the Gravatar
section, with a sentence that says what is sent; a grid of the person's own drawing
in every style; remove. Each change answers with the editor drawn again, carrying
a sentence of what changed or the messages of what was refused, in the person's
language -- `pt-BR` and `en` ship, and an application's own `Translator` is asked
first, key by key.

There is no inline script and no inline style. The behaviour is a file served from
your origin and registered with `arandu.ui.define`; the photo is read with
`createImageBitmap` and drawn on a canvas, so the page needs no `blob:` in
`img-src`. Without any script, the form still posts the file and the server cuts
the largest centred square.

## The routes

All behind `RequireAuth`, under `Prefix` (default `/avatars`). The segments are
`Config.Paths`; `PortuguesePaths` is the whole set in Brazilian Portuguese.

| route | |
| --- | --- |
| `GET  /users/{id}/{version}/{size}` | the picture. Also `/organizations/{id}/...` |
| `GET  /users/{id}/editor` | the editor, as a fragment |
| `POST /users/{id}/photo` | send a photo (multipart: `file`, `crop_x`, `crop_y`, `crop_size`) |
| `POST /users/{id}/gravatar` | `action=enable`, `refresh` or `disable` |
| `POST /users/{id}/style` | `style=marble` |
| `POST /users/{id}/removal` | remove the photo |

Under `TeamPrefix` (default `/admin/avatars`), when `TeamTenant` is set: the
picture, and the removal. The team's service also uploads and chooses a style from
Go. The team's address carries `?space=<tenant>` to tell apart an owner that exists
in two spaces.

## The picture, the cache and the CDN

A photo is kept at the sizes of `Config.Sizes` (default 64, 128 and 256 pixels)
and served at

```
/avatars/users/ada/3f9c2b7e1a4d5c60/128
                   └ version: the first 16 hex of the digest of the largest size
```

- **ETag and 304.** The entity tag is the version and the size, so a conditional
  request is answered `304` without the disk being read.
- **A long lifetime.** `Cache-Control: private, max-age=31536000, immutable`. The
  address changes when the photo does, so keeping it for a year is never wrong.
- **A replaced address redirects.** A page rendered before a change still links
  the old version; it answers a short-lived `302` to the current one, never the new
  picture under the old address, which a cache would keep.
- **`private` by default, and why.** Every picture is read through the policy: the
  members of the space and the platform team. A shared cache would serve it
  without asking who is looking. With a Cloudflare in front, set
  `Config.PublicCache` to answer `public` and let the edge keep it: what that
  exposes is only what somebody who already holds the address -- an owner, a
  version and a size -- could fetch. A photo is never served by a public address of
  the bucket.
- **Nothing the sender wrote is in a key.** Objects are
  `avatar/<row id>/<slot>-<version>/<size>.jpg`, written by the server, on the
  `Disk` the application passes in. In production that is the private R2 bucket;
  in tests a local adapter. Credentials stay in the application's environment.

## Gravatar and the LGPD

Switching Gravatar on sends the SHA-256 of the person's trimmed, lower-cased e-mail
to `gravatar.com`, once, so that is a transfer of personal data to a third party and
the person is the one who decides. Therefore:

- It is **off until the person switches it on**, and the editor says what is sent
  before the button.
- It is **one request**, made by the server when the person asks (on, or refresh),
  never when a page is drawn: nobody's browser is sent to Gravatar, so no visitor's
  address reaches it, and a page with fifty people makes no request at all.
- The copy is kept here, scaled and re-encoded like any photo, and served by this
  package. Switching it off removes the copy.
- The digest of the e-mail is kept on the row only to notice that the e-mail
  changed, so somebody else's face is not shown under a new address. The e-mail
  itself is never stored, and neither it nor the digest is in the activity log.
- `Config.Chain` without `SourceGravatar` turns the feature off for the
  application, and no request is ever made.

The look-up uses `d=404` and `r=g`, follows a redirect only inside the host it was
pointed at and Gravatar's own domain, has a five-second timeout and reads at most
`MaxUploadBytes`. `Config.HTTPClient` goes through a proxy.

## The drawings

Eleven styles, each a deterministic SVG made from the owner's identifier: the same
owner is the same picture on every page, server and release, and nothing is stored.
They are flat colour on a square, with no filter, no clip path, no inline style and
no script, and the one gradient has an identifier derived from the seed.

| style | |
| --- | --- |
| `initials` | a coloured tile with the first and the last letter of the name, in whichever of white and near-black reads better on it |
| `identicon` | a mirrored five-by-five block pattern on a light tile |
| `pattern` | a four-by-four tiling of quarter circles and diagonals |
| `beam` | a face on a coloured tile |
| `marble` | overlapping soft blobs |
| `pixel` | a mirrored eight-by-eight pixel pattern |
| `ring` | concentric rings |
| `sunset` | two blended colour bands, like a horizon |
| `bauhaus` | a square, a circle, a bar and a triangle |
| `shapes` | one regular polygon with a dot inside it |
| `silhouette` | the neutral head-and-shoulders of somebody unnamed |

`Config.Palette` colours them with an application's brand; `Config.Styles` narrows
what the editor offers and `Config.DefaultStyle` is what somebody who chose none
gets.

They are written here from the general ideas the market settled on, and are
inspired by -- and share no code with -- [DiceBear](https://www.dicebear.com/),
[boring-avatars](https://boringavatars.com/), [Jdenticon](https://jdenticon.com/)
and the identicons of GitHub and Gravatar.

## Who may do what

The policy denies unless a rule says otherwise, and the tenant of the record is
checked before any rule.

| act | who | |
| --- | --- | --- |
| `avatar.view` | anybody signed in to the space | the tenant check is the membership; a guest has no avatar |
| `avatar.update` | the person, on their own avatar | needs no group |
| `avatar.manage_organization` | whoever holds the action | changes the logo of an organization of the space |
| `avatar.view_all`, `avatar.manage_all` | the platform team | a subject of `TeamTenant` with the action; never somebody acting as another account |

`avatar.Actions()` and `avatar.TeamActions()` are what to declare in
`arandu-permission`'s catalogue; they are apart so an organization's groups are
never offered the team's. The tenant is always the Grant's -- `data.Tenant(g)` --
never the path, the query or the body: a member of another space gets a `404`, the
same as for an owner that does not exist. The platform team's reads cross tenants
through one internal table that is only reached after `TeamPolicy` answered, and
every write goes through the tenant's own model under a Grant scoped to it.

Every change and removal -- `avatar.uploaded`, `avatar.removed`,
`avatar.style_changed`, `avatar.gravatar_enabled`, `avatar.gravatar_refreshed`,
`avatar.gravatar_disabled` -- is recorded in the log named `avatar`, in the tenant
of the Grant, by the subject who acted, a team member included. No entry carries a
picture, an e-mail or a digest.

## Configuration

Everything is optional except what the application wires; a value out of range is
an error from `New`, never a number silently clamped. `New` also refuses a missing
database or session store.

| field | |
| --- | --- |
| `TeamTenant` | the team's realm. Empty turns its side off |
| `Prefix`, `TeamPrefix`, `Paths` | where the routes are mounted, and their segments |
| `Middleware`, `TeamMiddleware` | run after `RequireAuth`; mount `arandu-permission`'s `Roles()` here |
| `Directory` | who an owner is; see above |
| `Files` | the `Disk` the photos are kept on. Nil turns uploads and Gravatar off |
| `Chain` | the order of the sources, ending in `SourceGenerated`. Default upload, gravatar, generated |
| `DefaultStyle`, `Styles`, `Palette` | the drawings |
| `Sizes` | the sides a photo is kept at, ascending, up to four, 16 to 1024. Default 64, 128, 256 |
| `MaxUploadBytes` | default 5 MB, at most 20 MB |
| `MaxPixels` | the most pixels a photo may declare, checked from its header before it is decoded. Default 36 million |
| `JPEGQuality` | default 85 |
| `PublicCache`, `CacheMaxAge` | `public` instead of `private`; default a year |
| `GravatarURL`, `HTTPClient` | where Gravatar is fetched from (https, or a loopback address for a test) and with what |
| `Render` | `components.Render`, so a change answers an htmx request with the editor |
| `Activity` | the application's `arandu-activitylog` logger |
| `Translator`, `Locale` | `pt-BR` by default; `en` ships |
| `Now`, `Logger` | the clock and where what cannot be returned is logged |

## What a photo goes through

1. The body is bounded before it is read, and the file is read up to `MaxUploadBytes`.
2. The type is what the first bytes say -- JPEG, PNG or WebP -- never the file name
   or the announced type. HTML, SVG, GIF and a script named `photo.jpg` are refused.
3. The header is read and the size it declares is held against `MaxPixels`
   **before** a pixel is decoded, so a few dozen bytes cannot ask for gigabytes.
4. The picture is turned upright by its EXIF orientation, cut to the square the
   person chose -- a fraction of the picture, clamped into it, never a value that
   makes a request fail -- and scaled with a Catmull-Rom filter to each size.
5. It is encoded again: JPEG, or PNG when it has transparency worth keeping, such as
   a logo. Nothing but pixels is written, so every byte of EXIF, GPS, thumbnail and
   comment is gone.
6. The variants are written under keys the server generates; the row names the new
   version; the old files are removed after that.

## Install it with an assistant

`.agents/skills/avatar-package/` carries the wiring, the migration step and the
closed policy to an assistant working in your project; `aru skills:sync` copies it.

## Guarantees

- The policy denies by default; every service method authorizes before it reaches
  the Model; the tenant comes from `data.Tenant(g)`.
- A photo is never served by a public bucket address; keys are generated by the
  server; sensitive reads are authorized and a change is audited.
- `arandu.mod.toml` declares `network = true` for the one Gravatar request,
  `migrations = true` for the table, and nothing else; the package's own suite
  compares both with what the code calls.
- `CGO_ENABLED=0 go build -tags nodynamic ./...` builds, and a test refuses any
  import that would link C.

## Contract

| | |
| --- | --- |
| module | `github.com/hyz-is/arandu-avatar`; components in `.../components` |
| manifest | `hyz-is/avatar`, framework `>= 0.67` |
| table | `avatars` (`aru migrate`) |
| permission actions | `avatar.view`, `avatar.update`, `avatar.manage_organization`, `avatar.view_all`, `avatar.manage_all` |
| activity log | `avatar`, six events |
| translation group | `avatar` (`pt-BR`, `en`) |
| script and stylesheet | `arandu-avatar.js` (behaviours `avatar-image`, `avatar-editor`), `arandu-avatar.css` |
| third-party dependency | `golang.org/x/image` (WebP decoding and scaling), pure Go |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Report a vulnerability as
[SECURITY.md](SECURITY.md) says. The package is licensed under the terms of
[LICENSE.md](LICENSE.md).
