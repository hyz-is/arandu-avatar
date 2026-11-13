package avatar

import (
	"fmt"

	"github.com/arandu-io/framework/security"
)

// This file is the only place in the package that builds a Grant without a
// Policy answering for it, and the audit in tests/Unit holds that. The one below
// is issued after something else decided:
//
//   - scopedGrant, after TeamPolicy allowed the team member, for the one tenant
//     the avatar they act on lives in. The Grant the team member holds is the
//     realm's; an avatar is in a customer's tenant, and a statement needs the
//     Grant of the tenant it reads. The scope is checked against the action the
//     realm Grant was issued for, so a Grant issued to read cannot be scoped
//     into one that writes something else.

// scopedGrant scopes the realm Grant of an allowed team member to the tenant of
// one avatar. The realm Grant must have been issued for the action asked.
func scopedGrant(realm security.Grant, a security.Action, tenant string) (security.Grant, error) {
	if err := realm.Check(a); err != nil {
		return security.Grant{}, err
	}
	if !security.ValidTenant(tenant) {
		return security.Grant{}, fmt.Errorf("avatar: an avatar is in the tenant %q, which cannot be one", tenant)
	}
	//arandu:system-grant TeamPolicy allowed the team member in their realm; each read or write is scoped to the one tenant the avatar is in
	return security.SystemGrant(a, tenant), nil
}
