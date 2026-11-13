// Package avatar is an Arandu module: the avatar of the people of a multi-tenant
// application, and the logo of its companies' spaces.
//
// An avatar is drawn from the first of the sources the application's chain
// names that has something to show: the photo the person sent, the Gravatar they
// switched on, and a drawing made from their name -- initials, a block pattern,
// a face, marble, pixels, rings, a horizon, Bauhaus shapes, a polygon or a
// silhouette -- which needs no request and always works. A photo is checked by
// what its bytes are, cut to the square the person chose, scaled to a few sizes
// and encoded again, which strips every byte of metadata; it is kept on the
// application's Disk -- in production a private bucket -- under keys the server
// generates, and served by a route of this package with an entity tag and a
// long cache lifetime, at an address whose version changes when the photo does,
// so a CDN in front of the application keeps it.
//
// The editor a person changes it in is a Kyse component, in the components
// package beside this one, and the page draws an avatar anywhere with another.
//
// The files are laid out by role rather than by layer, so the whole package
// reads top to bottom:
//
//	module.go      -> registration, routes and the contract with the framework
//	config.go      -> what the application passes in, and the route table
//	contracts.go   -> what the application implements: the directory of owners
//	model.go       -> the entity, its own rules, and the owner and source types
//	picture.go     -> how an owner is drawn, and the addresses of the photo
//	policy.go      -> who may do what, for a member of a space and for the team
//	service.go     -> the members' use cases, and Model access after authorization
//	team.go        -> the platform team's use cases
//	editor.go      -> what the editor is handed
//	files.go       -> keeping and removing the photos on the application's Disk
//	gravatar.go    -> the one look-up of a Gravatar, made when a person asks for it
//	grants.go      -> the two places a Grant is issued without a signed-in member
//	activity.go    -> what is recorded in arandu-activitylog
//	handlers.go    -> the controllers of both sides
//	assets.go      -> the script and the stylesheet the editor and the image use
//	migrations.go  -> the schema
//
// An application registers it explicitly. There is no service provider, no
// container and no discovery: the wiring is the lines somebody wrote, and
// reading them is how they learn what the application is made of.
package avatar

import (
	"context"
	"errors"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/foundation"
	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/http/middleware"
	"github.com/arandu-io/framework/security"
)

// Module is what the application registers.
//
// It implements foundation.Module, which is Name and Routes and nothing else --
// that pair is the whole public contract between a package and the framework. It
// also implements foundation.Migratable, because it owns a table. The other
// optional interfaces are declared beside Module in the framework and are opted
// into the same way, by implementing them.
type Module struct {
	cfg      Config
	avatars  *AvatarService
	team     *TeamService
	sessions *security.SessionStore
}

// Compile-time proof that the module honors the contracts it claims.
var (
	_ foundation.Module     = (*Module)(nil)
	_ foundation.Migratable = (*Module)(nil)
)

// New returns the module, or the reason it cannot be built.
//
// The collaborators are parameters and not fields somebody fills in afterwards:
// a module that could be registered half-wired is a module whose first request
// is the thing that reports the missing half.
//
// It returns an error rather than panicking or carrying on, because everything
// it refuses is a wiring mistake, and a wiring mistake found at boot costs one
// restart. The same mistake found later is a request that reached a nil handle.
func New(cfg Config, db *data.DB, sessions *security.SessionStore) (*Module, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if db == nil {
		return nil, errors.New("avatar: New needs a database handle: this package owns a table, and there is no in-memory mode that would let it start without one")
	}
	if sessions == nil {
		return nil, errors.New("avatar: New needs a session store: every route is mounted behind RequireAuth, which reads it to admit a request and to put who is asking on it")
	}
	cfg = cfg.withDefaults()
	return &Module{
		cfg:      cfg,
		avatars:  NewAvatarService(db, cfg),
		team:     NewTeamService(db, cfg),
		sessions: sessions,
	}, nil
}

// Name is the module identifier: a lowercase slug, stable, no spaces.
//
// It is what `aru route:list` groups by and what the route names are prefixed
// with, so changing it changes addresses that other code has already written
// down.
func (m *Module) Name() string { return "avatar" }

// Avatars is the members' use cases, for an application that calls them from its
// own code. Every method asks the policy, so handing it out opens nothing.
func (m *Module) Avatars() *AvatarService { return m.avatars }

// Team is the platform team's use cases, for an application that calls them from
// its own code. Every method asks TeamPolicy, so handing it out opens nothing.
func (m *Module) Team() *TeamService { return m.team }

// Resolve is how the owners are drawn, in one query however many there are. It is
// the call a page makes before it draws a list of people. See AvatarService.Resolve.
func (m *Module) Resolve(ctx context.Context, actor security.Subject, owners ...Owner) (Pictures, error) {
	return m.avatars.Resolve(ctx, actor, owners...)
}

// Picture is how one owner is drawn. See AvatarService.Resolve.
func (m *Module) Picture(ctx context.Context, actor security.Subject, owner Owner) (Picture, error) {
	return m.avatars.Picture(ctx, actor, owner)
}

// URL is the address of one owner's picture at a size, for an <img src> or a
// stylesheet: the photo's address when they have a photo, and the drawing as a
// data: address when they do not, so the answer can always be used as an image.
// A page whose policy has no data: in img-src draws the drawing with
// Picture.SVG instead, which needs none.
func (m *Module) URL(ctx context.Context, actor security.Subject, owner Owner, size int) (string, error) {
	p, err := m.avatars.Picture(ctx, actor, owner)
	if err != nil {
		return "", err
	}
	if u := p.URL(size); u != "" {
		return u, nil
	}
	return p.DataURI(), nil
}

// Editor is what the editor component is handed for one owner. See
// AvatarService.Editor.
func (m *Module) Editor(ctx context.Context, actor security.Subject, ref Ref) (Editor, error) {
	return m.avatars.Editor(ctx, actor, ref)
}

// Routes registers every route of the module: the members' and the team's.
//
// An application that keeps the two sides apart -- and the team's session is
// valid on its own side only -- registers them separately with MemberRoutes and
// TeamRoutes, each on the router group its own guards are on.
//
// Every route needs somebody signed in, so the guard is mounted once per side, on
// the router the routes are registered on, rather than route by route: r is
// replaced by the guarded group before the first registration, and no unguarded
// router is left in scope for the next route to be added to. RequireAuth sends a
// request with no session to the sign-in screen, and puts the subject of one that
// has a session on its context, where a handler reads it with ctx.User;
// Config.Middleware runs after it, and Config.TeamMiddleware after that on the
// team's routes.
func (m *Module) Routes(r *fhttp.Router) {
	m.MemberRoutes(r)
	m.TeamRoutes(r)
}

// MemberRoutes registers the members' routes alone, under Prefix.
func (m *Module) MemberRoutes(r *fhttp.Router) { m.register(r, false) }

// TeamRoutes registers the team's routes alone, under TeamPrefix, for an
// application that mounts them inside its operator console with that console's
// guards on r. It registers nothing when Config.TeamTenant is empty.
func (m *Module) TeamRoutes(r *fhttp.Router) { m.register(r, true) }

func (m *Module) register(r *fhttp.Router, team bool) {
	guards := []fhttp.Middleware{middleware.RequireAuth(m.sessions)}
	for _, mw := range m.cfg.Middleware {
		guards = append(guards, mw)
	}
	if team {
		for _, mw := range m.cfg.TeamMiddleware {
			guards = append(guards, mw)
		}
	}
	r = r.Group("", guards...)
	for _, rt := range routeTable(m.cfg) {
		if rt.team != team {
			continue
		}
		r.Action(rt.method, rt.path, m.handler(rt)).Name(rt.name)
	}
}
