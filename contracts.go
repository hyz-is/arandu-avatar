package avatar

import (
	"context"
	"errors"
	"html/template"

	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/security"
)

// ErrUnknownOwner is what a Directory answers for an owner it does not know in
// the tenant of the Grant. The package turns it into ErrNotFound, so asking for
// the avatar of somebody who does not exist and of somebody in another space
// are the same answer.
var ErrUnknownOwner = errors.New("avatar: the directory does not know this owner")

// Owner is who an avatar is for, as the application knows them: the identifier,
// the name an avatar is drawn and described with, and the e-mail a Gravatar is
// looked up by.
type Owner struct {
	Ref
	// Name is the person's name or the organization's. It is the alternative
	// text of the picture and the letters of the initials. Empty draws no
	// letters.
	Name string
	// Email is the person's e-mail. It is used for one thing, and only when the
	// person switched Gravatar on: the digest Gravatar is asked for. It is never
	// stored, logged or sent anywhere else. Empty means Gravatar cannot be used.
	Email string
}

// Directory is what the application implements so the package can name the
// owners of its avatars without owning users or organizations.
type Directory interface {
	// Owner finds the owner in the tenant of g, or answers ErrUnknownOwner. The
	// tenant is the Grant's: an implementation that looks the owner up in
	// another one lets an avatar be attached to somebody else's account.
	Owner(ctx context.Context, g security.Grant, ref Ref) (Owner, error)
}

// Renderer draws the editor the module answers a request with, as the markup
// that replaces it on the page. components.Render is the one this package
// ships.
type Renderer func(ctx *fhttp.Context, editor Editor) (template.HTML, error)
