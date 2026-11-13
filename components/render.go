// Package components draws the avatar of a Kyse application: AvatarImage, the
// circle that shows a person's photo or a drawing made from their name, and
// AvatarEditor, where a person changes it.
//
// They are Kyse components -- exported Go functions that take their own props
// and return markup -- written in the .kyse.go files beside this one and
// compiled into the .go files committed with them, so a prop that does not exist
// is a build error at the line of the view that wrote it. The two behaviours they
// need are in the script the module serves; the layout loads it with
// avatar.Assets().
package components

import (
	"html/template"

	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/hesape/view"

	avatar "github.com/hyz-is/arandu-avatar"
)

// Render draws the editor the module answers a request with: the editor as a
// fragment, without the stylesheet and the script the page already has, over the
// page of the request, which carries the CSRF token its forms post. It is the
// value of avatar.Config.Render.
func Render(ctx *fhttp.Context, editor avatar.Editor) (template.HTML, error) {
	return AvatarEditor(AvatarEditorProps{Page: view.New(ctx, ""), Editor: editor, NoAssets: true}), nil
}
