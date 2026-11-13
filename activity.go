package avatar

import (
	"context"
	"strings"

	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/translation"
	activitylog "github.com/hyz-is/arandu-activitylog"
)

// ActivityLogName is the log every entry of this package is recorded in.
const ActivityLogName = "avatar"

// ActivityRefType is the kind of subject an entry about an avatar names.
const ActivityRefType = "avatar"

// The events this package records, one per act that changes something. The
// subject of each entry is the avatar row, and its properties say whose it is.
//
// No entry carries a picture, an e-mail or the digest of one: that an act
// happened is what the log keeps.
const (
	ActivityUploaded         = "avatar.uploaded"
	ActivityRemoved          = "avatar.removed"
	ActivityStyleChanged     = "avatar.style_changed"
	ActivityGravatarEnabled  = "avatar.gravatar_enabled"
	ActivityGravatarRefresh  = "avatar.gravatar_refreshed"
	ActivityGravatarDisabled = "avatar.gravatar_disabled"
)

// record writes one entry of the act just done, under the Grant it was
// authorized with: the tenant of the entry is the act's, data.Tenant(g), and the
// causer is the subject who acted, named explicitly because a team member's act
// is written under a Grant scoped to the customer's tenant, whose own subject is
// nobody. Without Config.Activity it records nothing.
//
// The act already happened when this runs, so a failure to record it is returned
// to the caller rather than swallowed: an act the log does not show is the one
// an audit asks about.
func (c *core) record(ctx context.Context, g security.Grant, actor security.Subject, event string, row *Avatar, properties map[string]any) error {
	if c.cfg.Activity == nil {
		return nil
	}
	description := c.labels.line("activity."+strings.ReplaceAll(event, ".", "_"), translation.Replace{"owner": row.OwnerID})
	entry := c.cfg.Activity.In(ctx, g, ActivityLogName).Event(event).
		OnRef(activitylog.Ref{Type: ActivityRefType, ID: row.ID})
	if actor.ID != "" {
		entry = entry.ByRef(activitylog.Ref{Type: "user", ID: actor.ID})
	}
	props := map[string]any{"owner_kind": row.OwnerKind, "owner_id": row.OwnerID}
	for key, value := range properties {
		props[key] = value
	}
	_, err := entry.WithProperties(props).Log(description)
	return err
}
