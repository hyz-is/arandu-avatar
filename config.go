package avatar

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/filesystem"
	"github.com/arandu-io/hesape/translation"
	activitylog "github.com/hyz-is/arandu-activitylog"

	"github.com/hyz-is/arandu-avatar/internal/art"
)

// The defaults for the optional settings. They are constants rather than
// literals inside Config.withDefaults, so the value a reader finds here is the
// value the package uses.
const (
	// DefaultPrefix is where the routes are mounted when Config leaves Prefix
	// empty.
	DefaultPrefix = "/avatars"
	// DefaultTeamPrefix is where the platform team's routes are mounted when
	// Config leaves TeamPrefix empty.
	DefaultTeamPrefix = "/admin/avatars"
	// DefaultLocale is the language of the editor, the messages and the activity
	// descriptions when Config.Locale names none.
	DefaultLocale = "pt-BR"
	// FallbackLocale answers every line the locale in use does not have.
	FallbackLocale = "en"
	// DefaultStyle is the generated style of an owner who chose none.
	DefaultStyle = StyleInitials
	// DefaultMaxUploadBytes is the size of the largest photo a person sends when
	// Config leaves MaxUploadBytes at zero.
	DefaultMaxUploadBytes = 5 << 20
	// MaxUploadBytesLimit is the ceiling MaxUploadBytes is refused above: the
	// file is read into memory to be looked at before it is kept.
	MaxUploadBytesLimit = 20 << 20
	// DefaultMaxPixels is the most pixels a photo may declare when Config leaves
	// MaxPixels at zero: about thirty-six megapixels, which a phone camera does
	// not pass and which decodes in a bounded amount of memory.
	DefaultMaxPixels = 36_000_000
	// MaxPixelsLimit is the ceiling MaxPixels is refused above.
	MaxPixelsLimit = 100_000_000
	// DefaultJPEGQuality is the quality photos are encoded at.
	DefaultJPEGQuality = 85
	// DefaultCacheMaxAge is how long a browser or a CDN may keep a picture, which
	// is as long as it may: the address changes when the picture does.
	DefaultCacheMaxAge = 365 * 24 * time.Hour
	// DefaultGravatarURL is where a Gravatar is fetched from.
	DefaultGravatarURL = "https://gravatar.com/avatar"
	// DefaultGravatarTimeout is how long the one request to Gravatar may take.
	DefaultGravatarTimeout = 5 * time.Second
	// MaxSizes is how many sizes a photo is kept at.
	MaxSizes = 4
	// MaxSize is the largest side a photo is kept at.
	MaxSize = 1024
	// MinSize is the smallest side a photo is kept at.
	MinSize = 16
)

// DefaultSizes are the sides, in pixels, a photo is kept at: a list row, a
// profile menu and the editor. The browser picks one from the srcset for the
// size the avatar is drawn at and the density of the screen.
var DefaultSizes = []int{64, 128, 256}

// DefaultChain is the order a picture is looked for: the photo the person sent,
// then their Gravatar when they switched it on, then a drawing.
var DefaultChain = []Source{SourceUpload, SourceGravatar, SourceGenerated}

// Config is what the application passes when it wires this package.
//
// A typed struct rather than a map: a misspelled key in a map is a setting that
// silently keeps its default, and the failure shows up as behaviour nobody asked
// for rather than as an error. Here a field that does not exist does not
// compile.
//
// It names no tenant for the members of a space, and that is deliberate. Every
// route sits behind RequireAuth, so the tenant of every statement is the one the
// Grant carries, from the session of whoever is asking; a tenant written here
// would be a second place for one to come from. The one tenant it does name is
// the team's own, TeamTenant, because the team is not a customer.
type Config struct {
	// TeamTenant is the tenant the platform's own team signs in to. A team
	// member is a subject of this tenant who holds the team's actions. Empty
	// turns the team's side off: no team route is registered and the team
	// service refuses everybody.
	TeamTenant string

	// Prefix is the path the routes are mounted under. Empty means
	// DefaultPrefix.
	Prefix string

	// TeamPrefix is the path the team's routes are mounted under. Empty means
	// DefaultTeamPrefix.
	TeamPrefix string

	// Paths are the segments every route is built from, under Prefix and
	// TeamPrefix. A field left empty keeps its DefaultPaths value;
	// PortuguesePaths is the whole set in Brazilian Portuguese.
	Paths Paths

	// Middleware runs on every route of this package after RequireAuth, in
	// order, and TeamMiddleware after it on the team's routes only. An
	// application whose permissions arandu-permission administers mounts its
	// Roles() in Middleware: RequireAuth puts the subject as the session
	// resolved it, without the actions arandu-permission resolves on each
	// request. A guard answers "is this request on the right side" and stops;
	// the policy still decides.
	Middleware     []func(http.Handler) http.Handler
	TeamMiddleware []func(http.Handler) http.Handler

	// Directory names who an avatar is for and says whether they exist. It
	// belongs to the application, which owns its users and its organizations.
	// Nil limits the editor to the signed-in person's own avatar, drawn with no
	// name and with no e-mail, so no Gravatar can be looked up.
	Directory Directory

	// Files is the Disk the photos are kept on: the application's, which in
	// production is a private bucket. Every key is generated by the server and
	// every read goes through an authorized route; nothing is served by a public
	// address. Nil turns uploads and Gravatar off, and the avatar is drawn from
	// the name alone.
	Files *filesystem.Disk

	// Chain is the order the sources of a picture are tried in, ending in
	// SourceGenerated, which cannot fail. Leaving SourceGravatar out turns
	// Gravatar off for the application. Empty means DefaultChain.
	Chain []Source

	// DefaultStyle is the generated style of an owner who chose none. Empty
	// means DefaultStyle.
	DefaultStyle Style

	// Styles are the generated styles the editor offers. Empty offers every
	// style. DefaultStyle has to be one of them.
	Styles []Style

	// Palette is the colours a generated avatar is drawn with, as #rgb or
	// #rrggbb: an application sets its brand's here. Empty means the palette of
	// the package.
	Palette []string

	// Sizes are the sides, in pixels, a photo is kept at, ascending, up to
	// MaxSizes of them between MinSize and MaxSize. Empty means DefaultSizes.
	Sizes []int

	// MaxUploadBytes is the size of the largest photo a person sends. Zero means
	// DefaultMaxUploadBytes, and anything above MaxUploadBytesLimit is refused.
	MaxUploadBytes int64

	// MaxPixels is the most pixels a photo may declare, width times height,
	// checked from the header before the picture is decoded. Zero means
	// DefaultMaxPixels, and anything above MaxPixelsLimit is refused.
	MaxPixels int

	// JPEGQuality is the quality photos are encoded at, 1 to 100. Zero means
	// DefaultJPEGQuality.
	JPEGQuality int

	// PublicCache says a CDN in front of the application may keep the pictures:
	// they are answered with Cache-Control public. The default is private, which
	// keeps them in the browser alone, because a shared cache serves a picture
	// without asking the application who is looking. A picture's address holds
	// the owner, a version and a size, so turning this on exposes only what
	// somebody who already has the address could fetch.
	PublicCache bool

	// CacheMaxAge is how long a picture may be kept by whoever caches it. Zero
	// means DefaultCacheMaxAge.
	CacheMaxAge time.Duration

	// GravatarURL is where Gravatar is fetched from. It has to be https, except
	// for a loopback address, which a test serves. Empty means
	// DefaultGravatarURL.
	GravatarURL string

	// HTTPClient makes the one request to Gravatar. Nil means a client with
	// DefaultGravatarTimeout that follows a redirect only inside the host it was
	// pointed at and Gravatar's own domain. An application passes its own to go
	// through a proxy.
	HTTPClient *http.Client

	// Render draws the editor the module answers a request with. Pass
	// components.Render from github.com/hyz-is/arandu-avatar/components. Nil
	// makes every change answer with a redirect back to where it came from.
	Render Renderer

	// Activity is arandu-activitylog's logger, the application's one instance
	// (activityModule.Logger()). Every change and removal of an avatar is
	// recorded under the Grant it was authorized with, in the log named
	// ActivityLogName. Nil records nothing.
	Activity *activitylog.Logger

	// Translator is the application's own catalogue, asked before the one this
	// package ships, key by key: "avatar.editor.title". Optional.
	Translator *translation.Translator

	// Locale is the language the editor, the validation messages and the
	// activity descriptions of this package are written in. Empty means
	// DefaultLocale. The package ships the locales Locales() lists; any other
	// reads the application's Translator first and English after it.
	Locale string

	// Now is the clock: every timestamp a row carries is read from it. Nil means
	// time.Now. A test holds it still; an application leaves it nil.
	Now func() time.Time

	// Logger receives what the package cannot return to anybody: a file that
	// could not be removed after the row that named it was changed. Nil means
	// slog.Default().
	Logger *slog.Logger
}

// Paths are the segments the routes of this package are built from. Each one
// starts with "/" and holds no wildcard.
//
//	GET  Prefix + Users + /{id}/{version}/{size}   the picture
//	GET  Prefix + Users + /{id} + Editor           the editor, as a fragment
//	POST Prefix + Users + /{id} + Upload           send a photo
//	POST Prefix + Users + /{id} + Gravatar         switch Gravatar on, refresh it, switch it off
//	POST Prefix + Users + /{id} + Style            choose the generated style
//	POST Prefix + Users + /{id} + Removal          remove the photo
//
// and the same under Organizations. Under TeamPrefix the picture and the
// removal.
type Paths struct {
	Users, Organizations                     string
	Editor, Upload, Gravatar, Style, Removal string
}

// DefaultPaths are the English segments.
var DefaultPaths = Paths{
	Users: "/users", Organizations: "/organizations",
	Editor: "/editor", Upload: "/photo", Gravatar: "/gravatar", Style: "/style", Removal: "/removal",
}

// PortuguesePaths are the segments in Brazilian Portuguese, for an application
// whose addresses are written in it. Mounted at "/avatares" they read
// /avatares/pessoas/{id}/foto and /avatares/espacos/{id}/remocao.
var PortuguesePaths = Paths{
	Users: "/pessoas", Organizations: "/espacos",
	Editor: "/editor", Upload: "/foto", Gravatar: "/gravatar", Style: "/estilo", Removal: "/remocao",
}

// fields are the segments with their names, for validating and filling them.
func (p *Paths) fields() []struct {
	name  string
	value *string
} {
	return []struct {
		name  string
		value *string
	}{
		{"Users", &p.Users}, {"Organizations", &p.Organizations}, {"Editor", &p.Editor}, {"Upload", &p.Upload},
		{"Gravatar", &p.Gravatar}, {"Style", &p.Style}, {"Removal", &p.Removal},
	}
}

// validate reports a segment that cannot be part of a route.
func (p Paths) validate() error {
	seen := map[string]string{}
	for _, f := range p.fields() {
		v := *f.value
		if v == "" {
			continue
		}
		if v[0] != '/' || strings.ContainsAny(v, "{} \t?#") || v == "/" || strings.HasSuffix(v, "/") {
			return fmt.Errorf("avatar: Config.Paths.%s is %q: a segment starts with /, ends without one and holds no wildcard", f.name, v)
		}
		if other, dup := seen[v]; dup {
			return fmt.Errorf("avatar: Config.Paths.%s and Config.Paths.%s are both %q", other, f.name, v)
		}
		seen[v] = f.name
	}
	return nil
}

// withDefaults fills every empty segment from DefaultPaths.
func (p Paths) withDefaults() Paths {
	d := DefaultPaths
	defaults := d.fields()
	for i, f := range p.fields() {
		if *f.value == "" {
			*f.value = *defaults[i].value
		}
	}
	return p
}

// reservedPrefix is the namespace the framework keeps for itself.
const reservedPrefix = "/_arandu"

// Validate reports what the configuration cannot be used with.
//
// It is called by New, so an application with a setting that cannot work fails
// where it is wired rather than on the first request that needed it.
func (c Config) Validate() error {
	if c.TeamTenant != "" && !security.ValidTenant(c.TeamTenant) {
		return fmt.Errorf("avatar: Config.TeamTenant is %q, which cannot be a tenant: pass the tenant the platform team signs in to (lowercase letters, digits, - and _, up to 64 characters), or leave it empty to turn the team's side off", c.TeamTenant)
	}
	for _, prefix := range []struct{ name, value string }{{"Prefix", c.Prefix}, {"TeamPrefix", c.TeamPrefix}} {
		if prefix.value == "" {
			continue
		}
		if prefix.value[0] != '/' || (len(prefix.value) > 1 && strings.HasSuffix(prefix.value, "/")) {
			return fmt.Errorf("avatar: Config.%s is %q and has to start with / and end without one", prefix.name, prefix.value)
		}
		if prefix.value == reservedPrefix || strings.HasPrefix(prefix.value, reservedPrefix+"/") {
			return fmt.Errorf("avatar: Config.%s is %q, under %s/, which the framework keeps for itself", prefix.name, prefix.value, reservedPrefix)
		}
	}
	if err := c.Paths.validate(); err != nil {
		return err
	}
	for i, mw := range c.Middleware {
		if mw == nil {
			return fmt.Errorf("avatar: Config.Middleware[%d] is nil", i)
		}
	}
	for i, mw := range c.TeamMiddleware {
		if mw == nil {
			return fmt.Errorf("avatar: Config.TeamMiddleware[%d] is nil", i)
		}
	}
	if err := validateChain(c.Chain); err != nil {
		return err
	}
	if c.DefaultStyle != "" && !art.Valid(c.DefaultStyle) {
		return fmt.Errorf("avatar: Config.DefaultStyle is %q, which is not one of %v", c.DefaultStyle, Styles())
	}
	for _, s := range c.Styles {
		if !art.Valid(s) {
			return fmt.Errorf("avatar: Config.Styles holds %q, which is not one of %v", s, Styles())
		}
	}
	if len(c.Styles) > 0 {
		def := c.DefaultStyle
		if def == "" {
			def = DefaultStyle
		}
		if !slices.Contains(c.Styles, def) {
			return fmt.Errorf("avatar: Config.DefaultStyle is %q and is not in Config.Styles", def)
		}
	}
	if len(c.Palette) > 0 {
		if len(c.Palette) < 2 || len(c.Palette) > 16 {
			return fmt.Errorf("avatar: Config.Palette holds %d colours, and has to hold between 2 and 16", len(c.Palette))
		}
		if _, err := art.NormalizePalette(c.Palette); err != nil {
			return fmt.Errorf("avatar: Config.Palette: %w", err)
		}
	}
	if err := validateSizes(c.Sizes); err != nil {
		return err
	}
	if c.MaxUploadBytes < 0 || c.MaxUploadBytes > MaxUploadBytesLimit {
		return fmt.Errorf("avatar: Config.MaxUploadBytes is %d, and has to be between 0 and %d, where 0 means %d", c.MaxUploadBytes, MaxUploadBytesLimit, DefaultMaxUploadBytes)
	}
	if c.MaxPixels < 0 || c.MaxPixels > MaxPixelsLimit {
		return fmt.Errorf("avatar: Config.MaxPixels is %d, and has to be between 0 and %d, where 0 means %d", c.MaxPixels, MaxPixelsLimit, DefaultMaxPixels)
	}
	if c.JPEGQuality < 0 || c.JPEGQuality > 100 {
		return fmt.Errorf("avatar: Config.JPEGQuality is %d, and has to be between 0 and 100, where 0 means %d", c.JPEGQuality, DefaultJPEGQuality)
	}
	if c.CacheMaxAge < 0 {
		return fmt.Errorf("avatar: Config.CacheMaxAge is %s and cannot be negative", c.CacheMaxAge)
	}
	if c.GravatarURL != "" {
		if err := validGravatarURL(c.GravatarURL); err != nil {
			return err
		}
	}
	return validateRoutes(c.withDefaults())
}

func validateChain(chain []Source) error {
	if len(chain) == 0 {
		return nil
	}
	seen := map[Source]bool{}
	for i, s := range chain {
		switch s {
		case SourceUpload, SourceGravatar, SourceGenerated:
		default:
			return fmt.Errorf("avatar: Config.Chain holds %q, which is not %q, %q or %q", s, SourceUpload, SourceGravatar, SourceGenerated)
		}
		if seen[s] {
			return fmt.Errorf("avatar: Config.Chain holds %q twice", s)
		}
		seen[s] = true
		if s == SourceGenerated && i != len(chain)-1 {
			return fmt.Errorf("avatar: Config.Chain has %q before the end, and nothing after it could ever be reached", SourceGenerated)
		}
	}
	if chain[len(chain)-1] != SourceGenerated {
		return fmt.Errorf("avatar: Config.Chain has to end in %q, the one source that cannot fail", SourceGenerated)
	}
	return nil
}

func validateSizes(sizes []int) error {
	if len(sizes) == 0 {
		return nil
	}
	if len(sizes) > MaxSizes {
		return fmt.Errorf("avatar: Config.Sizes holds %d sizes, and the most it keeps is %d", len(sizes), MaxSizes)
	}
	for i, s := range sizes {
		if s < MinSize || s > MaxSize {
			return fmt.Errorf("avatar: Config.Sizes[%d] is %d, and has to be between %d and %d", i, s, MinSize, MaxSize)
		}
		if i > 0 && s <= sizes[i-1] {
			return fmt.Errorf("avatar: Config.Sizes has to ascend, and %d does not follow %d", s, sizes[i-1])
		}
	}
	return nil
}

func validGravatarURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("avatar: Config.GravatarURL is %q, which is not an address without a query", raw)
	}
	switch {
	case u.Scheme == "https":
	case u.Scheme == "http" && isLoopback(u.Hostname()):
	default:
		return fmt.Errorf("avatar: Config.GravatarURL is %q, and has to be https: the e-mail's digest travels in it", raw)
	}
	return nil
}

func isLoopback(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// validateRoutes asks the standard library to parse the exact patterns the
// module will register, every one of them. Its parser is not exported and
// reports an invalid pattern, or two that collide, by panic, so the throwaway
// mux turns that boot-time panic into the configuration error New promises.
func validateRoutes(c Config) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("avatar: Config.Prefix %q, Config.TeamPrefix %q and Config.Paths cannot be registered as routes: %v", c.Prefix, c.TeamPrefix, recovered)
		}
	}()

	handler := http.NotFoundHandler()
	mux := http.NewServeMux()
	for _, route := range routeTable(c) {
		mux.Handle(route.method+" "+route.path, handler)
	}
	return nil
}

// withDefaults returns the configuration with the optional fields filled in.
//
// It runs after Validate and never before: filling a default in first would hide
// the value somebody actually wrote from the check that would have refused it.
func (c Config) withDefaults() Config {
	if c.Prefix == "" {
		c.Prefix = DefaultPrefix
	}
	if c.TeamPrefix == "" {
		c.TeamPrefix = DefaultTeamPrefix
	}
	c.Paths = c.Paths.withDefaults()
	if len(c.Chain) == 0 {
		c.Chain = slices.Clone(DefaultChain)
	}
	if c.DefaultStyle == "" {
		c.DefaultStyle = DefaultStyle
	}
	if len(c.Styles) == 0 {
		c.Styles = Styles()
	}
	if len(c.Palette) == 0 {
		c.Palette = slices.Clone(art.DefaultPalette)
	} else if normal, err := art.NormalizePalette(c.Palette); err == nil {
		c.Palette = normal
	}
	if len(c.Sizes) == 0 {
		c.Sizes = slices.Clone(DefaultSizes)
	}
	if c.MaxUploadBytes == 0 {
		c.MaxUploadBytes = DefaultMaxUploadBytes
	}
	if c.MaxPixels == 0 {
		c.MaxPixels = DefaultMaxPixels
	}
	if c.JPEGQuality == 0 {
		c.JPEGQuality = DefaultJPEGQuality
	}
	if c.CacheMaxAge == 0 {
		c.CacheMaxAge = DefaultCacheMaxAge
	}
	if c.GravatarURL == "" {
		c.GravatarURL = DefaultGravatarURL
	}
	c.GravatarURL = strings.TrimRight(c.GravatarURL, "/")
	if c.Locale == "" {
		c.Locale = DefaultLocale
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	return c
}

// gravatarOn says Gravatar is part of the chain and can be kept somewhere.
func (c Config) gravatarOn() bool {
	return c.Files != nil && slices.Contains(c.Chain, SourceGravatar)
}

// uploadOn says photos can be kept.
func (c Config) uploadOn() bool {
	return c.Files != nil && slices.Contains(c.Chain, SourceUpload)
}

// route is one line of the table this module registers.
type route struct {
	name   string
	method string
	path   string
	team   bool
	kind   Kind
	action string
}

// segment is the path segment of a kind of owner.
func (p Paths) segment(k Kind) string {
	if k == KindOrganization {
		return p.Organizations
	}
	return p.Users
}

// routeTable is the whole table, built from the prefixes and the segments. It
// is read by validateRoutes, which registers it on a throwaway mux, and by the
// router.
func routeTable(c Config) []route {
	p, t, s := c.Prefix, c.TeamPrefix, c.Paths
	var out []route
	for _, k := range Kinds {
		base := p + s.segment(k) + "/{id}"
		out = append(out,
			route{"avatar." + string(k) + ".show", http.MethodGet, base + "/{version}/{size}", false, k, "show"},
			route{"avatar." + string(k) + ".editor", http.MethodGet, base + s.Editor, false, k, "editor"},
			route{"avatar." + string(k) + ".upload", http.MethodPost, base + s.Upload, false, k, "upload"},
			route{"avatar." + string(k) + ".gravatar", http.MethodPost, base + s.Gravatar, false, k, "gravatar"},
			route{"avatar." + string(k) + ".style", http.MethodPost, base + s.Style, false, k, "style"},
			route{"avatar." + string(k) + ".remove", http.MethodPost, base + s.Removal, false, k, "remove"},
		)
		if c.TeamTenant != "" {
			tb := t + s.segment(k) + "/{id}"
			out = append(out,
				route{"avatar.team." + string(k) + ".show", http.MethodGet, tb + "/{version}/{size}", true, k, "show"},
				route{"avatar.team." + string(k) + ".remove", http.MethodPost, tb + s.Removal, true, k, "remove"},
			)
		}
	}
	return out
}
