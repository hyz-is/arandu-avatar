package unit_test

import (
	"context"
	"strings"
	"testing"

	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/auth"

	avatar "github.com/hyz-is/arandu-avatar"
)

var (
	memberActions = []security.Action{avatar.AvatarView, avatar.AvatarUpdate, avatar.AvatarManageOrganization}
	teamActions   = []security.Action{avatar.AvatarViewAll, avatar.AvatarManageAll}
	everyAction   = append(append([]security.Action(nil), memberActions...), teamActions...)
)

func subject(tenant, id string, actions ...security.Action) security.Subject {
	return security.Subject{ID: id, Tenant: tenant, Verified: true, Actions: actions}
}

func owned(tenant, kind, id string) avatar.Avatar {
	return avatar.Avatar{ID: "row", TenantID: tenant, OwnerKind: kind, OwnerID: id}
}

func probe(kind avatar.Kind, id string) avatar.Avatar {
	return avatar.Avatar{OwnerKind: string(kind), OwnerID: id}
}

func TestTheMembersPolicyReadsForEveryoneOfTheSpaceAndWritesForTheOwner(t *testing.T) {
	t.Parallel()

	p := avatar.AvatarPolicy{}
	ctx := context.Background()
	ada := subject("acme", "ada")
	for name, tc := range map[string]struct {
		who    security.Subject
		action security.Action
		record avatar.Avatar
		want   bool
	}{
		"anyone of the space reads a person":       {subject("acme", "bob"), avatar.AvatarView, owned("acme", "user", "ada"), true},
		"anyone of the space reads a logo":         {subject("acme", "bob"), avatar.AvatarView, owned("acme", "organization", "acme"), true},
		"the candidate is readable":                {subject("acme", "bob"), avatar.AvatarView, probe(avatar.KindUser, "ada"), true},
		"another space reads nothing":              {subject("other", "eve"), avatar.AvatarView, owned("acme", "user", "ada"), false},
		"a guest reads nothing":                    {security.Guest("acme"), avatar.AvatarView, probe(avatar.KindUser, "ada"), false},
		"a person changes their own":               {ada, avatar.AvatarUpdate, owned("acme", "user", "ada"), true},
		"a person changes their own, candidate":    {ada, avatar.AvatarUpdate, probe(avatar.KindUser, "ada"), true},
		"nobody changes somebody else's":           {subject("acme", "bob"), avatar.AvatarUpdate, owned("acme", "user", "ada"), false},
		"an administrator changes nobody's":        {security.Subject{ID: "boss", Tenant: "acme", Roles: []string{"admin", "owner"}}, avatar.AvatarUpdate, owned("acme", "user", "ada"), false},
		"the same id in another space is refused":  {ada, avatar.AvatarUpdate, owned("other", "user", "ada"), false},
		"an empty id is nobody":                    {subject("acme", ""), avatar.AvatarUpdate, probe(avatar.KindUser, ""), false},
		"a person does not change a logo":          {ada, avatar.AvatarUpdate, owned("acme", "organization", "ada"), false},
		"the logo needs the action":                {ada, avatar.AvatarManageOrganization, owned("acme", "organization", "acme"), false},
		"the action opens the logo":                {subject("acme", "boss", avatar.AvatarManageOrganization), avatar.AvatarManageOrganization, owned("acme", "organization", "acme"), true},
		"the action does not open a person":        {subject("acme", "boss", avatar.AvatarManageOrganization), avatar.AvatarManageOrganization, owned("acme", "user", "ada"), false},
		"the action is not valid in another space": {subject("acme", "boss", avatar.AvatarManageOrganization), avatar.AvatarManageOrganization, owned("other", "organization", "acme"), false},
		"the team's actions are not the space's":   {subject("acme", "boss", avatar.AvatarManageAll, avatar.AvatarViewAll), avatar.AvatarManageAll, owned("acme", "user", "ada"), false},
		"an action nobody declared":                {ada, "avatar.delete_everything", owned("acme", "user", "ada"), false},
	} {
		err := p.Can(ctx, tc.who, tc.action, tc.record)
		if (err == nil) != tc.want {
			t.Errorf("%s: err = %v, want allowed=%v", name, err, tc.want)
		}
	}
}

func TestARecordOfAnotherTenantIsRefusedWithTheSentenceTheContractHolds(t *testing.T) {
	t.Parallel()

	err := avatar.AvatarPolicy{}.Can(context.Background(), subject("acme", "ada"), avatar.AvatarView, owned("other", "user", "ada"))
	if err == nil || !strings.Contains(err.Error(), "another tenant") {
		t.Errorf("err = %v, want it to say another tenant", err)
	}
}

func TestTheTeamPolicyAllowsTheTeamAndOnlyTheTeam(t *testing.T) {
	t.Parallel()

	p := avatar.TeamPolicy{Realm: "platform"}
	ctx := context.Background()
	impersonating := subject("platform", "root", avatar.AvatarManageAll)
	impersonating.Impersonator = &security.Subject{ID: "x"}
	for name, tc := range map[string]struct {
		policy avatar.TeamPolicy
		who    security.Subject
		action security.Action
		want   bool
	}{
		"the team member with the action":      {p, subject("platform", "root", avatar.AvatarViewAll), avatar.AvatarViewAll, true},
		"managing needs its own action":        {p, subject("platform", "root", avatar.AvatarViewAll), avatar.AvatarManageAll, false},
		"managing allowed":                     {p, subject("platform", "root", avatar.AvatarManageAll), avatar.AvatarManageAll, true},
		"a customer with the team's action":    {p, subject("acme", "ada", avatar.AvatarManageAll, avatar.AvatarViewAll), avatar.AvatarViewAll, false},
		"a team member with no action":         {p, subject("platform", "root"), avatar.AvatarViewAll, false},
		"a guest of the realm":                 {p, security.Guest("platform"), avatar.AvatarViewAll, false},
		"somebody acting as another account":   {p, impersonating, avatar.AvatarManageAll, false},
		"no realm refuses everybody":           {avatar.TeamPolicy{}, subject("", "root", avatar.AvatarViewAll), avatar.AvatarViewAll, false},
		"the space's action is not the team's": {p, subject("platform", "root", avatar.AvatarManageOrganization), avatar.AvatarManageOrganization, false},
	} {
		err := tc.policy.Can(ctx, tc.who, tc.action, avatar.Avatar{})
		if (err == nil) != tc.want {
			t.Errorf("%s: err = %v, want allowed=%v", name, err, tc.want)
		}
	}
}

func TestThePoliciesDenyEveryActionToASubjectWithNothing(t *testing.T) {
	t.Parallel()

	nobody := subject("acme", "x")
	for _, a := range teamActions {
		if err := (avatar.AvatarPolicy{}).Can(context.Background(), nobody, a, avatar.Avatar{}); err == nil {
			t.Errorf("the members' policy allows the team's %s", a)
		}
		if err := (avatar.TeamPolicy{Realm: "acme"}).Can(context.Background(), nobody, a, avatar.Avatar{}); err == nil {
			t.Errorf("the team's policy allows %s to a subject with no action", a)
		}
	}
}

func TestTheGrantsOfReadingOnlyReadAndWhatWritesWrites(t *testing.T) {
	t.Parallel()

	var writes auth.Writes = avatar.AvatarPolicy{}
	var teamWrites auth.Writes = avatar.TeamPolicy{}
	for _, a := range everyAction {
		reads := a == avatar.AvatarView || a == avatar.AvatarViewAll
		if writes.Writes(a) == reads || teamWrites.Writes(a) == reads {
			t.Errorf("%s: Writes disagrees with the action's name (reads=%v)", a, reads)
		}
	}
}

func TestTheTwoSetsOfActionsAreApart(t *testing.T) {
	t.Parallel()

	for _, a := range avatar.Actions() {
		for _, b := range avatar.TeamActions() {
			if a == b {
				t.Errorf("%s is in both sets", a)
			}
		}
	}
	if len(avatar.Actions()) != 1 || len(avatar.TeamActions()) != 2 {
		t.Errorf("sets = %v and %v", avatar.Actions(), avatar.TeamActions())
	}
}
