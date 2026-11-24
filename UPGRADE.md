# Upgrade Guide

## v0.3.0

Nothing breaks. Three things are new, and none changes what an installation does
until it is asked to.

- **`Erase`** on `AvatarService` and `TeamService` forgets everything kept for an
  owner. Call it when an account is closed or deleted; the person's own needs no
  group, the team's needs `avatar.manage_all` and `Config.TeamTenant`.
- **`Config.Audience`** limits who sees another person's picture and editor. Nil is
  today's behaviour.
- **`Config.StaleVersion`** set to `avatar.StaleNotFound` answers 404 to a replaced
  address. The default still redirects.

The new event `avatar.erased` is in the log named `avatar`.

## v0.2.1

Nothing to do: it only raises the floor of the releases it requires (Hesape
v0.109.0, Framework v0.71.0, Kyse v0.53.1).

## v0.2.0

The editor is a YPY view. Nothing in the public API changes its name, and one
thing in the page does.

- **The editor draws `ypy-*` attributes** (`ypy-post`, `ypy-target`,
  `ypy-swap`, `ypy-encoding`, `ypy-trigger`) where it drew the attributes of the
  old library. The page that shows it has to load the YPY script, which the
  Hesape and Framework releases that carry it embed; a page that does not
  still posts the forms, as before, and is answered with a redirect back.
- **A change is recognised by `YPY-Request`.** `Config.Render` is still how a
  change answers a request that came from the script, and a request without the
  header is still sent back to the page it came from. The module reads the
  header through `Context.IsYPY`, so it needs the Framework release that has it.

## v0.1.0

First release: nothing to upgrade from. A new installation reads the README, in
this order: `go get`, the wiring (`Config.Directory` and `Config.Files` are what
make it useful, and `Config.Render` is `components.Render`), the actions in the
arandu-permission catalogue, `aru migrate`, and `avatar.Assets()` once in the
layout's head. The package requires Framework v0.67.0, Hesape v0.89.2,
arandu-activitylog v0.7.1 and Kyse v0.46.0, and Go 1.26.10.

