//go:build kyse

package components

import (
	"html/template"

	"github.com/arandu-io/framework/validation"
	"github.com/arandu-io/kyse/icons"
	kyse "github.com/arandu-io/kyse/components"

	avatar "github.com/hyz-is/arandu-avatar"
)

@go
// AvatarEditorBehavior is the name the client behaviour is registered under with
// arandu.ui.define.
const AvatarEditorBehavior = "avatar-editor"

// AvatarEditorProps is where a person changes their picture, or an organization
// its logo: the picture as it stands, a photo to send with a square to frame it,
// the Gravatar to switch on, and the drawing to choose when there is no photo.
//
// It is a section for a profile page, or, with Dialog, a button showing the
// avatar that opens the same editor in a dialog. Every control is a form that
// works on its own -- the file is posted as multipart and the server cuts the
// largest centred square -- and the client behaviour adds what a pointer wants on
// top: dropping a file, dragging the picture under the frame, zooming, the three
// previews, and posting without leaving the page. Each change answers with the
// editor drawn again, carrying what it said, in place of the old one.
//
// There is no script and no style in the markup, so the page keeps its strict
// Content-Security-Policy; the behaviour and the stylesheet are files served from
// the origin, which this component writes the tags for unless NoAssets says the
// layout has them. The controls are reachable by keyboard: the file field is the
// drop area itself, the frame moves with the arrow keys and zooms with + and -,
// and the styles are a radio group.
//
// It publishes root, trigger, dialog, notice, alert, current, upload, dropzone,
// file, cropper, stage, zoom, gravatar, styles and remove.
type AvatarEditorProps struct {
	// ComponentProps is the class, attributes and parts the caller adds.
	kyse.ComponentProps
	// Page is the page the editor is drawn on, for the CSRF token its forms post.
	Page kyse.Page
	// Editor is what the editor shows: Module.Editor answers it.
	Editor avatar.Editor
	// Dialog draws a button that shows the avatar and opens the editor in a
	// dialog, instead of drawing the editor in place.
	Dialog bool
	// DialogID is the identifier of the dialog. Empty is derived from the owner.
	DialogID string
	// NoAssets leaves out the stylesheet and the script, for a layout that
	// carries them.
	NoAssets bool
}

// Labels are the sentences.
func (p AvatarEditorProps) Labels() avatar.Labels { return p.Editor.Labels }

// Organization says the editor is for a logo.
func (p AvatarEditorProps) Organization() bool { return p.Editor.Owner.Kind == avatar.KindOrganization }

// Title is the heading of the editor.
func (p AvatarEditorProps) Title() string {
	if p.Organization() {
		return p.Labels().T("editor.title_organization")
	}
	return p.Labels().T("editor.title")
}

// Lead is the sentence under the heading.
func (p AvatarEditorProps) Lead() string {
	if p.Organization() {
		return p.Labels().T("editor.lead_organization")
	}
	return p.Labels().T("editor.lead")
}

// OpenLabel is the text beside the avatar on the button that opens the dialog.
func (p AvatarEditorProps) OpenLabel() string {
	if p.Organization() {
		return p.Labels().T("editor.open_organization")
	}
	return p.Labels().T("editor.open")
}

// DialogName is the identifier of the dialog.
func (p AvatarEditorProps) DialogName() string {
	if p.DialogID != "" {
		return p.DialogID
	}
	return "avatar-dialog-" + string(p.Editor.Owner.Kind) + "-" + p.Editor.Owner.ID
}

// Token is the CSRF token the forms post.
func (p AvatarEditorProps) Token() string {
	if page, ok := p.Page.(kyse.TokenPage); ok {
		return page.CSRFToken()
	}
	return ""
}

// Target is the selector the forms swap their answer into.
func (p AvatarEditorProps) Target() string { return "#" + p.Editor.ID() }

// FileID is the identifier of the file field.
func (p AvatarEditorProps) FileID() string { return p.Editor.ID() + "-file" }

// Summary is one line per message the last change was refused with.
func (p AvatarEditorProps) Summary() []string { return p.Labels().Summary(p.Editor.Errors) }

// SourceLabel says where the picture on show comes from.
func (p AvatarEditorProps) SourceLabel() string {
	return p.Labels().T("editor.source_" + string(p.Editor.Picture.Source))
}

// Described is the alternative text of the picture on show: the picture sits
// beside the name, so it is decorative when there is one.
func (p AvatarEditorProps) Described() string {
	if p.Editor.Owner.Name != "" {
		return "-"
	}
	return p.Labels().T("editor.alt_anonymous")
}

// GravatarText is the sentence under the Gravatar heading.
func (p AvatarEditorProps) GravatarText() string {
	if p.Editor.Gravatar.Enabled {
		return p.Labels().T("editor.gravatar_text_on")
	}
	return p.Labels().T("editor.gravatar_text")
}

// Errors is what the last change was refused for.
func (p AvatarEditorProps) Errors() validation.Errors { return p.Editor.Errors }

// RootAttrs are the editor's attributes, with the client bridge filled in when
// the caller named no behaviour of their own: the limit and the types the
// picker checks before it reads a file, and the sentences it says.
func (p AvatarEditorProps) RootAttrs() map[string]string {
	if p.Behavior.Name == "" {
		l := p.Labels()
		p.Behavior = kyse.Behavior{Name: AvatarEditorBehavior, Props: map[string]any{
			"maxBytes": p.Editor.MaxBytes,
			"types":    []string{"image/jpeg", "image/png", "image/webp"},
			"messages": map[string]string{
				"type": l.T("editor.js_type"),
				"size": l.With("editor.js_size", "mb", ":mb"),
				"read": l.T("editor.js_read"),
			},
		}}
	}
	return p.ComponentProps.RootAttrs()
}

// Assets are the stylesheet and the script, unless the layout carries them.
func (p AvatarEditorProps) Assets() template.HTML {
	if p.NoAssets {
		return ""
	}
	return avatar.Assets()
}

// PartNames are the parts this component publishes.
func (p AvatarEditorProps) PartNames() []string {
	return []string{
		"root", "trigger", "dialog", "notice", "alert", "current", "upload", "dropzone", "file", "cropper",
		"stage", "zoom", "gravatar", "styles", "remove",
	}
}
@endgo

{!! .Assets() !!}
@if(.Dialog)
	<button
		data-part="trigger"
		type="button"
		class="{{ .PartClass("trigger", "avatar-trigger") }}"
		data-dialog-open="{{ .DialogName() }}"
		aria-haspopup="dialog"
		@attributes(.PartAttrs("trigger"))
	>
		{!! AvatarImage(AvatarImageProps{Picture: .Editor.Picture, Size: "lg", Alt: "-"}) !!}
		<span>{{ .OpenLabel() }}</span>
	</button>
	<dialog
		data-part="dialog"
		id="{{ .DialogName() }}"
		class="{{ .PartClass("dialog", "dialog avatar-dialog") }}"
		aria-labelledby="{{ .DialogName() }}-title"
		@attributes(.PartAttrs("dialog"))
	>
		<div>
			<header>
				<h2 id="{{ .DialogName() }}-title">{{ .Title() }}</h2>
				<p>{{ .Lead() }}</p>
			</header>
			<section class="avatar-dialog-body">
@endif
<section
	data-part="root"
	id="{{ .Editor.ID() }}"
	class="{{ .RootClass("avatar-editor") }}"
	@if(!.Dialog)
		aria-labelledby="{{ .Editor.ID() }}-title"
	@endif
	@attributes(.RootAttrs())
>
	@if(!.Dialog)
		<header>
			<h3 id="{{ .Editor.ID() }}-title">{{ .Title() }}</h3>
			<p>{{ .Lead() }}</p>
		</header>
	@endif

	<p
		data-part="notice"
		class="{{ .PartClass("notice", "avatar-notice") }}"
		role="status"
		aria-live="polite"
	>{{ .Editor.Notice }}</p>

	@if(len(.Summary()) > 0)
		<div
			data-part="alert"
			class="{{ .PartClass("alert", "avatar-alert") }}"
			role="alert"
		>
			<ul>
				@foreach(.Summary() as line)
					<li>{{ line }}</li>
				@endforeach
			</ul>
		</div>
	@endif

	<div data-part="current" class="{{ .PartClass("current", "avatar-current") }}">
		{!! AvatarImage(AvatarImageProps{Picture: .Editor.Picture, Size: "2xl", Eager: true, Alt: .Described()}) !!}
		<div class="avatar-current-meta">
			@if(.Editor.Owner.Name != "")
				<strong>{{ .Editor.Owner.Name }}</strong>
			@endif
			<span>{{ .SourceLabel() }}</span>
			@if(.Editor.CanEdit && .Editor.HasPhoto)
				<form
					data-part="remove"
					method="post"
					action="{{ .Editor.URLs.Remove }}"
					hx-post="{{ .Editor.URLs.Remove }}"
					hx-target="{{ .Target() }}"
					hx-swap="outerHTML"
				>
					<input type="hidden" name="_token" value="{{ .Token() }}">
					<button type="submit" class="btn" data-variant="outline">{{ .Labels().T("editor.remove") }}</button>
				</form>
			@endif
		</div>
	</div>

	@if(!.Editor.CanEdit)
		<p class="avatar-notice">{{ .Labels().T("editor.readonly") }}</p>
	@endif

	@if(.Editor.CanEdit && .Editor.Upload)
		<form
			data-part="upload"
			class="avatar-block"
			method="post"
			action="{{ .Editor.URLs.Upload }}"
			enctype="multipart/form-data"
			hx-post="{{ .Editor.URLs.Upload }}"
			hx-target="{{ .Target() }}"
			hx-swap="outerHTML"
			hx-encoding="multipart/form-data"
		>
			<input type="hidden" name="_token" value="{{ .Token() }}">
			<input type="hidden" name="crop_x" value="" data-ref="crop-x">
			<input type="hidden" name="crop_y" value="" data-ref="crop-y">
			<input type="hidden" name="crop_size" value="" data-ref="crop-size">
			<h4>{{ .Labels().T("editor.photo_title") }}</h4>
			<div data-part="dropzone" class="{{ .PartClass("dropzone", "avatar-dropzone") }}">
				{!! icons.Upload(icons.Props{}) !!}
				<label class="avatar-dropzone-title" for="{{ .FileID() }}">{{ .Labels().T("editor.photo_prompt") }}</label>
				<span class="avatar-dropzone-hint" id="{{ .FileID() }}-hint">{{ .Labels().With("editor.photo_hint", "mb", .Editor.MaxMB) }}</span>
				<input
					data-part="file"
					type="file"
					id="{{ .FileID() }}"
					name="file"
					accept="{{ .Editor.Accept }}"
					aria-describedby="{{ .FileID() }}-hint"
				>
			</div>
			<p data-ref="client-error" class="avatar-error" role="alert" hidden></p>
			<div data-part="cropper" class="{{ .PartClass("cropper", "avatar-cropper") }}" hidden>
				<h5 id="{{ .FileID() }}-crop">{{ .Labels().T("editor.crop_title") }}</h5>
				<div
					data-part="stage"
					class="{{ .PartClass("stage", "avatar-stage") }}"
					tabindex="0"
					role="group"
					aria-labelledby="{{ .FileID() }}-crop"
					aria-describedby="{{ .FileID() }}-crop-hint"
				>
					<canvas data-ref="stage-canvas" width="256" height="256" aria-hidden="true"></canvas>
				</div>
				<p id="{{ .FileID() }}-crop-hint">{{ .Labels().T("editor.crop_hint") }}</p>
				<label class="avatar-zoom">
					<span>{{ .Labels().T("editor.zoom") }}</span>
					<input data-part="zoom" type="range" min="1" max="4" step="0.01" value="1">
				</label>
				<div class="avatar-previews" aria-hidden="true">
					<span>{{ .Labels().T("editor.preview") }}</span>
					<canvas data-ref="preview" width="96" height="96" class="avatar-preview-lg"></canvas>
					<canvas data-ref="preview" width="48" height="48"></canvas>
					<canvas data-ref="preview" width="24" height="24"></canvas>
				</div>
			</div>
			<div class="avatar-actions">
				<button type="submit" class="btn" data-ref="submit-photo">{{ .Labels().T("editor.save") }}</button>
				<button type="button" class="btn" data-variant="outline" data-ref="cancel" hidden>{{ .Labels().T("editor.cancel") }}</button>
			</div>
		</form>
	@endif


	@if(.Editor.CanEdit && .Editor.Gravatar.Available)
		<div data-part="gravatar" class="{{ .PartClass("gravatar", "avatar-block") }}">
			<h4>{{ .Labels().T("editor.gravatar_title") }}</h4>
			<p>{{ .GravatarText() }}</p>
			@if(!.Editor.Gravatar.HasEmail)
				<p>{{ .Labels().T("editor.gravatar_noemail") }}</p>
			@else
				@if(.Editor.Gravatar.Covered)
					<p>{{ .Labels().T("editor.gravatar_covered") }}</p>
				@endif
				<form
					class="avatar-actions"
					method="post"
					action="{{ .Editor.URLs.Gravatar }}"
					hx-post="{{ .Editor.URLs.Gravatar }}"
					hx-target="{{ .Target() }}"
					hx-swap="outerHTML"
				>
					<input type="hidden" name="_token" value="{{ .Token() }}">
					@if(.Editor.Gravatar.Enabled)
						<button type="submit" class="btn" data-variant="outline" name="action" value="refresh">{{ .Labels().T("editor.gravatar_refresh") }}</button>
						<button type="submit" class="btn" data-variant="outline" name="action" value="disable">{{ .Labels().T("editor.gravatar_disable") }}</button>
					@else
						<button type="submit" class="btn" name="action" value="enable">{{ .Labels().T("editor.gravatar_enable") }}</button>
					@endif
				</form>
			@endif
		</div>
	@endif

	@if(.Editor.CanEdit)
		<form
			data-part="styles"
			class="avatar-block"
			method="post"
			action="{{ .Editor.URLs.Style }}"
			hx-post="{{ .Editor.URLs.Style }}"
			hx-target="{{ .Target() }}"
			hx-swap="outerHTML"
			hx-trigger="change"
		>
			<input type="hidden" name="_token" value="{{ .Token() }}">
			<h4 id="{{ .Editor.ID() }}-styles">{{ .Labels().T("editor.style_title") }}</h4>
			<p>{{ .Labels().T("editor.style_lead") }}</p>
			<div class="avatar-styles" role="radiogroup" aria-labelledby="{{ .Editor.ID() }}-styles">
				@foreach(.Editor.Styles as choice)
					<label class="avatar-style">
						<input
							type="radio"
							name="style"
							value="{{ choice.Style }}"
							@if(choice.Selected)
								checked
							@endif
						>
						{!! AvatarImage(AvatarImageProps{Picture: choice.Picture, Size: "lg", Alt: "-"}) !!}
						<span>{{ choice.Name }}</span>
					</label>
				@endforeach
			</div>
			<noscript><button type="submit" class="btn">{{ .Labels().T("editor.save") }}</button></noscript>
		</form>
	@endif
</section>
@if(.Dialog)
			</section>
			<footer>
				<form method="dialog">
					<button type="submit" class="btn" data-variant="outline">{{ .Labels().T("editor.close") }}</button>
				</form>
			</footer>
		</div>
	</dialog>
@endif
