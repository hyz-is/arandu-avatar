// Package platform is the avatars table read without the tenant filter: the one
// read of arandu-avatar that crosses tenants, the lookup the platform team makes
// to find which space an avatar is in, after the team's policy allowed it.
//
// It is internal so no application can import it. Its table is Global, which is
// how Hesape spells a table with no tenant filter, and any Grant at all would
// read every tenant through it; the package that may reach it is the one whose
// callers have asked the team's policy first. Its rows never leave them: they
// are copied into snapshots that hold no connection, so nothing read across
// tenants can be saved or deleted through it, and every write goes through the
// tenant's own model under a Grant scoped to that tenant.
package platform

import (
	"github.com/arandu-io/hesape/database/model"
)

// Avatar is a row of the avatars table, of any tenant. It declares what a
// lookup needs and nothing else.
type Avatar struct {
	model.Model

	ID              string `db:"id"`
	TenantID        string `db:"tenant_id"`
	OwnerKind       string `db:"owner_kind"`
	OwnerID         string `db:"owner_id"`
	Style           string `db:"style"`
	UploadVersion   string `db:"upload_version"`
	UploadType      string `db:"upload_type"`
	UploadSizes     string `db:"upload_sizes"`
	GravatarEnabled int64  `db:"gravatar_enabled"`
	GravatarHash    string `db:"gravatar_hash"`
	GravatarVersion string `db:"gravatar_version"`
	GravatarType    string `db:"gravatar_type"`
	GravatarSizes   string `db:"gravatar_sizes"`
}

// avatarTable is the avatars table, Global: no tenant filter on any statement.
// The name is written here rather than imported, because the package that owns
// the constant imports this one.
var avatarTable = model.NewTable(model.TableSpec{
	Name:         "avatars",
	New:          func() model.Entity { return new(Avatar) },
	ManualKey:    true,
	NoTimestamps: true,
	Global:       true,
})

// Columns are the columns Avatar reads.
var Columns = []any{
	"id", "tenant_id", "owner_kind", "owner_id", "style", "upload_version", "upload_type", "upload_sizes",
	"gravatar_enabled", "gravatar_hash", "gravatar_version", "gravatar_type", "gravatar_sizes",
}
