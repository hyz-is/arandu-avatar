package feature_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/framework/validation"

	avatar "github.com/hyz-is/arandu-avatar"
)

var ctx = context.Background()

// upload sends a photo as the subject.
func upload(t *testing.T, w world, who security.Subject, ref avatar.Ref, body []byte) (*avatar.Avatar, error) {
	t.Helper()
	return w.avatars.Upload(ctx, who, ref, avatar.UploadRequest{Body: bytes.NewReader(body)})
}

// picture is how the subject sees an owner.
func picture(t *testing.T, w world, who security.Subject, owner avatar.Owner) avatar.Picture {
	t.Helper()
	p, err := w.avatars.Picture(ctx, who, owner)
	if err != nil {
		t.Fatalf("resolving %s: %v", owner.Ref, err)
	}
	return p
}

// refusedOn is the field a validation error is on, and its message.
func refusedOn(t *testing.T, err error) (string, string) {
	t.Helper()
	var errs validation.Errors
	if !asErrors(err, &errs) || len(errs.Keys()) == 0 {
		t.Fatalf("got %v, want validation errors", err)
	}
	field := errs.Keys()[0]
	return field, errs.Get(field)[0]
}

func asErrors(err error, target *validation.Errors) bool {
	errs, ok := err.(validation.Errors)
	if ok {
		*target = errs
	}
	return ok
}

func TestAnOwnerNobodySetUpIsDrawnFromTheirName(t *testing.T) {
	w := install(t)
	ada := w.enroll("acme", "ada", "Ada Lovelace", "ada@example.com")
	p := picture(t, w, member("acme", "ada"), ada)
	if p.Source != avatar.SourceGenerated || p.HasImage() || p.URL(64) != "" || p.Initials() != "AL" {
		t.Fatalf("picture = %+v", p)
	}
	if !strings.HasPrefix(string(p.SVG()), "<svg ") || !strings.HasPrefix(p.DataURI(), "data:image/svg+xml;base64,") {
		t.Error("the drawing is not an SVG")
	}
	if got := w.gravatar.requests(); len(got) != 0 {
		t.Errorf("Gravatar was asked without the person switching it on: %v", got)
	}
}

func TestAPhotoIsKeptAtEverySizeAndServedWithAnETagAndALongLifetime(t *testing.T) {
	w := install(t)
	ada := w.enroll("acme", "ada", "Ada Lovelace", "ada@example.com")
	who := member("acme", "ada")

	row, err := upload(t, w, who, avatar.User("ada"), jpegBytes(t, 300, 200))
	if err != nil {
		t.Fatal(err)
	}
	p := picture(t, w, who, ada)
	if p.Source != avatar.SourceUpload || !p.HasImage() || len(p.Sizes) != 3 || row.TenantID != "acme" {
		t.Fatalf("picture = %+v, tenant %q", p, row.TenantID)
	}
	address := regexp.MustCompile(`^/avatars/users/ada/[0-9a-f]{16}/(64|128|256)$`)
	for _, size := range []int{32, 64, 100, 128, 256, 900} {
		if !address.MatchString(p.URL(size)) {
			t.Errorf("URL(%d) = %q", size, p.URL(size))
		}
	}
	if !strings.HasSuffix(p.URL(100), "/128") || !strings.HasSuffix(p.URL(900), "/256") {
		t.Errorf("the nearest size is not chosen: %s %s", p.URL(100), p.URL(900))
	}
	if !strings.Contains(p.Srcset(), " 64w, ") || !strings.HasSuffix(p.Srcset(), " 256w") {
		t.Errorf("srcset = %q", p.Srcset())
	}

	cookies := w.signIn(t, who)
	rec := w.do(t, request{method: "GET", target: p.URL(64), cookies: cookies})
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d", p.URL(64), rec.Code)
	}
	h := rec.Header()
	if h.Get("Content-Type") != "image/jpeg" || h.Get("X-Content-Type-Options") != "nosniff" ||
		h.Get("Cache-Control") != "private, max-age=31536000, immutable" || h.Get("ETag") == "" {
		t.Errorf("headers = %v", h)
	}
	img, _, err := image.Decode(rec.Body)
	if err != nil || img.Bounds().Dx() != 64 || img.Bounds().Dy() != 64 {
		t.Fatalf("body is not a 64 px square: %v", err)
	}

	// The same entity tag answers 304 with no body.
	again := w.do(t, request{method: "GET", target: p.URL(64), cookies: cookies, header: http.Header{"If-None-Match": {h.Get("ETag")}}})
	if again.Code != http.StatusNotModified || again.Body.Len() != 0 || again.Header().Get("ETag") != h.Get("ETag") {
		t.Errorf("conditional GET = %d with %d bytes", again.Code, again.Body.Len())
	}
	weak := w.do(t, request{method: "GET", target: p.URL(64), cookies: cookies, header: http.Header{"If-None-Match": {`"nope", W/` + h.Get("ETag")}}})
	if weak.Code != http.StatusNotModified {
		t.Errorf("a weak match = %d", weak.Code)
	}
	// A size that is not stored is not there.
	if rec := w.do(t, request{method: "GET", target: strings.TrimSuffix(p.URL(64), "/64") + "/99", cookies: cookies}); rec.Code != http.StatusNotFound {
		t.Errorf("an unknown size = %d", rec.Code)
	}
}

func TestANewPhotoIsANewAddressAndTheOldAddressRedirectsToIt(t *testing.T) {
	w := install(t)
	ada := w.enroll("acme", "ada", "Ada Lovelace", "")
	who := member("acme", "ada")
	if _, err := upload(t, w, who, avatar.User("ada"), jpegBytes(t, 200, 200)); err != nil {
		t.Fatal(err)
	}
	old := picture(t, w, who, ada)
	if _, err := upload(t, w, who, avatar.User("ada"), jpegBytes(t, 240, 180)); err != nil {
		t.Fatal(err)
	}
	fresh := picture(t, w, who, ada)
	if old.Version == fresh.Version {
		t.Fatal("a different photo kept the same version")
	}
	cookies := w.signIn(t, who)
	rec := w.do(t, request{method: "GET", target: old.URL(128), cookies: cookies})
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != fresh.URL(128) {
		t.Errorf("the replaced address answered %d to %q, want a redirect to %q", rec.Code, rec.Header().Get("Location"), fresh.URL(128))
	}
	if rec.Header().Get("Cache-Control") == "" || strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("a redirect must not be cached forever: %q", rec.Header().Get("Cache-Control"))
	}
	// Nothing of the replaced photo is left on the Disk: the same bytes at the
	// old key are gone.
	g := security.SystemGrant(avatar.AvatarView, "acme") //arandu:system-grant test reads the Disk of the tenant it wrote to
	files, err := w.disk.AllFiles(ctx, g, "avatar")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Errorf("the Disk holds %d files after a replacement, want the 3 of the new photo: %v", len(files), files)
	}
}

func TestInvalidUploadsAreRefusedAndKeepNothing(t *testing.T) {
	w := installWith(t, func(c *avatar.Config) { c.MaxUploadBytes = 20 << 10 })
	ada := w.enroll("acme", "ada", "Ada Lovelace", "")
	who := member("acme", "ada")

	big := jpegBytes(t, 900, 900)
	for name, tc := range map[string]struct {
		body []byte
		want string
	}{
		"fake bytes": {[]byte("MZ\x90\x00 definitely a jpeg named photo.jpg"), "JPEG, PNG ou WebP"},
		"html":       {[]byte("<html><script>alert(1)</script>"), "JPEG, PNG ou WebP"},
		"empty":      {[]byte{}, "vazio"},
		"too big":    {append(big, make([]byte, 21<<10)...), "passa de"},
		"unreadable": {append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, 9, 9, 9), "ler esta imagem"},
		"too small":  {jpegBytes(t, 20, 20), "pequena demais"},
	} {
		_, err := upload(t, w, who, avatar.User("ada"), tc.body)
		field, message := refusedOn(t, err)
		if field != "file" || !strings.Contains(message, tc.want) {
			t.Errorf("%s: %s: %q, want it to say %q", name, field, message, tc.want)
		}
	}
	if _, err := w.avatars.Upload(ctx, who, avatar.User("ada"), avatar.UploadRequest{}); err == nil {
		t.Error("an upload with no file was accepted")
	}
	if p := picture(t, w, who, ada); p.Source != avatar.SourceGenerated {
		t.Errorf("a refused upload changed the picture: %+v", p)
	}
	g := security.SystemGrant(avatar.AvatarView, "acme") //arandu:system-grant test reads the Disk of the tenant it wrote to
	if files, _ := w.disk.AllFiles(ctx, g, "avatar"); len(files) != 0 {
		t.Errorf("refused uploads left files behind: %v", files)
	}
}

func TestTheStoredPhotoCarriesNoEXIFAndNoGPS(t *testing.T) {
	w := install(t)
	w.enroll("acme", "ada", "Ada Lovelace", "")
	who := member("acme", "ada")

	plain := jpegBytes(t, 200, 200)
	payload := append([]byte("Exif\x00\x00"), []byte("GPSLatitude=-23.55;SECRET-SERIAL-123")...)
	segment := []byte{0xFF, 0xE1, byte((len(payload) + 2) >> 8), byte(len(payload) + 2)}
	tainted := append(append([]byte{0xFF, 0xD8}, append(segment, payload...)...), plain[2:]...)
	if !bytes.Contains(tainted, []byte("SECRET-SERIAL-123")) {
		t.Fatal("fixture")
	}
	if _, err := upload(t, w, who, avatar.User("ada"), tainted); err != nil {
		t.Fatal(err)
	}
	p := picture(t, w, who, avatar.Owner{Ref: avatar.User("ada")})
	cookies := w.signIn(t, who)
	for _, size := range p.Sizes {
		rec := w.do(t, request{method: "GET", target: p.URL(size), cookies: cookies})
		if rec.Code != 200 {
			t.Fatalf("size %d: %d", size, rec.Code)
		}
		for _, marker := range []string{"Exif", "GPSLatitude", "SECRET-SERIAL-123"} {
			if bytes.Contains(rec.Body.Bytes(), []byte(marker)) {
				t.Errorf("size %d still carries %q", size, marker)
			}
		}
	}
}

func TestATransparentLogoStaysAPNG(t *testing.T) {
	w := install(t)
	org := avatar.Owner{Ref: avatar.Organization("acme"), Name: "Acme Ltda"}
	w.dir.add("acme", org)
	boss := manager("acme", "boss")
	body := transparentPNG(t)
	if _, err := w.avatars.Upload(ctx, boss, avatar.Organization("acme"), avatar.UploadRequest{Body: bytes.NewReader(body)}); err != nil {
		t.Fatal(err)
	}
	p := picture(t, w, member("acme", "ada"), org)
	rec := w.do(t, request{method: "GET", target: p.URL(128), cookies: w.signIn(t, member("acme", "ada"))})
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("logo: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	if !strings.HasPrefix(p.URL(128), "/avatars/organizations/acme/") {
		t.Errorf("logo address = %s", p.URL(128))
	}
}

func digest(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(sum[:])
}

func TestTheChainFallsFromTheUploadToGravatarToTheDrawing(t *testing.T) {
	w := install(t)
	ada := w.enroll("acme", "ada", "Ada Lovelace", " Ada@Example.com ")
	who := member("acme", "ada")
	w.gravatar.pictures[digest("ada@example.com")] = jpegBytes(t, 400, 400)

	// 1. Gravatar is off until the person switches it on.
	if p := picture(t, w, who, ada); p.Source != avatar.SourceGenerated {
		t.Fatalf("before: %s", p.Source)
	}
	if len(w.gravatar.requests()) != 0 {
		t.Fatal("Gravatar was asked before consent")
	}

	// 2. On: one request, with the digest and d=404, never the e-mail.
	if _, err := w.avatars.SetGravatar(ctx, who, avatar.User("ada"), avatar.GravatarEnable); err != nil {
		t.Fatal(err)
	}
	asked := w.gravatar.requests()
	if len(asked) != 1 || !strings.HasPrefix(asked[0], "/avatar/"+digest("ada@example.com")+"?") ||
		!strings.Contains(asked[0], "d=404") || strings.Contains(asked[0], "example.com") || strings.Contains(asked[0], "@") {
		t.Fatalf("Gravatar was asked %v", asked)
	}
	if p := picture(t, w, who, ada); p.Source != avatar.SourceGravatar || !p.HasImage() {
		t.Fatalf("gravatar: %+v", p)
	}
	// The copy is served by this package, not by Gravatar.
	p := picture(t, w, who, ada)
	if rec := w.do(t, request{method: "GET", target: p.URL(64), cookies: w.signIn(t, who)}); rec.Code != 200 {
		t.Errorf("the Gravatar copy answered %d", rec.Code)
	}
	if len(w.gravatar.requests()) != 1 {
		t.Error("serving the copy went back to Gravatar")
	}

	// 3. A photo the person sent is in front of it.
	if _, err := upload(t, w, who, avatar.User("ada"), jpegBytes(t, 200, 200)); err != nil {
		t.Fatal(err)
	}
	if p := picture(t, w, who, ada); p.Source != avatar.SourceUpload {
		t.Fatalf("upload: %s", p.Source)
	}

	// 4. Remove it and Gravatar is back; switch that off and it is a drawing.
	if err := w.avatars.Remove(ctx, who, avatar.User("ada")); err != nil {
		t.Fatal(err)
	}
	if p := picture(t, w, who, ada); p.Source != avatar.SourceGravatar {
		t.Fatalf("after remove: %s", p.Source)
	}
	if _, err := w.avatars.SetGravatar(ctx, who, avatar.User("ada"), avatar.GravatarDisable); err != nil {
		t.Fatal(err)
	}
	if p := picture(t, w, who, ada); p.Source != avatar.SourceGenerated || p.HasImage() {
		t.Fatalf("after disable: %+v", p)
	}
	g := security.SystemGrant(avatar.AvatarView, "acme") //arandu:system-grant test reads the Disk of the tenant it wrote to
	if files, _ := w.disk.AllFiles(ctx, g, "avatar"); len(files) != 0 {
		t.Errorf("files left after everything was removed: %v", files)
	}
}

func TestAChangedEmailDoesNotShowSomebodyElsesFace(t *testing.T) {
	w := install(t)
	ada := w.enroll("acme", "ada", "Ada Lovelace", "ada@example.com")
	who := member("acme", "ada")
	w.gravatar.pictures[digest("ada@example.com")] = jpegBytes(t, 300, 300)
	if _, err := w.avatars.SetGravatar(ctx, who, avatar.User("ada"), avatar.GravatarEnable); err != nil {
		t.Fatal(err)
	}
	ada.Email = "somebody.else@example.com"
	if p := picture(t, w, who, ada); p.Source != avatar.SourceGenerated {
		t.Errorf("the copy of another e-mail's Gravatar is still shown: %s", p.Source)
	}
}

func TestGravatarWithNoPictureOrNoAnswerChangesNothing(t *testing.T) {
	w := install(t)
	ada := w.enroll("acme", "ada", "Ada Lovelace", "ada@example.com")
	who := member("acme", "ada")

	_, err := w.avatars.SetGravatar(ctx, who, avatar.User("ada"), avatar.GravatarEnable)
	field, message := refusedOn(t, err)
	if field != "gravatar" || !strings.Contains(message, "encontramos um Gravatar") {
		t.Errorf("no Gravatar: %s %q", field, message)
	}
	w.gravatar.status = http.StatusInternalServerError
	_, err = w.avatars.SetGravatar(ctx, who, avatar.User("ada"), avatar.GravatarEnable)
	if _, message := refusedOn(t, err); !strings.Contains(message, "não respondeu") {
		t.Errorf("a failing Gravatar: %q", message)
	}
	if p := picture(t, w, who, ada); p.Source != avatar.SourceGenerated {
		t.Errorf("a failed look-up changed the picture: %s", p.Source)
	}

	// A person with no e-mail on record cannot look one up, and an organization
	// has no Gravatar at all.
	w.enroll("acme", "bob", "Bob", "")
	_, err = w.avatars.SetGravatar(ctx, member("acme", "bob"), avatar.User("bob"), avatar.GravatarEnable)
	if _, message := refusedOn(t, err); !strings.Contains(message, "e-mail") {
		t.Errorf("no e-mail: %q", message)
	}
	w.dir.add("acme", avatar.Owner{Ref: avatar.Organization("acme"), Name: "Acme"})
	_, err = w.avatars.SetGravatar(ctx, manager("acme", "boss"), avatar.Organization("acme"), avatar.GravatarEnable)
	if _, message := refusedOn(t, err); !strings.Contains(message, "Só uma pessoa") {
		t.Errorf("an organization's Gravatar: %q", message)
	}
	if _, err = w.avatars.SetGravatar(ctx, who, avatar.User("ada"), "nonsense"); err == nil {
		t.Error("an unknown Gravatar action was accepted")
	}
}

func TestGravatarCanBeLeftOutOfTheChain(t *testing.T) {
	w := installWith(t, func(c *avatar.Config) { c.Chain = []avatar.Source{avatar.SourceUpload, avatar.SourceGenerated} })
	w.enroll("acme", "ada", "Ada", "ada@example.com")
	_, err := w.avatars.SetGravatar(ctx, member("acme", "ada"), avatar.User("ada"), avatar.GravatarEnable)
	if _, message := refusedOn(t, err); !strings.Contains(message, "Gravatar não está ligado") {
		t.Errorf("message = %q", message)
	}
	if len(w.gravatar.requests()) != 0 {
		t.Error("Gravatar was asked although it is not in the chain")
	}
}

func TestWithoutADiskThereAreNoUploadsAndTheDrawingStillWorks(t *testing.T) {
	w := installWith(t, func(c *avatar.Config) { c.Files = nil })
	ada := w.enroll("acme", "ada", "Ada Lovelace", "ada@example.com")
	who := member("acme", "ada")
	_, err := upload(t, w, who, avatar.User("ada"), jpegBytes(t, 200, 200))
	if field, _ := refusedOn(t, err); field != "file" {
		t.Errorf("field = %s", field)
	}
	if p := picture(t, w, who, ada); p.Source != avatar.SourceGenerated {
		t.Error("the drawing does not work without a Disk")
	}
	if _, err := w.avatars.SetStyle(ctx, who, avatar.User("ada"), avatar.StyleBeam); err != nil {
		t.Fatal(err)
	}
	if p := picture(t, w, who, ada); p.Style != avatar.StyleBeam {
		t.Errorf("style = %s", p.Style)
	}
}

func TestAPersonChoosesTheStyleOfTheirDrawing(t *testing.T) {
	w := install(t)
	ada := w.enroll("acme", "ada", "Ada Lovelace", "")
	who := member("acme", "ada")
	for _, style := range avatar.Styles() {
		if _, err := w.avatars.SetStyle(ctx, who, avatar.User("ada"), style); err != nil {
			t.Fatalf("%s: %v", style, err)
		}
		if p := picture(t, w, who, ada); p.Style != style || !strings.HasPrefix(string(p.SVG()), "<svg") {
			t.Fatalf("style %s: %+v", style, p)
		}
	}
	if _, err := w.avatars.SetStyle(ctx, who, avatar.User("ada"), "../../etc/passwd"); err == nil {
		t.Error("an unknown style was accepted")
	}
	// Choosing the style a second time changes nothing and writes nothing.
	before := len(entries(t, w, "acme"))
	if _, err := w.avatars.SetStyle(ctx, who, avatar.User("ada"), avatar.Styles()[len(avatar.Styles())-1]); err != nil {
		t.Fatal(err)
	}
	if len(entries(t, w, "acme")) != before {
		t.Error("choosing the same style wrote an entry")
	}
}

func TestOnlyTheStylesTheApplicationOffersCanBeChosen(t *testing.T) {
	w := installWith(t, func(c *avatar.Config) {
		c.Styles = []avatar.Style{avatar.StyleInitials, avatar.StyleMarble}
		c.DefaultStyle = avatar.StyleMarble
	})
	ada := w.enroll("acme", "ada", "Ada", "")
	who := member("acme", "ada")
	if p := picture(t, w, who, ada); p.Style != avatar.StyleMarble {
		t.Errorf("default style = %s", p.Style)
	}
	if _, err := w.avatars.SetStyle(ctx, who, avatar.User("ada"), avatar.StyleBeam); err == nil {
		t.Error("a style that is not offered was accepted")
	}
	ed, err := w.avatars.Editor(ctx, who, avatar.User("ada"))
	if err != nil || len(ed.Styles) != 2 {
		t.Fatalf("editor offers %d styles (%v)", len(ed.Styles), err)
	}
}

func TestAPersonChangesTheirOwnPictureAndNobodyElses(t *testing.T) {
	w := install(t)
	w.enroll("acme", "ada", "Ada", "")
	w.enroll("acme", "bob", "Bob", "")
	bob := member("acme", "bob")

	for name, err := range map[string]error{
		"upload":   second(upload(t, w, bob, avatar.User("ada"), jpegBytes(t, 200, 200))),
		"style":    second(w.avatars.SetStyle(ctx, bob, avatar.User("ada"), avatar.StyleBeam)),
		"remove":   w.avatars.Remove(ctx, bob, avatar.User("ada")),
		"gravatar": second(w.avatars.SetGravatar(ctx, bob, avatar.User("ada"), avatar.GravatarDisable)),
	} {
		requireErr(t, err, security.ErrForbidden, name+" on somebody else's avatar")
	}
	// Reading the avatars of the space is open to the space.
	if _, err := w.avatars.Picture(ctx, bob, avatar.Owner{Ref: avatar.User("ada"), Name: "Ada"}); err != nil {
		t.Errorf("a member could not read another member's avatar: %v", err)
	}
	// A guest has none.
	guest := security.Guest("acme")
	requireErr(t, second(w.avatars.Picture(ctx, guest, avatar.Owner{Ref: avatar.User("ada")})), security.ErrForbidden, "a guest reading")
}

func second[T any](_ T, err error) error { return err }

func TestTheLogoOfAnOrganizationNeedsTheActionThatChangesIt(t *testing.T) {
	w := install(t)
	org := avatar.Owner{Ref: avatar.Organization("acme"), Name: "Acme Ltda"}
	w.dir.add("acme", org)
	logo := jpegBytes(t, 200, 200)

	requireErr(t, second(upload(t, w, member("acme", "ada"), org.Ref, logo)), security.ErrForbidden, "a member changing the logo")
	if _, err := upload(t, w, manager("acme", "boss"), org.Ref, logo); err != nil {
		t.Fatalf("the manager changing the logo: %v", err)
	}
	if p := picture(t, w, member("acme", "ada"), org); p.Source != avatar.SourceUpload {
		t.Errorf("the members do not see the logo: %s", p.Source)
	}
	if err := w.avatars.Remove(ctx, manager("acme", "boss"), org.Ref); err != nil {
		t.Fatal(err)
	}
	// An action granted by name opens only that: the logo action does not let
	// the manager change a person's avatar.
	w.enroll("acme", "ada", "Ada", "")
	requireErr(t, second(upload(t, w, manager("acme", "boss"), avatar.User("ada"), logo)), security.ErrForbidden, "a manager changing a person")
}

func TestTheSpacesAreIsolatedFromEachOther(t *testing.T) {
	w := install(t)
	acme := w.enroll("acme", "ada", "Ada Lovelace", "")
	w.enroll("other", "eve", "Eve", "")
	ada := member("acme", "ada")
	eve := member("other", "eve")
	if _, err := upload(t, w, ada, avatar.User("ada"), jpegBytes(t, 200, 200)); err != nil {
		t.Fatal(err)
	}
	p := picture(t, w, ada, acme)

	// Another space's member is drawn the address as a 404 and the owner as nobody.
	requireErr(t, second(w.avatars.Open(ctx, eve, avatar.User("ada"), p.Version, 64, "")), avatar.ErrNotFound, "opening another space's photo")
	rec := w.do(t, request{method: "GET", target: p.URL(64), cookies: w.signIn(t, eve)})
	if rec.Code != http.StatusNotFound {
		t.Errorf("another space's GET = %d, want 404", rec.Code)
	}
	if got := picture(t, w, eve, acme); got.Source != avatar.SourceGenerated {
		t.Errorf("another space resolves the photo: %s", got.Source)
	}
	// The same identifier in another space is another avatar.
	w.enroll("other", "ada", "Another Ada", "")
	if _, err := upload(t, w, member("other", "ada"), avatar.User("ada"), jpegBytes(t, 180, 240)); err != nil {
		t.Fatal(err)
	}
	if got := picture(t, w, ada, acme); got.Version != p.Version {
		t.Error("an upload in another space changed this space's photo")
	}
	var rows int
	if err := w.sql.QueryRow(`SELECT COUNT(*) FROM avatars WHERE owner_id = 'ada'`).Scan(&rows); err != nil || rows != 2 {
		t.Errorf("rows = %d (%v), want one per space", rows, err)
	}
	// An owner the directory does not know in this space is a 404, not a row.
	requireErr(t, second(upload(t, w, member("acme", "zed"), avatar.User("zed"), jpegBytes(t, 200, 200))), avatar.ErrNotFound, "an unknown owner")
	// A reference that cannot name an owner is a 404 before anything is read.
	requireErr(t, second(w.avatars.Editor(ctx, ada, avatar.Ref{Kind: "user", ID: "../../etc"})), avatar.ErrNotFound, "a path as an identifier")
}

func TestThePlatformTeamSeesAndRemovesAnyonesPictureThroughItsOwnRoutes(t *testing.T) {
	w := install(t)
	ada := w.enroll("acme", "ada", "Ada Lovelace", "")
	if _, err := upload(t, w, member("acme", "ada"), avatar.User("ada"), jpegBytes(t, 200, 200)); err != nil {
		t.Fatal(err)
	}
	root := agent("root")
	pictures, err := w.team.Resolve(ctx, root, avatar.TeamOwner{Owner: ada})
	if err != nil {
		t.Fatal(err)
	}
	p := pictures.Of(ada.Ref)
	if p.Source != avatar.SourceUpload || !strings.HasPrefix(p.URL(64), "/admin/avatars/users/ada/") || !strings.HasSuffix(p.URL(64), "?space=acme") {
		t.Fatalf("team picture = %s %s", p.Source, p.URL(64))
	}
	rec := w.do(t, request{method: "GET", target: p.URL(64), cookies: w.signIn(t, root)})
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("team GET = %d", rec.Code)
	}
	// A customer is not the team, whatever it asks.
	if rec := w.do(t, request{method: "GET", target: p.URL(64), cookies: w.signIn(t, member("acme", "ada"))}); rec.Code != http.StatusForbidden {
		t.Errorf("a customer on the team's route = %d, want 403", rec.Code)
	}
	// A team member without the action is refused too.
	bare := remember(security.Subject{ID: "intern", Tenant: realm, Verified: true})
	requireErr(t, w.team.Remove(ctx, bare, ada.Ref, "acme"), security.ErrForbidden, "a team member with no action")

	if err := w.team.Remove(ctx, root, ada.Ref, "acme"); err != nil {
		t.Fatal(err)
	}
	if got := picture(t, w, member("acme", "ada"), ada); got.Source != avatar.SourceGenerated {
		t.Errorf("after the team removed it: %s", got.Source)
	}
	// The act is in the log of the customer's tenant, by the team member.
	var found bool
	for _, e := range entries(t, w, "acme") {
		if e.Event == avatar.ActivityRemoved && e.Causer == "root" {
			found = true
		}
	}
	if !found {
		t.Errorf("the team's removal is not in the log: %v", entries(t, w, "acme"))
	}
	// A space that does not exist for the owner is a 404.
	requireErr(t, w.team.Remove(ctx, root, avatar.User("nobody"), ""), avatar.ErrNotFound, "an owner with no row and no space")
	requireErr(t, w.team.Remove(ctx, root, ada.Ref, "no such space!"), avatar.ErrNotFound, "a space that cannot be one")
}

func TestEveryChangeAndRemovalIsInTheActivityLogWithoutWhatIsPrivate(t *testing.T) {
	w := install(t)
	w.enroll("acme", "ada", "Ada Lovelace", "ada@example.com")
	w.gravatar.pictures[digest("ada@example.com")] = jpegBytes(t, 200, 200)
	who := member("acme", "ada")

	steps := []func() error{
		func() error { return second(upload(t, w, who, avatar.User("ada"), jpegBytes(t, 200, 200))) },
		func() error { return second(w.avatars.SetStyle(ctx, who, avatar.User("ada"), avatar.StyleMarble)) },
		func() error {
			return second(w.avatars.SetGravatar(ctx, who, avatar.User("ada"), avatar.GravatarEnable))
		},
		func() error {
			return second(w.avatars.SetGravatar(ctx, who, avatar.User("ada"), avatar.GravatarRefresh))
		},
		func() error {
			return second(w.avatars.SetGravatar(ctx, who, avatar.User("ada"), avatar.GravatarDisable))
		},
		func() error { return w.avatars.Remove(ctx, who, avatar.User("ada")) },
	}
	for i, step := range steps {
		if err := step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
	want := []string{
		avatar.ActivityUploaded, avatar.ActivityStyleChanged, avatar.ActivityGravatarEnabled,
		avatar.ActivityGravatarRefresh, avatar.ActivityGravatarDisabled, avatar.ActivityRemoved,
	}
	got := events(t, w, "acme")
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("events = %v, want %v", got, want)
	}
	for _, e := range entries(t, w, "acme") {
		if e.Causer != "ada" || e.Description == "" {
			t.Errorf("entry %s: causer %q, description %q", e.Event, e.Causer, e.Description)
		}
		for _, secret := range []string{"ada@example.com", digest("ada@example.com"), "example.com"} {
			if strings.Contains(e.Properties, secret) || strings.Contains(e.Description, secret) {
				t.Errorf("entry %s carries %q", e.Event, secret)
			}
		}
	}
}

func TestTheOwnersOfAListAreResolvedInOneQueryPerKind(t *testing.T) {
	w := install(t)
	var owners []avatar.Owner
	for _, id := range []string{"a", "b", "c", "d"} {
		owners = append(owners, w.enroll("acme", id, "Person "+strings.ToUpper(id), ""))
	}
	if _, err := upload(t, w, member("acme", "b"), avatar.User("b"), jpegBytes(t, 200, 200)); err != nil {
		t.Fatal(err)
	}
	if _, err := w.avatars.SetStyle(ctx, member("acme", "c"), avatar.User("c"), avatar.StylePixel); err != nil {
		t.Fatal(err)
	}
	owners = append(owners, avatar.Owner{Ref: avatar.Ref{Kind: "user", ID: "bad/id"}, Name: "Bad"})
	pictures, err := w.module.Resolve(ctx, member("acme", "a"), owners...)
	if err != nil {
		t.Fatal(err)
	}
	if len(pictures) != 5 || pictures.Of(avatar.User("b")).Source != avatar.SourceUpload ||
		pictures.Of(avatar.User("c")).Style != avatar.StylePixel || pictures.Of(avatar.User("a")).Source != avatar.SourceGenerated {
		t.Errorf("pictures = %+v", pictures)
	}
	url, err := w.module.URL(ctx, member("acme", "a"), owners[1], 64)
	if err != nil || !strings.HasPrefix(url, "/avatars/users/b/") {
		t.Errorf("URL of a photo = %q (%v)", url, err)
	}
	url, err = w.module.URL(ctx, member("acme", "a"), owners[0], 64)
	if err != nil || !strings.HasPrefix(url, "data:image/svg+xml;base64,") {
		t.Errorf("URL of a drawing = %.40q (%v)", url, err)
	}
}

func TestACDNInFrontMayKeepThePicturesWhenTheApplicationSaysSo(t *testing.T) {
	w := installWith(t, func(c *avatar.Config) { c.PublicCache = true })
	ada := w.enroll("acme", "ada", "Ada", "")
	who := member("acme", "ada")
	if _, err := upload(t, w, who, avatar.User("ada"), jpegBytes(t, 200, 200)); err != nil {
		t.Fatal(err)
	}
	p := picture(t, w, who, ada)
	rec := w.do(t, request{method: "GET", target: p.URL(64), cookies: w.signIn(t, who)})
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q", got)
	}
}
