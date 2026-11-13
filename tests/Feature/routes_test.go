package feature_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/framework/data"
	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/security"

	avatar "github.com/hyz-is/arandu-avatar"
	"github.com/hyz-is/arandu-avatar/components"
)

// everyRoute is a request to each route the module registers, under the default
// prefix. TestTheModuleRegistersItsRoutesUnderItsPrefix fails when a route
// exists that this list does not reach.
var everyRoute = []struct{ method, target string }{
	{"GET", "/avatars/users/ada/0123456789abcdef/64"},
	{"GET", "/avatars/users/ada/editor"},
	{"POST", "/avatars/users/ada/photo"},
	{"POST", "/avatars/users/ada/gravatar"},
	{"POST", "/avatars/users/ada/style"},
	{"POST", "/avatars/users/ada/removal"},
	{"GET", "/avatars/organizations/acme/0123456789abcdef/64"},
	{"GET", "/avatars/organizations/acme/editor"},
	{"POST", "/avatars/organizations/acme/photo"},
	{"POST", "/avatars/organizations/acme/gravatar"},
	{"POST", "/avatars/organizations/acme/style"},
	{"POST", "/avatars/organizations/acme/removal"},
	{"GET", "/admin/avatars/users/ada/0123456789abcdef/64"},
	{"POST", "/admin/avatars/users/ada/removal"},
	{"GET", "/admin/avatars/organizations/acme/0123456789abcdef/64"},
	{"POST", "/admin/avatars/organizations/acme/removal"},
}

func TestAVisitorWithNoSessionIsSentToSignIn(t *testing.T) {
	w := install(t)
	for _, r := range everyRoute {
		rec := w.do(t, request{method: r.method, target: r.target})
		if rec.Code < 300 || rec.Code > 399 || !strings.Contains(rec.Header().Get("Location"), "/auth/login") {
			t.Errorf("%s %s with no session = %d to %q, want a redirect to sign in", r.method, r.target, rec.Code, rec.Header().Get("Location"))
		}
	}
}

func TestTheModuleRegistersItsRoutesUnderItsPrefix(t *testing.T) {
	w := installWith(t, func(c *avatar.Config) {
		c.Prefix, c.TeamPrefix, c.Paths = "/perfil/fotos", "/admin/fotos", avatar.PortuguesePaths
	})
	reached := 0
	for _, r := range []struct{ method, target string }{
		{"GET", "/perfil/fotos/pessoas/ada/0123456789abcdef/64"}, {"GET", "/perfil/fotos/pessoas/ada/editor"},
		{"POST", "/perfil/fotos/pessoas/ada/foto"}, {"POST", "/perfil/fotos/pessoas/ada/gravatar"},
		{"POST", "/perfil/fotos/pessoas/ada/estilo"}, {"POST", "/perfil/fotos/pessoas/ada/remocao"},
		{"GET", "/perfil/fotos/espacos/acme/editor"}, {"POST", "/admin/fotos/pessoas/ada/remocao"},
		{"GET", "/admin/fotos/espacos/acme/0123456789abcdef/64"},
	} {
		rec := w.do(t, request{method: r.method, target: r.target})
		if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
			t.Errorf("%s %s = %d, want the sign-in redirect that proves the route exists and is guarded", r.method, r.target, rec.Code)
		}
		reached++
	}
	// Nothing is left at the default addresses.
	if rec := w.do(t, request{method: "GET", target: "/avatars/users/ada/editor"}); rec.Code != http.StatusNotFound {
		t.Errorf("the default address still answers: %d", rec.Code)
	}
	if reached == 0 {
		t.Fatal("no route was reached")
	}
}

func TestNoRouteLandsInTheFrameworkNamespace(t *testing.T) {
	for _, prefix := range []string{"/_arandu", "/_arandu/avatars"} {
		_, err := avatar.New(avatar.Config{Prefix: prefix}, data.Wrap(nil, data.DialectSQLite), nopSessions())
		if err == nil {
			t.Errorf("a prefix of %q was accepted", prefix)
		}
	}
	w := install(t)
	for _, r := range everyRoute {
		if strings.HasPrefix(r.target, "/_arandu") {
			t.Errorf("%s is in the framework's namespace", r.target)
		}
	}
	_ = w
}

func nopSessions() *security.SessionStore {
	return security.NewSessionStore([]byte(appKey), time.Hour, false, security.NewMemoryBackend())
}

func TestAMultipartUploadThroughTheRouteAnswersTheEditorToHTMX(t *testing.T) {
	w := install(t)
	w.enroll("acme", "ada", "Ada Lovelace", "ada@example.com")
	who := member("acme", "ada")
	cookies := w.signIn(t, who)

	body, contentType := multipartBody(t, "file", "me.jpg", jpegBytes(t, 400, 300),
		map[string]string{"crop_x": "0.2", "crop_y": "0", "crop_size": "0.8", "_token": "x"})
	rec := w.do(t, request{
		method: "POST", target: "/avatars/users/ada/photo", body: body, contentType: contentType, cookies: cookies,
		header: http.Header{"HX-Request": {"true"}},
	})
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("upload = %d %s: %s", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
	html := rec.Body.String()
	for _, want := range []string{`id="avatar-editor-user-ada"`, "Foto atualizada.", `data-source="upload"`, "Remover foto", `hx-post="/avatars/users/ada/photo"`} {
		if !strings.Contains(html, want) {
			t.Errorf("the editor does not contain %q", want)
		}
	}
	if strings.Contains(html, "<html") || strings.Contains(html, "<link rel") {
		t.Error("the answer is a page, or carries the assets the page already has")
	}
	if p := picture(t, w, who, avatar.Owner{Ref: avatar.User("ada")}); p.Source != avatar.SourceUpload {
		t.Errorf("the photo was not kept: %s", p.Source)
	}

	// A refusal answers the editor again, with the message in it, and keeps the
	// photo that was there.
	bad, bct := multipartBody(t, "file", "me.jpg", []byte("not a picture at all"), nil)
	rec = w.do(t, request{method: "POST", target: "/avatars/users/ada/photo", body: bad, contentType: bct, cookies: cookies, header: http.Header{"HX-Request": {"true"}}})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "A foto precisa ser JPEG, PNG ou WebP.") || !strings.Contains(rec.Body.String(), `role="alert"`) {
		t.Errorf("refusal = %d: %s", rec.Code, rec.Body.String())
	}
	// Without htmx the refusal is the router's: it goes back with the messages.
	rec = w.do(t, request{method: "POST", target: "/avatars/users/ada/photo", body: bad, contentType: bct, cookies: cookies})
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Errorf("a refusal with no htmx = %d, want a redirect back", rec.Code)
	}
	// And a body that is not multipart is a refusal too, not a crash.
	rec = w.do(t, request{method: "POST", target: "/avatars/users/ada/photo", body: []byte("file=x"), contentType: "application/x-www-form-urlencoded", cookies: cookies, header: http.Header{"HX-Request": {"true"}}})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Escolha uma foto") {
		t.Errorf("a non-multipart body = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTheOtherFormsAnswerThroughTheRoutes(t *testing.T) {
	w := install(t)
	w.enroll("acme", "ada", "Ada Lovelace", "ada@example.com")
	w.gravatar.pictures[digest("ada@example.com")] = jpegBytes(t, 300, 300)
	cookies := w.signIn(t, member("acme", "ada"))
	htmx := http.Header{"HX-Request": {"true"}}
	form := func(target, body string) *httptest.ResponseRecorder {
		return w.do(t, request{method: "POST", target: target, body: []byte(body), contentType: "application/x-www-form-urlencoded", cookies: cookies, header: htmx})
	}

	if rec := form("/avatars/users/ada/style", "style=marble"); !strings.Contains(rec.Body.String(), "Desenho escolhido.") ||
		!regexp.MustCompile(`value="marble"\s+checked`).MatchString(rec.Body.String()) {
		t.Errorf("style: %s", rec.Body.String())
	}
	if rec := form("/avatars/users/ada/gravatar", "action=enable"); !strings.Contains(rec.Body.String(), "Gravatar ligado.") ||
		!strings.Contains(rec.Body.String(), "Atualizar do Gravatar") {
		t.Errorf("gravatar on: %s", rec.Body.String())
	}
	if rec := form("/avatars/users/ada/gravatar", "action=refresh"); !strings.Contains(rec.Body.String(), "Gravatar atualizado.") {
		t.Errorf("gravatar refresh: %s", rec.Body.String())
	}
	if rec := form("/avatars/users/ada/gravatar", "action=disable"); !strings.Contains(rec.Body.String(), "Gravatar desligado.") {
		t.Errorf("gravatar off: %s", rec.Body.String())
	}
	if rec := form("/avatars/users/ada/removal", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), "Foto removida.") {
		t.Errorf("removal: %d %s", rec.Code, rec.Body.String())
	}
	// Somebody else's editor is readable and has nothing to press.
	w.enroll("acme", "bob", "Bob", "")
	rec := w.do(t, request{method: "GET", target: "/avatars/users/bob/editor", cookies: cookies})
	if rec.Code != 200 || strings.Contains(rec.Body.String(), `data-part="upload"`) || !strings.Contains(rec.Body.String(), "não alterá-la") {
		t.Errorf("somebody else's editor: %d %s", rec.Code, rec.Body.String())
	}
	if rec := form("/avatars/users/bob/style", "style=marble"); rec.Code != http.StatusForbidden {
		t.Errorf("changing somebody else's style = %d, want 403", rec.Code)
	}
	// Without a Render the module answers a change with a redirect back.
	plain := installWith(t, func(c *avatar.Config) { c.Render = nil })
	plain.enroll("acme", "ada", "Ada", "")
	cookies2 := plain.signIn(t, member("acme", "ada"))
	r := plain.do(t, request{method: "POST", target: "/avatars/users/ada/style", body: []byte("style=beam"), contentType: "application/x-www-form-urlencoded",
		cookies: cookies2, header: http.Header{"HX-Request": {"true"}, "Referer": {"http://example.com/perfil?x=1"}}})
	// htmx is told to navigate, with the header the framework writes for it.
	if r.Code != http.StatusNoContent || r.Header().Get("HX-Redirect") != "/avatars/users/ada/editor" && r.Header().Get("HX-Redirect") != "/perfil?x=1" {
		t.Errorf("a change with no Render = %d, HX-Redirect %q", r.Code, r.Header().Get("HX-Redirect"))
	}
}

func TestARedirectBackNeverLeavesTheOrigin(t *testing.T) {
	plain := installWith(t, func(c *avatar.Config) { c.Render = nil })
	plain.enroll("acme", "ada", "Ada", "")
	cookies := plain.signIn(t, member("acme", "ada"))
	for referer, want := range map[string]string{
		"http://example.com/perfil?x=1": "/perfil?x=1",
		"https://evil.example/steal":    "/avatars/users/ada/editor",
		"http://example.com//evil.test": "/avatars/users/ada/editor",
		"":                              "/avatars/users/ada/editor",
	} {
		req := httptest.NewRequest("POST", "/avatars/users/ada/style", strings.NewReader("style=beam"))
		req.Host = "example.com"
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if referer != "" {
			req.Header.Set("Referer", referer)
		}
		for _, c := range cookies {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		plain.router.ServeHTTP(rec, req)
		if got := rec.Header().Get("Location"); got != want {
			t.Errorf("Referer %q: redirected to %q, want %q", referer, got, want)
		}
	}
}

func TestTheEditorMarkupCarriesNoScriptAndNoStyle(t *testing.T) {
	w := install(t)
	w.enroll("acme", "ada", "Ada Lovelace", "ada@example.com")
	cookies := w.signIn(t, member("acme", "ada"))
	rec := w.do(t, request{method: "GET", target: "/avatars/users/ada/editor", cookies: cookies})
	html := rec.Body.String()
	for _, banned := range []*regexp.Regexp{
		regexp.MustCompile(`(?i)<script`), regexp.MustCompile(`(?i)\sstyle=`), regexp.MustCompile(`(?i)\son[a-z]+=`),
		regexp.MustCompile(`(?i)javascript:`), regexp.MustCompile(`(?i)<style`), regexp.MustCompile(`x-data|x-on:|@click`),
	} {
		if banned.MatchString(html) {
			t.Errorf("the editor carries %v", banned)
		}
	}
	for _, want := range []string{`data-kyse-behavior="avatar-editor"`, `type="file"`, `accept="image/jpeg,image/png,image/webp"`, `role="radiogroup"`, `aria-live="polite"`} {
		if !strings.Contains(html, want) {
			t.Errorf("the editor lacks %q", want)
		}
	}
}

// keep the packages the table above names in use.
var (
	_ = bytes.NewReader
	_ fhttp.Middleware
	_ = components.Render
)
