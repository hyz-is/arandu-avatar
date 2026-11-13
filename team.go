package avatar

import (
	"context"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"

	"github.com/hyz-is/arandu-avatar/internal/platform"
)

// TeamService orchestrates what the platform team does with the avatars of every
// space.
//
// A member of the team is a subject of Config.TeamTenant, and TeamPolicy decides
// what they may do. The avatars are in the customers' tenants, so a use case here
// goes in two steps: it asks the policy for a Grant in the team's own realm,
// finds the avatar's tenant with it, and then acts with a Grant scoped to that
// one tenant, which exists only because the first one was issued for the same
// action. Nothing a request names chooses the tenant: it is the one the row says.
// The one value a request may add is Space, to tell apart the rows of one owner
// that exist in two spaces, or to name the space of an owner who has no row yet;
// it narrows a look-up and is never a Grant.
type TeamService struct {
	*core
	policy TeamPolicy
}

// NewTeamService wires the service over the application's database handle.
func NewTeamService(db *data.DB, cfg Config) *TeamService {
	cfg = cfg.withDefaults()
	return &TeamService{core: newCore(db, cfg), policy: TeamPolicy{Realm: cfg.TeamTenant}}
}

// TeamOwner is an owner of a space the team looks at: the owner, and the space
// they are in when the application knows it.
type TeamOwner struct {
	Owner
	// Space is the tenant of the owner. Empty takes the first space that has a
	// row for them.
	Space string
}

// Resolve is how the owners are drawn for the team, with addresses under the
// team's own prefix, which the team's session can read. An owner with no row is
// drawn from their name.
func (s *TeamService) Resolve(ctx context.Context, actor security.Subject, owners ...TeamOwner) (Pictures, error) {
	realm, err := security.Authorize(ctx, s.policy, actor, AvatarViewAll, Avatar{})
	if err != nil {
		return nil, err
	}
	byKind := map[Kind][]any{}
	for _, o := range owners {
		if o.Ref.Valid() {
			byKind[o.Kind] = append(byKind[o.Kind], o.ID)
		}
	}
	type key struct {
		ref   Ref
		space string
	}
	rows := map[key]*Avatar{}
	first := map[Ref]*Avatar{}
	for kind, ids := range byKind {
		found, err := platform.Avatars(s.db).Where("owner_kind", "=", string(kind)).WhereIn("owner_id", ids).Get(ctx, realm, platform.Columns...)
		if err != nil {
			return nil, err
		}
		for _, row := range found {
			snapshot := fromPlatform(row)
			ref := Ref{Kind: kind, ID: row.OwnerID}
			rows[key{ref, row.TenantID}] = snapshot
			if first[ref] == nil {
				first[ref] = snapshot
			}
		}
	}
	out := make(Pictures, len(owners))
	for _, o := range owners {
		row := first[o.Ref]
		if o.Space != "" {
			row = rows[key{o.Ref, o.Space}]
		}
		space := o.Space
		if space == "" && row != nil {
			space = row.TenantID
		}
		out[o.Ref] = buildPicture(s.cfg, o.Owner, row, true, space)
	}
	return out, nil
}

// Open reads the picture an address names, for the team. See AvatarService.Open.
func (s *TeamService) Open(ctx context.Context, actor security.Subject, ref Ref, space, version string, size int, ifNoneMatch string) (Opened, error) {
	g, row, _, err := s.reach(ctx, actor, AvatarViewAll, ref, space)
	if err != nil {
		return Opened{}, err
	}
	if row == nil {
		return Opened{}, ErrNotFound
	}
	return s.core.open(ctx, g, row, version, size, ifNoneMatch, true, row.TenantID)
}

// Upload keeps a photo as the owner's picture, for the team.
func (s *TeamService) Upload(ctx context.Context, actor security.Subject, ref Ref, space string, in UploadRequest) (*Avatar, error) {
	g, row, owner, err := s.reach(ctx, actor, AvatarManageAll, ref, space)
	if err != nil {
		return nil, err
	}
	return s.core.upload(ctx, g, actor, owner, row, in)
}

// Remove removes the owner's photo, for the team: what moderation does with a
// picture that should not be there.
func (s *TeamService) Remove(ctx context.Context, actor security.Subject, ref Ref, space string) error {
	g, row, _, err := s.reach(ctx, actor, AvatarManageAll, ref, space)
	if err != nil {
		return err
	}
	return s.core.removePhoto(ctx, g, actor, row)
}

// SetStyle chooses the generated style of an owner, for the team.
func (s *TeamService) SetStyle(ctx context.Context, actor security.Subject, ref Ref, space string, style Style) (*Avatar, error) {
	g, row, _, err := s.reach(ctx, actor, AvatarManageAll, ref, space)
	if err != nil {
		return nil, err
	}
	return s.core.setStyle(ctx, g, actor, ref, row, style)
}

// reach finds the avatar a team member acts on and the Grant to act on it with.
//
// The policy is asked twice, as it is for a member's own: first for the kind of
// act, with nothing in hand, and then about the row. Between them the row is
// found in any tenant -- through the Global table, with the realm's Grant -- and
// its tenant is the one the work is scoped to. An owner with no row yet is
// reached only when the request names the space, and the Directory confirms the
// owner is in it.
func (s *TeamService) reach(ctx context.Context, actor security.Subject, a security.Action, ref Ref, space string) (security.Grant, *Avatar, Owner, error) {
	if !ref.Valid() {
		return security.Grant{}, nil, Owner{}, ErrNotFound
	}
	realm, err := security.Authorize(ctx, s.policy, actor, a, Avatar{})
	if err != nil {
		return security.Grant{}, nil, Owner{}, err
	}
	lookup := platform.Avatars(s.db).Where("owner_kind", "=", string(ref.Kind)).Where("owner_id", "=", ref.ID)
	if space != "" {
		if !security.ValidTenant(space) {
			return security.Grant{}, nil, Owner{}, ErrNotFound
		}
		lookup = lookup.Where("tenant_id", "=", space)
	}
	found, err := lookup.OrderBy("tenant_id").First(ctx, realm, "id", "tenant_id")
	if err != nil {
		return security.Grant{}, nil, Owner{}, err
	}
	tenant := space
	if found != nil {
		tenant = found.TenantID
	}
	if tenant == "" {
		return security.Grant{}, nil, Owner{}, ErrNotFound
	}
	g, err := scopedGrant(realm, a, tenant)
	if err != nil {
		return security.Grant{}, nil, Owner{}, err
	}
	owner, err := s.owner(ctx, g, actor, ref)
	if err != nil {
		return security.Grant{}, nil, Owner{}, err
	}
	row, err := s.row(ctx, g, ref)
	if err != nil {
		return security.Grant{}, nil, Owner{}, err
	}
	probe := candidate(ref)
	if row != nil {
		probe = *row
	}
	if _, err := security.Authorize(ctx, s.policy, actor, a, probe); err != nil {
		return security.Grant{}, nil, Owner{}, err
	}
	return g, row, owner, nil
}

// fromPlatform copies a row read across tenants into a snapshot that holds no
// connection, so nothing read this way can be saved.
func fromPlatform(row *platform.Avatar) *Avatar {
	return &Avatar{
		ID: row.ID, TenantID: row.TenantID, OwnerKind: row.OwnerKind, OwnerID: row.OwnerID, Style: row.Style,
		UploadVersion: row.UploadVersion, UploadType: row.UploadType, UploadSizes: row.UploadSizes,
		GravatarEnabled: Flag(row.GravatarEnabled != 0), GravatarHash: row.GravatarHash,
		GravatarVersion: row.GravatarVersion, GravatarType: row.GravatarType, GravatarSizes: row.GravatarSizes,
	}
}
