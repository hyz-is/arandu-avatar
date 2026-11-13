package avatar

import (
	"context"

	"github.com/arandu-io/framework/foundation"
	"github.com/arandu-io/hesape/database/migrations"
	"github.com/arandu-io/hesape/database/schema"
)

// Migrations declares the schema this module owns.
//
// They are returned in the order their names sort in, which is the order they
// apply in: the name carries the order, and nothing else decides it.
func (m *Module) Migrations() []foundation.Migration {
	return []foundation.Migration{createAvatars{}}
}

// The migration is reversible, and the assertion is here rather than discovered
// at rollback: the migrator tests for Down with a type assertion, so a Down with
// the wrong signature would leave a rollback that silently does nothing.
var _ migrations.ReversibleMigration = createAvatars{}

// createAvatars is the table this module owns and the indexes it is read by.
type createAvatars struct{ migrations.BaseMigration }

// GetName is the migration's identity, and it carries the order. It is fixed
// once the package is published: changing what an applied name means leaves the
// change missing everywhere it already ran, and nothing says so.
func (createAvatars) GetName() string { return "20261113_0001_create_avatars" }

// Up creates the table and its indexes.
//
// The timestamps have no database default: the value comes from Go, which is
// also what lets a test hold the clock. One owner has one row in a space, which
// the unique index holds against two requests racing to write the first.
func (createAvatars) Up(ctx context.Context, conn migrations.Connection) error {
	return conn.Schema().Create(ctx, avatarsTable, func(table *schema.Blueprint) {
		table.String("id").Primary()
		table.String("tenant_id")
		table.String("owner_kind", 16)
		table.String("owner_id", maxIdentifierLen)
		table.String("style", maxStyleLen).Default("")
		table.String("upload_version", maxVersionLen).Default("")
		table.String("upload_type", maxTypeLen).Default("")
		table.String("upload_sizes", maxSizesLen).Default("")
		table.Integer("gravatar_enabled").Default(0)
		table.String("gravatar_hash", maxHashLen).Default("")
		table.String("gravatar_version", maxVersionLen).Default("")
		table.String("gravatar_type", maxTypeLen).Default("")
		table.String("gravatar_sizes", maxSizesLen).Default("")
		table.Timestamp("created_at")
		table.Timestamp("updated_at")
		table.Unique([]string{"tenant_id", "owner_kind", "owner_id"}, "avatars_tenant_owner_unique")
		table.Index([]string{"owner_kind", "owner_id"}, "avatars_owner_idx")
	})
}

// Down drops the table.
func (createAvatars) Down(ctx context.Context, conn migrations.Connection) error {
	return conn.Schema().DropIfExists(ctx, avatarsTable)
}
