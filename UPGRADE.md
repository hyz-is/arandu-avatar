# Upgrade Guide

## Unreleased

## v0.1.0

First release: nothing to upgrade from. A new installation reads the README, in
this order: `go get`, the wiring (`Config.Directory` and `Config.Files` are what
make it useful, and `Config.Render` is `components.Render`), the actions in the
arandu-permission catalogue, `aru migrate`, and `avatar.Assets()` once in the
layout's head. The package requires Framework v0.67.0, Hesape v0.89.2,
arandu-activitylog v0.7.1 and Kyse v0.46.0, and Go 1.26.10.

