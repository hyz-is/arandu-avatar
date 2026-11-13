package unit_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"

	avatar "github.com/hyz-is/arandu-avatar"
)

func TestTheConfigurationRefusesWhatCannotWork(t *testing.T) {
	t.Parallel()

	nilMiddleware := []func(http.Handler) http.Handler{nil}
	for name, tc := range map[string]struct {
		cfg  avatar.Config
		want string
	}{
		"team tenant that is not one":     {avatar.Config{TeamTenant: "No Spaces!"}, "TeamTenant"},
		"prefix with no slash":            {avatar.Config{Prefix: "avatars"}, "Prefix"},
		"prefix with a trailing slash":    {avatar.Config{Prefix: "/avatars/"}, "Prefix"},
		"prefix in the framework's":       {avatar.Config{Prefix: "/_arandu/x"}, "framework keeps"},
		"team prefix with no slash":       {avatar.Config{TeamPrefix: "admin"}, "TeamPrefix"},
		"a segment with a wildcard":       {avatar.Config{Paths: avatar.Paths{Upload: "/{id}"}}, "Paths.Upload"},
		"a segment with no slash":         {avatar.Config{Paths: avatar.Paths{Style: "style"}}, "Paths.Style"},
		"two segments alike":              {avatar.Config{Paths: avatar.Paths{Style: "/photo"}}, "cannot be registered"},
		"nil middleware":                  {avatar.Config{Middleware: nilMiddleware}, "Middleware[0]"},
		"nil team middleware":             {avatar.Config{TeamMiddleware: nilMiddleware}, "TeamMiddleware[0]"},
		"an unknown source":               {avatar.Config{Chain: []avatar.Source{"carrier-pigeon", avatar.SourceGenerated}}, "carrier-pigeon"},
		"a chain with a source twice":     {avatar.Config{Chain: []avatar.Source{avatar.SourceUpload, avatar.SourceUpload, avatar.SourceGenerated}}, "twice"},
		"a chain that cannot end":         {avatar.Config{Chain: []avatar.Source{avatar.SourceUpload, avatar.SourceGravatar}}, "has to end"},
		"a chain that ends early":         {avatar.Config{Chain: []avatar.Source{avatar.SourceGenerated, avatar.SourceUpload}}, "before the end"},
		"a style that does not exist":     {avatar.Config{DefaultStyle: "cubist"}, "DefaultStyle"},
		"styles with one that is unknown": {avatar.Config{Styles: []avatar.Style{"cubist"}}, "Styles"},
		"a default outside the styles":    {avatar.Config{Styles: []avatar.Style{avatar.StyleBeam}}, "not in Config.Styles"},
		"a palette of one colour":         {avatar.Config{Palette: []string{"#fff"}}, "between 2 and 16"},
		"a palette entry that is markup":  {avatar.Config{Palette: []string{"#fff", `"><script>`}}, "Palette"},
		"sizes too many":                  {avatar.Config{Sizes: []int{16, 32, 64, 128, 256}}, "most it keeps"},
		"sizes too small":                 {avatar.Config{Sizes: []int{4}}, "between"},
		"sizes too large":                 {avatar.Config{Sizes: []int{4096}}, "between"},
		"sizes that do not ascend":        {avatar.Config{Sizes: []int{64, 64}}, "ascend"},
		"an upload ceiling over the max":  {avatar.Config{MaxUploadBytes: avatar.MaxUploadBytesLimit + 1}, "MaxUploadBytes"},
		"a negative upload ceiling":       {avatar.Config{MaxUploadBytes: -1}, "MaxUploadBytes"},
		"pixels over the max":             {avatar.Config{MaxPixels: avatar.MaxPixelsLimit + 1}, "MaxPixels"},
		"a quality over 100":              {avatar.Config{JPEGQuality: 101}, "JPEGQuality"},
		"a negative cache lifetime":       {avatar.Config{CacheMaxAge: -time.Second}, "CacheMaxAge"},
		"a Gravatar over plain http":      {avatar.Config{GravatarURL: "http://gravatar.example/avatar"}, "https"},
		"a Gravatar address with a query": {avatar.Config{GravatarURL: "https://gravatar.com/avatar?d=x"}, "GravatarURL"},
	} {
		err := tc.cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to mention %q", name, err, tc.want)
		}
	}
	for name, cfg := range map[string]avatar.Config{
		"zero":                   {},
		"gravatar on loopback":   {GravatarURL: "http://127.0.0.1:9/avatar"},
		"a brand palette":        {Palette: []string{"#0a0a0a", "#00ff88", "#2563EB"}},
		"portuguese":             {Paths: avatar.PortuguesePaths, Prefix: "/avatares"},
		"every source reordered": {Chain: []avatar.Source{avatar.SourceGravatar, avatar.SourceUpload, avatar.SourceGenerated}},
		"just the drawing":       {Chain: []avatar.Source{avatar.SourceGenerated}},
		"no team":                {TeamTenant: ""},
	} {
		if err := cfg.Validate(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestNewRefusesAWiringThatCannotWork(t *testing.T) {
	t.Parallel()

	sessions := security.NewSessionStore([]byte("0123456789abcdef0123456789abcdef"), time.Hour, false, security.NewMemoryBackend())
	if _, err := avatar.New(avatar.Config{}, nil, sessions); err == nil || !strings.Contains(err.Error(), "database handle") {
		t.Errorf("no database: %v", err)
	}
	if _, err := avatar.New(avatar.Config{}, data.Wrap(nil, data.DialectSQLite), nil); err == nil || !strings.Contains(err.Error(), "session store") {
		t.Errorf("no sessions: %v", err)
	}
	if m, err := avatar.New(avatar.Config{}, data.Wrap(nil, data.DialectSQLite), sessions); err != nil || m.Name() != "avatar" {
		t.Errorf("a zero configuration: %v", err)
	}
}

func TestTheDefaultsAreTheConstantsTheyAreDocumentedAs(t *testing.T) {
	t.Parallel()

	if avatar.DefaultMaxUploadBytes != 5<<20 || avatar.DefaultPrefix != "/avatars" || avatar.DefaultStyle != avatar.StyleInitials {
		t.Error("a default moved")
	}
	if len(avatar.DefaultSizes) != 3 || len(avatar.DefaultChain) != 3 || avatar.DefaultChain[2] != avatar.SourceGenerated {
		t.Errorf("sizes %v chain %v", avatar.DefaultSizes, avatar.DefaultChain)
	}
	if got := len(avatar.Styles()); got < 10 {
		t.Errorf("%d styles", got)
	}
}

func TestARefIsOnlyWhatCanBeAnOwner(t *testing.T) {
	t.Parallel()

	good := []avatar.Ref{avatar.User("01HZX"), avatar.User("a-b_c.d:e"), avatar.Organization("acme"), avatar.User(strings.Repeat("a", 64))}
	bad := []avatar.Ref{
		avatar.User(""), avatar.User("a/b"), avatar.User("a b"), avatar.User("../x"), avatar.User("a?b=c"), avatar.User("a%2fb"),
		avatar.User(strings.Repeat("a", 65)), {Kind: "team", ID: "x"}, {ID: "x"}, avatar.User("ü"),
	}
	for _, r := range good {
		if !r.Valid() {
			t.Errorf("%v is refused", r)
		}
	}
	for _, r := range bad {
		if r.Valid() {
			t.Errorf("%v is accepted", r)
		}
	}
}
