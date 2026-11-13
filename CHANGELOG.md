# Changelog

Everything worth knowing about a release of Arandu Avatar is recorded here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
the versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

A published module version is immutable: Go serves it from the proxy forever, so
a release is corrected by another release and never by moving a tag.

## [Unreleased]

## [0.1.0] - 2026-11-13

### Added

- The avatar of the people of a multi-tenant application and the logo of its
  companies' spaces. An avatar is drawn from the first source of
  `Config.Chain` that has something to show -- the photo the person sent, their
  Gravatar when they switched it on, and a drawing made from their name, which
  cannot fail -- and is one row per owner and space in the `avatars` table
  (migration `20261113_0001_create_avatars`; run `aru migrate`).
- Photos: up to 5 MB, JPEG, PNG or WebP by their bytes, never by name or
  announced type; the declared size is held against `Config.MaxPixels` before a
  pixel is decoded; the EXIF orientation is applied; the square a person framed
  is cut (fractions, clamped into the picture) and scaled with a Catmull-Rom
  filter to `Config.Sizes` (64, 128 and 256 pixels by default); the variants are
  encoded again -- JPEG, or PNG when the picture is transparent -- which removes
  EXIF and GPS. They are kept on `Config.Files` under keys the server generates.
  Pure Go: the standard library and `golang.org/x/image`; no cgo.
- Serving: `GET /avatars/{users,organizations}/{id}/{version}/{size}` with an
  entity tag, `304`, `Cache-Control: private, max-age=31536000, immutable`
  (`public` with `Config.PublicCache`, for a CDN in front), a short-lived redirect
  from a replaced version to the current one, and `nosniff`. A photo is never
  served by a public address of the bucket.
- Gravatar: only when the person switches it on. One request by the server,
  with the SHA-256 of the e-mail, `d=404` and `r=g`; the copy is kept here and
  served from here, and a changed e-mail stops showing it. Switch on, refresh and
  switch off. Declared in `arandu.mod.toml` as `network = true`.
- Eleven generated styles, deterministic SVG made from the owner's identifier:
  `initials`, `identicon`, `pattern`, `beam`, `marble`, `pixel`, `ring`, `sunset`,
  `bauhaus`, `shapes` and `silhouette`, coloured by `Config.Palette`.
- Owners: a person (`avatar.User`) and an organization (`avatar.Organization`).
  The application names them with `Config.Directory`.
- The policy: `AvatarPolicy` and `TeamPolicy`, with the actions `AvatarView`,
  `AvatarUpdate`, `AvatarManageOrganization`, `AvatarViewAll` and
  `AvatarManageAll`. A person changes their own avatar; whoever holds
  `AvatarManageOrganization` changes the logo of an organization; the platform
  team, a subject of `Config.TeamTenant`, views and changes any. `Actions()` and
  `TeamActions()` are what to declare in the `arandu-permission` catalogue.
- The activity log: every change and removal is recorded in the log `avatar`, in
  the tenant of the Grant, by the subject who acted.
- The bridge: `Module.Resolve` (one query for any number of owners), `Picture`,
  `URL` and `Editor`, and `Picture.SVG`, `DataURI`, `URL`, `Srcset`, `Initials`.
- Two Kyse components in `components`: `AvatarImage` (sizes `xs` to `2xl`, the
  drawing under the photo, `srcset`, `loading`, the name read aloud, no layout
  shift) and `AvatarEditor` (in place or in a dialog: drop or choose a photo, a
  frame to drag, zoom and move with the keyboard, three previews, Gravatar, a grid
  of styles, remove), with `components.Render` for `Config.Render`. No inline
  script or style: the behaviours `avatar-image` and `avatar-editor` are served as
  `arandu-avatar.js`, and `arandu-avatar.css` as the stylesheet; `avatar.Assets()`
  writes both tags.
- Translations in `pt-BR` and `en`, under the group `avatar`.

