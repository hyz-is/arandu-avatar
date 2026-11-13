package avatar

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/framework/validation"
)

// StyleChoice is one generated style the editor offers, drawn for this owner so
// the person sees their own picture in each of them.
type StyleChoice struct {
	Style    Style
	Name     string
	Selected bool
	// Picture draws the style: Picture.SVG is the preview.
	Picture Picture
}

// GravatarState is what the editor says about Gravatar.
type GravatarState struct {
	// Available says Gravatar is part of the chain and a Disk keeps the copy, and
	// the owner is a person.
	Available bool
	// HasEmail says the application named an e-mail to look up. The e-mail itself
	// is never in the editor.
	HasEmail bool
	// Enabled says the person switched it on and a copy is kept.
	Enabled bool
	// Covered says an uploaded photo is in front of it.
	Covered bool
}

// EditorURLs are the addresses the editor posts to and reads from.
type EditorURLs struct {
	Self, Upload, Gravatar, Style, Remove string
}

// Editor is what the editor component draws: the picture as it stands, what the
// person can do about it, and the sentences in their language.
type Editor struct {
	// Owner is who the picture is for.
	Owner Owner
	// Picture is how the owner is drawn now.
	Picture Picture
	// CanEdit says the person may change this avatar. Without it the editor
	// shows the picture and nothing to press.
	CanEdit bool
	// Upload says photos can be sent, and Accept and MaxMB describe what the
	// picker offers and the limit.
	Upload   bool
	Accept   string
	MaxMB    string
	MaxBytes int64
	// HasPhoto says a photo the person sent is kept, so it can be removed.
	HasPhoto bool
	// Gravatar is the state of the Gravatar section.
	Gravatar GravatarState
	// Styles are the generated styles on offer.
	Styles []StyleChoice
	// URLs are where the forms post to.
	URLs EditorURLs
	// Labels are the sentences.
	Labels Labels
	// Notice is what the last change said, and Errors what the last was refused
	// for. The handler sets them on the editor it answers with.
	Notice string
	Errors validation.Errors
}

// ID is the identifier of the element the editor is drawn in, which the forms
// swap their answer into.
func (e Editor) ID() string {
	return "avatar-editor-" + string(e.Owner.Kind) + "-" + url.PathEscape(e.Owner.ID)
}

// WithNotice returns the editor with a sentence about what just changed.
func (e Editor) WithNotice(notice string) Editor { e.Notice = notice; return e }

// WithErrors returns the editor with what the last change was refused for.
func (e Editor) WithErrors(errs validation.Errors) Editor { e.Errors = errs; return e }

// Editor is what the editor component is handed for one owner: the picture as it
// stands and what the signed-in subject may do about it. Reading it needs only
// the right to read; CanEdit says whether the forms are drawn.
func (s *AvatarService) Editor(ctx context.Context, actor security.Subject, ref Ref) (Editor, error) {
	g, row, owner, err := s.reach(ctx, actor, AvatarView, ref)
	if err != nil {
		return Editor{}, err
	}
	canEdit := false
	if _, err := security.Authorize(ctx, s.policy, actor, updateAction(ref.Kind), candidate(ref)); err == nil {
		canEdit = true
		if row != nil {
			_, err = security.Authorize(ctx, s.policy, actor, updateAction(ref.Kind), *row)
			canEdit = err == nil
		}
	}
	_ = g
	return s.core.editorOf(owner, row, canEdit, false, ""), nil
}

// editorOf builds the editor state of an owner.
func (c *core) editorOf(owner Owner, row *Avatar, canEdit, team bool, space string) Editor {
	picture := buildPicture(c.cfg, owner, row, team, space)
	e := Editor{
		Owner: owner, Picture: picture, CanEdit: canEdit,
		Upload: c.cfg.uploadOn(), Accept: "image/jpeg,image/png,image/webp", MaxMB: megabytes(c.cfg.MaxUploadBytes), MaxBytes: c.cfg.MaxUploadBytes,
		HasPhoto: row != nil && row.HasUpload(), Labels: c.labels,
	}
	e.Gravatar = GravatarState{
		Available: c.cfg.gravatarOn() && owner.Kind == KindUser,
		HasEmail:  strings.TrimSpace(owner.Email) != "",
		Enabled:   row != nil && row.HasGravatar(),
		Covered:   row != nil && row.HasUpload() && row.HasGravatar(),
	}
	selected := picture.Style
	for _, style := range c.cfg.Styles {
		choice := picture
		choice.Style = style
		choice.Source = SourceGenerated
		e.Styles = append(e.Styles, StyleChoice{Style: style, Name: c.labels.Style(style), Selected: style == selected, Picture: choice})
	}
	e.URLs = urlsOf(c.cfg).Editor(owner.Ref)
	return e
}

// URLs are the addresses of the module's routes, for an application that links
// to them: a menu, an e-mail, a form of its own.
type URLs struct{ cfg Config }

func urlsOf(cfg Config) URLs { return URLs{cfg: cfg} }

// URLs answers the addresses of the module's routes.
func (m *Module) URLs() URLs { return urlsOf(m.cfg) }

func (u URLs) base(ref Ref) string {
	return u.cfg.Prefix + u.cfg.Paths.segment(ref.Kind) + "/" + url.PathEscape(ref.ID)
}

// Editor is the addresses the editor of an owner uses.
func (u URLs) Editor(ref Ref) EditorURLs {
	b := u.base(ref)
	return EditorURLs{
		Self: b + u.cfg.Paths.Editor, Upload: b + u.cfg.Paths.Upload, Gravatar: b + u.cfg.Paths.Gravatar,
		Style: b + u.cfg.Paths.Style, Remove: b + u.cfg.Paths.Removal,
	}
}

// Image is the address of a photo.
func (u URLs) Image(ref Ref, version string, size int) string {
	return u.base(ref) + "/" + url.PathEscape(version) + "/" + strconv.Itoa(size)
}
