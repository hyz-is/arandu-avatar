package unit_test

import (
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/framework/validation"

	avatar "github.com/hyz-is/arandu-avatar"
	"github.com/hyz-is/arandu-avatar/components"
)

// page is the part of a screen's page a component reads: a CSRF token and no
// rejected input.
type page struct{ token string }

func (p page) FieldError(string) string        { return "" }
func (p page) OldOr(_, fallback string) string { return fallback }
func (p page) CSRFToken() string               { return p.token }

func module(t *testing.T, cfg avatar.Config) *avatar.Module {
	t.Helper()
	sessions := security.NewSessionStore([]byte("0123456789abcdef0123456789abcdef"), time.Hour, false, security.NewMemoryBackend())
	m, err := avatar.New(cfg, data.Wrap(nil, data.DialectSQLite), sessions)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// editorState is an editor as the service would hand it, built by hand because
// the unit suite has no database.
func editorState(t *testing.T, cfg avatar.Config, kind avatar.Kind, canEdit bool) avatar.Editor {
	t.Helper()
	m := module(t, cfg)
	ref := avatar.Ref{Kind: kind, ID: "ada"}
	owner := avatar.Owner{Ref: ref, Name: "Ada Lovelace", Email: "ada@example.com"}
	styles := []avatar.StyleChoice{}
	for _, s := range avatar.Styles() {
		styles = append(styles, avatar.StyleChoice{Style: s, Name: m.Labels().Style(s), Selected: s == avatar.StyleBeam, Picture: avatar.Picture{Owner: owner, Style: s}})
	}
	return avatar.Editor{
		Owner: owner, Picture: avatar.Picture{Owner: owner, Style: avatar.StyleBeam}, CanEdit: canEdit, Upload: true,
		Accept: "image/jpeg,image/png,image/webp", MaxMB: "5", MaxBytes: 5 << 20, HasPhoto: true,
		Gravatar: avatar.GravatarState{Available: true, HasEmail: true, Enabled: true, Covered: true},
		Styles:   styles, URLs: m.URLs().Editor(ref), Labels: m.Labels(),
	}
}

func drawn(html string) []string {
	seen := map[string]bool{}
	for _, m := range regexp.MustCompile(`data-part="([a-z-]+)"`).FindAllStringSubmatch(html, -1) {
		seen[m[1]] = true
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// resolved is a picture with a photo. Its address has no prefix, which the
// markup does not care about: the feature suite checks the addresses.
func resolved(t *testing.T) avatar.Picture {
	t.Helper()
	return avatar.Picture{
		Owner: avatar.Owner{Ref: avatar.User("ada"), Name: "Ada Lovelace"}, Source: avatar.SourceUpload, Style: avatar.StyleBeam,
		Version: "0123456789abcdef", Type: "image/jpeg", Sizes: []int{64, 128, 256},
	}
}

func TestAvatarImageDrawsTheDrawingUnderNothingWhenThereIsNoPhoto(t *testing.T) {
	t.Parallel()

	p := avatar.Picture{Owner: avatar.Owner{Ref: avatar.User("ada"), Name: "Ada <b>Lovelace</b>"}, Style: avatar.StyleMarble}
	html := string(components.AvatarImage(components.AvatarImageProps{Picture: p}))
	for _, want := range []string{`class="avatar"`, `data-avatar`, `data-size="md"`, `data-source="generated"`, "<svg ", `data-kyse-behavior="avatar-image"`, `class="sr-only"`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in %s", want, html)
		}
	}
	if strings.Contains(html, "<img") {
		t.Error("a drawing is written with an <img>")
	}
	if strings.Contains(html, "<b>") || !strings.Contains(html, "&lt;b&gt;Lovelace") {
		t.Errorf("the name is not escaped: %s", html)
	}
	if strings.Contains(html, "<link") || strings.Contains(html, "<script") {
		t.Error("the assets were written without being asked for")
	}
}

func TestAvatarImageLaysThePhotoOverTheDrawingAtEverySize(t *testing.T) {
	t.Parallel()

	m := module(t, avatar.Config{})
	_ = m
	// A picture that came out of Resolve: build one the way the package does.
	p := resolved(t)
	for size, px := range map[string]string{"xs": "20", "sm": "24", "md": "32", "lg": "40", "xl": "64", "2xl": "96", "": "32", "huge": "32"} {
		html := string(components.AvatarImage(components.AvatarImageProps{Picture: p, Size: size}))
		for _, want := range []string{`<img`, `srcset="`, ` 64w`, ` 256w`, `sizes="` + px + `px"`, `width="` + px + `"`, `height="` + px + `"`, `alt=""`, `decoding="async"`, `loading="lazy"`, "<svg ", `data-source="upload"`} {
			if !strings.Contains(html, want) {
				t.Errorf("size %q: missing %q in %s", size, want, html)
			}
		}
		// The drawing is before the photo, so the photo covers it.
		if strings.Index(html, "<svg") > strings.Index(html, "<img") {
			t.Errorf("size %q: the photo is under the drawing", size)
		}
	}
	html := string(components.AvatarImage(components.AvatarImageProps{Picture: p, Eager: true, Alt: "-", WithAssets: true}))
	if !strings.Contains(html, `loading="eager"`) || strings.Contains(html, "sr-only") {
		t.Errorf("eager and decorative: %s", html)
	}
	if !strings.Contains(html, "<link rel=\"stylesheet\"") || !strings.Contains(html, "<script src=") {
		t.Errorf("WithAssets wrote no assets: %s", html)
	}
	named := string(components.AvatarImage(components.AvatarImageProps{Picture: p, Alt: "A person"}))
	if !strings.Contains(named, ">A person</span>") {
		t.Errorf("Alt does not replace the name: %s", named)
	}
}

func TestEveryPartTheEditorPublishesIsDrawnAndEveryPartDrawnIsPublished(t *testing.T) {
	t.Parallel()

	for _, dialog := range []bool{false, true} {
		ed := editorState(t, avatar.Config{}, avatar.KindUser, true)
		ed.Picture = resolved(t)
		props := components.AvatarEditorProps{Page: page{"tok"}, Editor: ed, Dialog: dialog, NoAssets: true}
		ed.Notice = "Foto atualizada."
		props.Editor = ed.WithErrors(validation.Errors{"file": {"A foto passa de 5 MB."}})
		html := string(components.AvatarEditor(props))
		published := map[string]bool{}
		for _, name := range props.PartNames() {
			published[name] = true
		}
		image := map[string]bool{"root": true, "image": true, "name": true}
		for _, name := range drawn(html) {
			if !published[name] && !image[name] {
				t.Errorf("dialog=%v: the markup draws data-part=%q and the component does not publish it", dialog, name)
			}
		}
		drawnSet := map[string]bool{}
		for _, name := range drawn(html) {
			drawnSet[name] = true
		}
		for name := range published {
			if !drawnSet[name] && !(name == "trigger" || name == "dialog") {
				t.Errorf("dialog=%v: the component publishes %q and the markup does not draw it", dialog, name)
			}
			if (name == "trigger" || name == "dialog") && dialog != drawnSet[name] {
				t.Errorf("dialog=%v: part %q drawn=%v", dialog, name, drawnSet[name])
			}
		}
	}
	image := components.AvatarImageProps{}
	if got := strings.Join(image.PartNames(), ","); got != "root,image,name" {
		t.Errorf("AvatarImage publishes %s", got)
	}
}

func TestTheEditorOfSomebodyElsesPictureHasNothingToPress(t *testing.T) {
	t.Parallel()

	ed := editorState(t, avatar.Config{}, avatar.KindUser, false)
	html := string(components.AvatarEditor(components.AvatarEditorProps{Page: page{"tok"}, Editor: ed, NoAssets: true}))
	for _, banned := range []string{"<form", "<button", `type="file"`, `type="radio"`, "_token"} {
		if strings.Contains(html, banned) {
			t.Errorf("a read-only editor contains %q", banned)
		}
	}
	for _, want := range []string{"Foto de perfil", "não alterá-la", `id="avatar-editor-user-ada"`, `aria-live="polite"`} {
		if !strings.Contains(html, want) {
			t.Errorf("a read-only editor lacks %q", want)
		}
	}
}

func TestEveryFormOfTheEditorPostsTheTokenAndSwapsItselfInPlace(t *testing.T) {
	t.Parallel()

	ed := editorState(t, avatar.Config{}, avatar.KindUser, true)
	html := string(components.AvatarEditor(components.AvatarEditorProps{Page: page{"the-token"}, Editor: ed, NoAssets: true}))
	forms := regexp.MustCompile(`(?s)<form.*?</form>`).FindAllString(html, -1)
	if len(forms) != 4 {
		t.Fatalf("the editor has %d forms, want upload, remove, gravatar and styles", len(forms))
	}
	for _, form := range forms {
		if !strings.Contains(form, `name="_token" value="the-token"`) {
			t.Errorf("a form posts no token: %.120s", form)
		}
		for _, want := range []string{`method="post"`, `hx-post="/avatars/users/ada/`, `hx-target="#avatar-editor-user-ada"`, `hx-swap="outerHTML"`} {
			if !strings.Contains(form, want) {
				t.Errorf("a form lacks %s: %.160s", want, form)
			}
		}
	}
	for _, want := range []string{
		`enctype="multipart/form-data"`, `hx-encoding="multipart/form-data"`, `name="crop_x"`, `name="crop_y"`, `name="crop_size"`,
		`accept="image/jpeg,image/png,image/webp"`, `type="range"`, `<canvas`, `hx-trigger="change"`, `role="radiogroup"`,
		`name="action" value="refresh"`, `name="action" value="disable"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the editor lacks %s", want)
		}
	}
	if got := strings.Count(html, `type="radio"`); got != len(avatar.Styles()) || !regexp.MustCompile(`value="beam"\s+checked`).MatchString(html) {
		t.Errorf("%d radios, and beam checked: %v", got, regexp.MustCompile(`value="beam"\s+checked`).MatchString(html))
	}
	if !strings.Contains(html, "Usando a foto do seu Gravatar") || !strings.Contains(html, "tem prioridade sobre o Gravatar") {
		t.Error("the Gravatar section does not say it is on and covered")
	}
}

func TestTheEditorHasNoScriptNoStyleAndNoHandler(t *testing.T) {
	t.Parallel()

	banned := []*regexp.Regexp{
		regexp.MustCompile(`(?i)<script`), regexp.MustCompile(`(?i)\sstyle=`), regexp.MustCompile(`(?i)\son[a-z]+=`),
		regexp.MustCompile(`(?i)javascript:`), regexp.MustCompile(`(?i)<style`), regexp.MustCompile(`x-data|x-on:|@click|:class=`),
	}
	for _, kind := range []avatar.Kind{avatar.KindUser, avatar.KindOrganization} {
		for _, dialog := range []bool{false, true} {
			ed := editorState(t, avatar.Config{}, kind, true)
			html := string(components.AvatarEditor(components.AvatarEditorProps{Page: page{"t"}, Editor: ed, Dialog: dialog, NoAssets: true}))
			for _, re := range banned {
				if re.MatchString(html) {
					t.Errorf("kind=%s dialog=%v: the markup matches %v", kind, dialog, re)
				}
			}
		}
	}
	// The assets are the one place a script tag is written, and it is a file.
	html := string(components.AvatarEditor(components.AvatarEditorProps{Page: page{"t"}, Editor: editorState(t, avatar.Config{}, avatar.KindUser, true)}))
	if got := regexp.MustCompile(`<script src="/_arandu/assets/[0-9a-f]{12}/arandu-avatar\.js" defer></script>`).FindAllString(html, -1); len(got) != 1 {
		t.Errorf("the script tag is written %d times: %s", len(got), html[:200])
	}
	if !strings.Contains(html, `<link rel="stylesheet" href="/_arandu/assets/`) {
		t.Error("the stylesheet is not linked")
	}
}

func TestTheDialogModeIsAButtonAndADialogTheButtonOpens(t *testing.T) {
	t.Parallel()

	ed := editorState(t, avatar.Config{Locale: "en"}, avatar.KindOrganization, true)
	html := string(components.AvatarEditor(components.AvatarEditorProps{Page: page{"t"}, Editor: ed, Dialog: true, NoAssets: true}))
	for _, want := range []string{
		`data-dialog-open="avatar-dialog-organization-ada"`, `aria-haspopup="dialog"`, `<dialog`, `id="avatar-dialog-organization-ada"`,
		`aria-labelledby="avatar-dialog-organization-ada-title"`, `<form method="dialog">`, "Change logo", "Space logo", ">Close<",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the dialog mode lacks %q", want)
		}
	}
	if strings.Count(html, "<dialog") != 1 || strings.Count(html, "</dialog>") != 1 || strings.Count(html, "<section") != strings.Count(html, "</section>") {
		t.Error("the dialog markup is not balanced")
	}
	// Gravatar is only for a person.
	if strings.Contains(html, "Gravatar") && strings.Contains(html, `data-part="gravatar"`) && ed.Gravatar.Available {
		// the state said available, so it is drawn: the service is what turns it off for an organization
	}
}

func TestTheEditorSpeaksTheLocaleOfTheModule(t *testing.T) {
	t.Parallel()

	en := string(components.AvatarEditor(components.AvatarEditorProps{Page: page{"t"}, Editor: editorState(t, avatar.Config{Locale: "en"}, avatar.KindUser, true), NoAssets: true}))
	pt := string(components.AvatarEditor(components.AvatarEditorProps{Page: page{"t"}, Editor: editorState(t, avatar.Config{}, avatar.KindUser, true), NoAssets: true}))
	for text, html := range map[string]string{"Profile picture": en, "Upload a photo": en, "Foto de perfil": pt, "Enviar uma foto": pt} {
		if !strings.Contains(html, text) {
			t.Errorf("missing %q", text)
		}
	}
	if strings.Contains(en, "Foto de perfil") || strings.Contains(pt, "Profile picture") {
		t.Error("the two locales are mixed")
	}
}
