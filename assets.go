package avatar

import (
	_ "embed"
	"html/template"

	"github.com/arandu-io/hesape/view"
)

// The names the script and the stylesheet are served under, from this origin,
// by the application's asset handler. Hesape puts a digest of the bytes in the
// address, so a new release is a new address and the old one can be cached
// forever.
const (
	ScriptAsset     = "arandu-avatar.js"
	StylesheetAsset = "arandu-avatar.css"
)

// script is the client half of the components: the cropper of the editor and
// the fallback of the image. It registers two behaviours with arandu.ui.define,
// so it runs on the elements the components draw and nowhere else. Nothing in
// it reads an attribute and evaluates it, and nothing is inline.
//
//go:embed assets/arandu-avatar.js
var script []byte

// stylesheet is the avatar's own style: the sizes, the layer a drawing sits in
// under a photo, and the editor. It is written against the tokens the Basecoat
// stylesheet of the application defines, so it follows the light and the dark
// theme with nothing to configure.
//
//go:embed assets/arandu-avatar.css
var stylesheet []byte

// The assets are registered when the package is linked: a view that draws a
// component names them, and they have to exist before the first page is drawn.
func init() {
	view.RegisterAsset(ScriptAsset, "application/javascript; charset=utf-8", script)
	view.RegisterAsset(StylesheetAsset, "text/css; charset=utf-8", stylesheet)
}

// Stylesheet is the <link> to the stylesheet, for a layout's <head>:
//
//	{!! avatar.Stylesheet() !!}
//
// It is served from the origin, so a strict Content-Security-Policy holds.
func Stylesheet() template.HTML {
	return template.HTML(`<link rel="stylesheet" href="` + template.HTMLEscapeString(view.Asset(StylesheetAsset)) + `">`)
}

// Script is the <script> that loads the behaviours, for the end of a layout's
// <body> or its <head> with defer, after ui.js:
//
//	{!! avatar.Script() !!}
//
// It is served from the origin.
func Script() template.HTML {
	return template.HTML(`<script src="` + template.HTMLEscapeString(view.Asset(ScriptAsset)) + `" defer></script>`)
}

// Assets is the stylesheet and the script together: the one line a layout needs
// so every avatar and editor on every page is drawn and works.
//
//	{!! avatar.Assets() !!}
func Assets() template.HTML { return Stylesheet() + Script() }
