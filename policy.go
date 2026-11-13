package avatar

import (
	"context"
	"fmt"

	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/auth"
)

// The actions of this package. Constants rather than strings at the call site:
// a typo in an action name would silently authorize nothing, or worse,
// everything.
//
// They are what an arandu-permission group carries where a group is needed: the
// application declares them in its catalogue (Actions and TeamActions), an
// organization or the platform grants them to whom it decides, and
// arandu-permission fills the subject's actions on every request. A person
// changing their own picture needs no group: it is theirs.
const (
	// AvatarView is reading an avatar. Anybody signed in to the space reads the
	// avatars of the space: the tenant check is the membership.
	AvatarView security.Action = "avatar.view"
	// AvatarUpdate is a person changing, switching and removing their own
	// picture.
	AvatarUpdate security.Action = "avatar.update"
	// AvatarManageOrganization is changing the logo of an organization of the
	// space: for the people who run it.
	AvatarManageOrganization security.Action = "avatar.manage_organization"

	// AvatarViewAll is the platform team reading the avatar of any space.
	AvatarViewAll security.Action = "avatar.view_all"
	// AvatarManageAll is the platform team changing or removing the avatar of
	// any owner of any space.
	AvatarManageAll security.Action = "avatar.manage_all"
)

// Actions are the space's actions, for an application to declare in
// arandu-permission's catalogue beside its own, so an organization grants them
// to whom it decides. AvatarView and AvatarUpdate are decided by the space and
// by ownership, and are not offered.
func Actions() []security.Action {
	return []security.Action{AvatarManageOrganization}
}

// TeamActions are the platform team's actions. They are declared apart from
// Actions so a catalogue an organization grants from never holds them: an
// application gives them to the groups of its own team only, and TeamPolicy
// refuses them to any subject outside Config.TeamTenant besides.
func TeamActions() []security.Action {
	return []security.Action{AvatarViewAll, AvatarManageAll}
}

// reads says an action only looks. Everything else is told to write, so an
// action added without a decision here fails the first time it tries to write
// under a name that says it only looks, and not the other way round.
func reads(a security.Action) bool {
	return a == AvatarView || a == AvatarViewAll
}

// AvatarPolicy is the only authority over what a member of a space does with an
// Avatar.
//
// It denies unless a rule below says otherwise. The tenant of the record is
// checked first, before any rule, so a rule that opens an action opens it inside
// one space only. A guest has no avatar. Everyone else of the space reads the
// space's avatars; a person changes their own; and the logo of an organization
// is changed by a subject who was granted AvatarManageOrganization.
type AvatarPolicy struct{}

// Compile-time proof that the policy answers about this entity and no other. A
// policy that drifted onto another type would leave this one unguarded while the
// Model path still compiled.
var _ security.Policy[Avatar] = AvatarPolicy{}

// Compile-time proof that the policy says which of its actions write: without it
// the Grant issued to read an avatar would also be one that writes.
var _ auth.Writes = AvatarPolicy{}

// Writes reports whether a Grant issued for the action may change anything.
func (AvatarPolicy) Writes(a security.Action) bool { return !reads(a) }

// Can decides whether the subject may perform the action on the record.
//
// The record is the candidate -- an empty ID, carrying only whose avatar it is
// -- where the question is "may this subject do this to this owner's avatar",
// and the loaded row where it is "may they do it to this one". Both say whose it
// is, which is what the rules about ownership read.
func (AvatarPolicy) Can(_ context.Context, s security.Subject, a security.Action, record Avatar) error {
	// Tenant isolation comes first and applies to every action. Without it every
	// check below would be pointless in a multi-tenant system: a rule that
	// allows an owner to change their own picture would allow it across
	// customers as soon as two of them have a record with the same identifier.
	//
	// The empty id is the candidate that has not been stored yet, which belongs
	// to nobody until it is written with the tenant off the Grant.
	if record.ID != "" && record.TenantID != s.Tenant {
		return fmt.Errorf("avatar belongs to another tenant")
	}

	// arandu:begin custom
	if s.IsGuest() {
		return fmt.Errorf("a guest has no avatar")
	}
	switch a {
	case AvatarView:
		return nil
	case AvatarUpdate:
		if Kind(record.OwnerKind) == KindUser && record.OwnerID != "" && record.OwnerID == s.ID {
			return nil
		}
		return fmt.Errorf("a person changes their own avatar and nobody else's")
	case AvatarManageOrganization:
		if Kind(record.OwnerKind) != KindOrganization {
			return fmt.Errorf("%s is the logo of an organization", a)
		}
		if s.Can(a) {
			return nil
		}
		return fmt.Errorf("the subject was not granted %s", a)
	}
	// arandu:end custom

	return fmt.Errorf("no rule allows %s on avatar", a)
}

// TeamPolicy is the only authority over what the platform team does with an
// Avatar of any space.
//
// It denies unless the subject is a member of the team -- a subject of Realm,
// the tenant the team signs in to -- who was granted the action. A subject of
// any other tenant is refused whatever actions it carries, and so is a subject
// somebody else is viewing as: an operator inside a customer's account acts as
// the customer and reads no other space.
//
// It is the one policy that does not compare a record's tenant with the
// subject's, because crossing tenants is what the team does. The tenant of the
// work itself is not the subject's: the service reads the row to find it, and
// then acts in that tenant under a Grant scoped to it, issued only after this
// policy allowed the action.
type TeamPolicy struct {
	// Realm is the tenant the team signs in to. Empty refuses everybody.
	Realm string
}

var _ security.Policy[Avatar] = TeamPolicy{}

var _ auth.Writes = TeamPolicy{}

// Writes reports whether a Grant issued for the action may change anything.
func (TeamPolicy) Writes(a security.Action) bool { return !reads(a) }

// Can decides whether the subject may perform the action.
func (p TeamPolicy) Can(_ context.Context, s security.Subject, a security.Action, _ Avatar) error {
	if p.Realm == "" || s.Tenant != p.Realm {
		return fmt.Errorf("only the platform team acts on every space's avatars")
	}

	// arandu:begin custom
	if s.IsGuest() {
		return fmt.Errorf("a guest is no member of the team")
	}
	if s.Impersonator != nil {
		return fmt.Errorf("a subject inside somebody else's account reads no other space")
	}
	switch a {
	case AvatarViewAll, AvatarManageAll:
		if s.Can(a) {
			return nil
		}
		return fmt.Errorf("the subject was not granted %s", a)
	}
	// arandu:end custom

	return fmt.Errorf("no rule allows %s on avatar", a)
}
