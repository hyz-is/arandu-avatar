---
name: avatar-policy
description: Decide who may do what in Arandu Avatar, and open or add an action. Use when the request is to "let admins change the logo", "add an action", "add a permission", "everything returns 403", "the tests say ErrForbidden", "add a service method", "why does Open authorize twice", "let the team do X", or when a change touches policy.go, grants.go, service.go or team.go. Covers the Grant, authorization before the Model, the two policies, the custom block that survives regeneration, why the tenant is never read from the request, and the one Grant issued without a member.
license: MIT
---

# Deciding who may do what

## How a request gets through

1. `service.go` validates the input where the validation does not depend on a
   Grant. A rejected input never reaches a policy.
2. `security.Authorize(ctx, policy, actor, ACTION, record)` -- through `reach` for a
   use case on one owner. The first call sees the candidate (an empty `ID`, carrying
   only whose avatar it is); after the row is read the policy is asked again about
   the row, which is what a rule about ownership answers.
3. `policy.go` `Can` returns nil, or the reason it will not.
4. Only then does a Grant exist, and only then the generated query is reached, with
   the Grant, so the tenant is `data.Tenant(g)`.

`TestEveryServiceMethodAuthorizesBeforeTheModel` reads every exported method of
`AvatarService` and `TeamService` and fails on one that reaches the Model first or
never asks. `reach` and `Resolve` are the deciders it trusts, and
`TestTheHelpersThatDecideActuallyDecide` holds that they call `Authorize`.

## The two policies

- **`AvatarPolicy`**, for a member of a space. The tenant check is first and applies
  to every action. `AvatarView` is for anybody non-guest of the tenant.
  `AvatarUpdate` is the person on their own avatar. `AvatarManageOrganization` needs
  the granted action and is for organizations only.
- **`TeamPolicy`**, for the platform team: a subject of `Realm`, holding the action,
  not a guest, not impersonating. It is the one policy that does not compare a
  record's tenant, because crossing tenants is its job.

Rules live inside `// arandu:begin custom` ... `// arandu:end custom`. What is not
written there stays closed, including every action added later.

## Adding an action

Declare it as a constant, put it in `Actions()` or `TeamActions()` (never both: the
team's is never offered to an organization's groups), decide in `reads()` whether it
only looks, write its rule in the custom block, add it to `everyAction` and to the
matrix in `tests/Unit/policy_test.go`, and name it in the changelog in backticks
(`TestEveryActionIsNamedInAReleasedChangelogEntry`).

## The team reads across tenants in one place

`team.go` reaches `internal/platform`, whose table is `Global`, after `TeamPolicy`
answered, to find the tenant of a row; the work itself is then done under
`scopedGrant`, the only `security.SystemGrant` of the package, which checks the realm
Grant was issued for the same action and that the tenant can be one.
`TestAGrantWithoutAMemberIsIssuedInGrantsGoAlone` holds it to `grants.go` and to a
comment saying what decided first. `Space` in a team request narrows a look-up; it is
never a Grant.

## Three shapes that look like authorization and are not

- **A tenant taken from the request.** `data.Tenant(g)` is the only source;
  `TestNoTenantIsReadOutOfTheRequest` fails on the others.
- **A check in the handler.** Handlers read the input and who is asking and call a
  service.
- **A refusal that explains itself.** The router answers the status and its
  standard sentence; the reason is in the log.
