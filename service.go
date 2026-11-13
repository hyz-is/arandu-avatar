package avatar

import (
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/filesystem"

	"github.com/hyz-is/arandu-avatar/internal/art"
	"github.com/hyz-is/arandu-avatar/internal/imaging"
)

// Crop is the square a person chose, in fractions of the photo as it is shown
// upright: X and Y are the top-left corner as a share of the width and of the
// height, and Size the side as a share of the shorter side. The zero Crop is the
// largest centred square, and whatever is written is clamped into the photo.
type Crop = imaging.Crop

// core is what both sides of the package share: the database handle, the
// configuration and the use cases that run after a policy decided.
type core struct {
	db     *data.DB
	cfg    Config
	labels Labels
	http   *http.Client
}

func newCore(db *data.DB, cfg Config) *core {
	c := &core{db: db, cfg: cfg, labels: labelsOf(cfg), http: cfg.HTTPClient}
	if c.http == nil {
		c.http = newGravatarClient(cfg.GravatarURL)
	}
	return c
}

func (c *core) now() time.Time { return c.cfg.Now().UTC() }

// AvatarService orchestrates what the members of a space do with avatars: it
// validates the request, asks the policy for a Grant, applies the rules the
// entity holds for itself, and persists with that Grant.
//
// It receives its collaborators through the constructor. There is no container
// and no resolution by reflection: what this service is made of is written at
// the one place that builds it, and reading that place is how somebody learns
// what the package touches.
type AvatarService struct {
	*core
	policy AvatarPolicy
}

// NewAvatarService wires the service over the application's database handle.
func NewAvatarService(db *data.DB, cfg Config) *AvatarService {
	cfg = cfg.withDefaults()
	return &AvatarService{core: newCore(db, cfg), policy: AvatarPolicy{}}
}

// updateAction is the action that changes the avatar of this kind of owner.
func updateAction(k Kind) security.Action {
	if k == KindOrganization {
		return AvatarManageOrganization
	}
	return AvatarUpdate
}

// candidate is the owner's avatar as the policy sees it before any row is read:
// no identifier, and whose it is.
func candidate(ref Ref) Avatar { return Avatar{OwnerKind: string(ref.Kind), OwnerID: ref.ID} }

// row finds the row of an owner in the tenant of the Grant, and nil when there
// is none.
func (c *core) row(ctx context.Context, g security.Grant, ref Ref) (*Avatar, error) {
	return Avatars(c.db).Where("owner_kind", "=", string(ref.Kind)).Where("owner_id", "=", ref.ID).First(ctx, g)
}

// newRow is a row for an owner that has none, written under the Grant's tenant.
func (c *core) newRow(g security.Grant, ref Ref) (*Avatar, error) {
	id, err := data.NewID()
	if err != nil {
		return nil, err
	}
	row, err := Avatars(c.db).New()
	if err != nil {
		return nil, err
	}
	row.ID = id
	row.TenantID = data.Tenant(g)
	row.OwnerKind, row.OwnerID = string(ref.Kind), ref.ID
	row.CreatedAt = c.now()
	row.UpdatedAt = row.CreatedAt
	return row, nil
}

// owner is who the avatar is for, as the application's Directory names them. With
// no Directory the only owner is the signed-in person, with no name and no
// e-mail.
func (c *core) owner(ctx context.Context, g security.Grant, actor security.Subject, ref Ref) (Owner, error) {
	if c.cfg.Directory == nil {
		if ref.Kind == KindUser && ref.ID == actor.ID {
			return Owner{Ref: ref}, nil
		}
		return Owner{}, ErrNotFound
	}
	o, err := c.cfg.Directory.Owner(ctx, g, ref)
	if errors.Is(err, ErrUnknownOwner) {
		return Owner{}, ErrNotFound
	}
	if err != nil {
		return Owner{}, err
	}
	o.Ref = ref
	return o, nil
}

// reach is what every use case on one owner starts with: the policy is asked
// about the kind of act with nothing in hand, the owner is named by the
// Directory in the tenant of the Grant that answer gave, and the row, if any, is
// read and the policy is asked again about it. A reference that cannot name an
// owner is a 404, as is an owner of another space.
func (s *AvatarService) reach(ctx context.Context, actor security.Subject, a security.Action, ref Ref) (security.Grant, *Avatar, Owner, error) {
	if !ref.Valid() {
		return security.Grant{}, nil, Owner{}, ErrNotFound
	}
	g, err := security.Authorize(ctx, s.policy, actor, a, candidate(ref))
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
	if row != nil {
		if _, err := security.Authorize(ctx, s.policy, actor, a, *row); err != nil {
			return security.Grant{}, nil, Owner{}, err
		}
	}
	return g, row, owner, nil
}

// Resolve is how the owners are drawn, in one query however many there are. An
// owner nobody has set up is drawn from their name, with the default style, so
// the answer holds an entry for every owner asked about. An owner of another
// space is drawn the same way: nothing here tells it exists.
func (s *AvatarService) Resolve(ctx context.Context, actor security.Subject, owners ...Owner) (Pictures, error) {
	g, err := security.Authorize(ctx, s.policy, actor, AvatarView, Avatar{})
	if err != nil {
		return nil, err
	}
	rows, err := s.rows(ctx, g, owners)
	if err != nil {
		return nil, err
	}
	out := make(Pictures, len(owners))
	for _, o := range owners {
		out[o.Ref] = buildPicture(s.cfg, o, rows[o.Ref], false, "")
	}
	return out, nil
}

// rows reads the rows of several owners in the tenant of the Grant, one
// statement per kind.
func (c *core) rows(ctx context.Context, g security.Grant, owners []Owner) (map[Ref]*Avatar, error) {
	byKind := map[Kind][]any{}
	for _, o := range owners {
		if o.Ref.Valid() {
			byKind[o.Kind] = append(byKind[o.Kind], o.ID)
		}
	}
	out := map[Ref]*Avatar{}
	for kind, ids := range byKind {
		found, err := Avatars(c.db).Where("owner_kind", "=", string(kind)).WhereIn("owner_id", ids).Get(ctx, g)
		if err != nil {
			return nil, err
		}
		for _, row := range found {
			out[row.Ref()] = row
		}
	}
	return out, nil
}

// Picture is how one owner is drawn. See Resolve.
func (s *AvatarService) Picture(ctx context.Context, actor security.Subject, owner Owner) (Picture, error) {
	pictures, err := s.Resolve(ctx, actor, owner)
	if err != nil {
		return Picture{}, err
	}
	return pictures.Of(owner.Ref), nil
}

// Opened is a picture ready to be written to a response.
type Opened struct {
	// Body is the file. The caller closes it. Zero when NotModified or
	// Redirect.
	Body filesystem.File
	// ContentType is what the file is, as the package detected and stored it.
	ContentType string
	// ETag is the entity tag of the picture, quoted.
	ETag string
	// NotModified says the caller's copy is the current one: answer 304.
	NotModified bool
	// Redirect is where the owner's current picture is, when the address asked
	// for is one that was replaced: answer 302 there.
	Redirect string
}

// Open reads the picture an address names: the owner, the version and the size.
//
// The version is part of the address so that a cache may keep the answer for as
// long as it likes. An address whose version is not the owner's current one is
// not served: it is redirected to the current picture, which a page rendered
// before the change still links to. A size that is not one of the stored sizes
// is a 404.
func (s *AvatarService) Open(ctx context.Context, actor security.Subject, ref Ref, version string, size int, ifNoneMatch string) (Opened, error) {
	if !ref.Valid() {
		return Opened{}, ErrNotFound
	}
	g, err := security.Authorize(ctx, s.policy, actor, AvatarView, candidate(ref))
	if err != nil {
		return Opened{}, err
	}
	row, err := s.row(ctx, g, ref)
	if err != nil {
		return Opened{}, err
	}
	if row == nil {
		return Opened{}, ErrNotFound
	}
	if _, err := security.Authorize(ctx, s.policy, actor, AvatarView, *row); err != nil {
		return Opened{}, err
	}
	return s.core.open(ctx, g, row, version, size, ifNoneMatch, false, "")
}

// open is the part of Open the two sides share, after a policy decided.
func (c *core) open(ctx context.Context, g security.Grant, row *Avatar, version string, size int, ifNoneMatch string, team bool, space string) (Opened, error) {
	var which slot
	var contentType string
	var sizes []int
	switch {
	case row.HasUpload() && row.UploadVersion == version:
		which, contentType, sizes = slotUpload, row.UploadType, parseSizes(row.UploadSizes)
	case row.HasGravatar() && row.GravatarVersion == version:
		which, contentType, sizes = slotGravatar, row.GravatarType, parseSizes(row.GravatarSizes)
	default:
		current := buildPicture(c.cfg, Owner{Ref: row.Ref()}, row, team, space)
		if !current.HasImage() {
			return Opened{}, ErrNotFound
		}
		return Opened{Redirect: current.URL(size)}, nil
	}
	if !slices.Contains(sizes, size) || c.cfg.Files == nil {
		return Opened{}, ErrNotFound
	}
	etag := `"` + version + "-" + strconv.Itoa(size) + `"`
	if matches(ifNoneMatch, etag) {
		return Opened{ETag: etag, ContentType: contentType, NotModified: true}, nil
	}
	body, err := c.cfg.Files.Get(ctx, g, fileKey(row.ID, which, version, size, contentType))
	if err != nil {
		return Opened{}, ErrNotFound
	}
	return Opened{Body: body, ContentType: contentType, ETag: etag}, nil
}

// matches reports whether an If-None-Match header names the entity tag, weakly,
// as the header is read for a GET.
func matches(header, etag string) bool {
	if header == "" {
		return false
	}
	if strings.TrimSpace(header) == "*" {
		return true
	}
	for _, part := range strings.Split(header, ",") {
		if strings.TrimPrefix(strings.TrimSpace(part), "W/") == etag {
			return true
		}
	}
	return false
}

// UploadRequest is the input contract of a photo.
//
// The fields are explicit and there is no mass assignment. There is no tenant
// here and there must never be one: the tenant comes from the Grant, which comes
// from the session.
type UploadRequest struct {
	// Body is the file as it was sent. It is read up to Config.MaxUploadBytes.
	Body io.Reader
	// Crop is the square the person chose. The zero Crop is the largest centred
	// square.
	Crop Crop
}

// Upload keeps a photo as the owner's picture: checked by what its bytes are,
// cut to the square, scaled to every size, encoded again without a byte of
// metadata, and written under keys the server generates. The photo it replaces
// is removed once the row names the new one.
func (s *AvatarService) Upload(ctx context.Context, actor security.Subject, ref Ref, in UploadRequest) (*Avatar, error) {
	g, row, owner, err := s.reach(ctx, actor, updateAction(ref.Kind), ref)
	if err != nil {
		return nil, err
	}
	return s.core.upload(ctx, g, actor, owner, row, in)
}

// Remove removes the owner's photo, and the files it was kept in. An owner with
// no photo is left as they are.
func (s *AvatarService) Remove(ctx context.Context, actor security.Subject, ref Ref) error {
	g, row, _, err := s.reach(ctx, actor, updateAction(ref.Kind), ref)
	if err != nil {
		return err
	}
	return s.core.removePhoto(ctx, g, actor, row)
}

// SetStyle chooses the generated style an owner is drawn with when no photo
// stands in front of it.
func (s *AvatarService) SetStyle(ctx context.Context, actor security.Subject, ref Ref, style Style) (*Avatar, error) {
	g, row, _, err := s.reach(ctx, actor, updateAction(ref.Kind), ref)
	if err != nil {
		return nil, err
	}
	return s.core.setStyle(ctx, g, actor, ref, row, style)
}

// GravatarAction is what a person asks of their Gravatar.
type GravatarAction string

// The things a person asks of their Gravatar.
const (
	// GravatarEnable switches it on: the e-mail's digest goes to Gravatar once
	// and the picture that comes back is kept.
	GravatarEnable GravatarAction = "enable"
	// GravatarRefresh fetches the picture again.
	GravatarRefresh GravatarAction = "refresh"
	// GravatarDisable switches it off and forgets the copy.
	GravatarDisable GravatarAction = "disable"
)

// Valid reports whether the action is one of the three.
func (a GravatarAction) Valid() bool {
	return a == GravatarEnable || a == GravatarRefresh || a == GravatarDisable
}

// SetGravatar switches the person's Gravatar on, refreshes it or switches it off.
// It is the person's act alone: only a person has one, and only they may switch
// it on, because the digest of their e-mail leaves for a third party.
func (s *AvatarService) SetGravatar(ctx context.Context, actor security.Subject, ref Ref, action GravatarAction) (*Avatar, error) {
	g, row, owner, err := s.reach(ctx, actor, updateAction(ref.Kind), ref)
	if err != nil {
		return nil, err
	}
	return s.core.setGravatar(ctx, g, actor, owner, row, action)
}

// upload is Upload after a policy decided.
func (c *core) upload(ctx context.Context, g security.Grant, actor security.Subject, owner Owner, row *Avatar, in UploadRequest) (*Avatar, error) {
	if !c.cfg.uploadOn() {
		return nil, c.labels.invalid("file", "files_off")
	}
	if in.Body == nil {
		return nil, c.labels.invalid("file", "file_required")
	}
	body, err := io.ReadAll(io.LimitReader(in.Body, c.cfg.MaxUploadBytes+1))
	if err != nil {
		return nil, err
	}
	switch {
	case len(body) == 0:
		return nil, c.labels.invalid("file", "file_empty")
	case int64(len(body)) > c.cfg.MaxUploadBytes:
		return nil, c.labels.invalid("file", "file_large", "mb", megabytes(c.cfg.MaxUploadBytes))
	}
	res, err := c.process(body, in.Crop)
	if err != nil {
		return nil, err
	}
	return c.store(ctx, g, actor, owner.Ref, row, slotUpload, res, ActivityUploaded, nil)
}

// process runs the image pipeline and says what went wrong in the person's words.
func (c *core) process(body []byte, crop Crop) (imaging.Result, error) {
	res, err := imaging.Process(body, crop, c.cfg.Sizes, imaging.Limits{MaxPixels: c.cfg.MaxPixels, JPEGQuality: c.cfg.JPEGQuality})
	switch {
	case err == nil:
		return res, nil
	case errors.Is(err, imaging.ErrType):
		return imaging.Result{}, c.labels.invalid("file", "file_type")
	case errors.Is(err, imaging.ErrTooLarge):
		return imaging.Result{}, c.labels.invalid("file", "file_pixels")
	case errors.Is(err, imaging.ErrTooSmall):
		return imaging.Result{}, c.labels.invalid("file", "file_small", "px", strconv.Itoa(c.cfg.Sizes[0]))
	default:
		return imaging.Result{}, c.labels.invalid("file", "file_corrupt")
	}
}

// store keeps a processed photo in a slot of the owner's row, creating the row
// when the owner has none, and removes the photo the slot held.
func (c *core) store(ctx context.Context, g security.Grant, actor security.Subject, ref Ref, row *Avatar, which slot, res imaging.Result, event string, extra map[string]any) (*Avatar, error) {
	if row == nil {
		var err error
		if row, err = c.newRow(g, ref); err != nil {
			return nil, err
		}
	}
	kept, err := c.keep(ctx, g, row.ID, which, res)
	if err != nil {
		return nil, err
	}
	old := keysOf(row, which)
	if which == slotUpload {
		row.SetUpload(kept.version, kept.contentType, kept.sizes, c.now())
	} else {
		hash, _ := extra["hash"].(string)
		row.SetGravatar(hash, kept.version, kept.contentType, kept.sizes, c.now())
	}
	if _, err := row.Save(ctx, g); err != nil {
		c.drop(ctx, g, kept.keys)
		return nil, err
	}
	// The old files go after the row stopped naming them. When the new photo is
	// the same bytes the keys are the same, and nothing is removed.
	c.drop(ctx, g, withoutKeys(old, kept.keys))
	if err := c.record(ctx, g, actor, event, row, map[string]any{"version": kept.version}); err != nil {
		return nil, err
	}
	return row, nil
}

func withoutKeys(keys, keep []string) []string {
	var out []string
	for _, k := range keys {
		if !slices.Contains(keep, k) {
			out = append(out, k)
		}
	}
	return out
}

// removePhoto is Remove after a policy decided.
func (c *core) removePhoto(ctx context.Context, g security.Grant, actor security.Subject, row *Avatar) error {
	if row == nil || !row.HasUpload() {
		return nil
	}
	old := keysOf(row, slotUpload)
	row.ClearUpload(c.now())
	if _, err := row.Save(ctx, g); err != nil {
		return err
	}
	if c.cfg.Files != nil {
		c.drop(ctx, g, old)
	}
	return c.record(ctx, g, actor, ActivityRemoved, row, nil)
}

// setStyle is SetStyle after a policy decided.
func (c *core) setStyle(ctx context.Context, g security.Grant, actor security.Subject, ref Ref, row *Avatar, style Style) (*Avatar, error) {
	if !art.Valid(style) || !styleOffered(c.cfg, style) {
		return nil, c.labels.invalid("style", "style")
	}
	if row == nil {
		var err error
		if row, err = c.newRow(g, ref); err != nil {
			return nil, err
		}
	}
	if Style(row.Style) == style {
		return row, nil
	}
	row.SetStyle(style, c.now())
	if _, err := row.Save(ctx, g); err != nil {
		return nil, err
	}
	if err := c.record(ctx, g, actor, ActivityStyleChanged, row, map[string]any{"style": string(style)}); err != nil {
		return nil, err
	}
	return row, nil
}

// setGravatar is SetGravatar after a policy decided.
func (c *core) setGravatar(ctx context.Context, g security.Grant, actor security.Subject, owner Owner, row *Avatar, action GravatarAction) (*Avatar, error) {
	if !action.Valid() {
		return nil, c.labels.invalid("gravatar", "gravatar_action")
	}
	if !c.cfg.gravatarOn() {
		return nil, c.labels.invalid("gravatar", "gravatar_off")
	}
	if owner.Kind != KindUser {
		return nil, c.labels.invalid("gravatar", "gravatar_kind")
	}
	if action == GravatarDisable {
		if row == nil || !bool(row.GravatarEnabled) {
			return row, nil
		}
		old := keysOf(row, slotGravatar)
		row.ClearGravatar(c.now())
		if _, err := row.Save(ctx, g); err != nil {
			return nil, err
		}
		c.drop(ctx, g, old)
		if err := c.record(ctx, g, actor, ActivityGravatarDisabled, row, nil); err != nil {
			return nil, err
		}
		return row, nil
	}
	if strings.TrimSpace(owner.Email) == "" {
		return nil, c.labels.invalid("gravatar", "gravatar_email")
	}
	raw, err := c.fetchGravatar(ctx, owner.Email)
	switch {
	case errors.Is(err, errNoGravatar):
		return nil, c.labels.invalid("gravatar", "gravatar_none")
	case err != nil:
		return nil, c.labels.invalid("gravatar", "gravatar_unavailable")
	}
	res, err := c.process(raw, Crop{})
	if err != nil {
		return nil, c.labels.invalid("gravatar", "gravatar_unavailable")
	}
	event := ActivityGravatarEnabled
	if action == GravatarRefresh && row != nil && row.HasGravatar() {
		event = ActivityGravatarRefresh
	}
	return c.store(ctx, g, actor, owner.Ref, row, slotGravatar, res, event, map[string]any{"hash": gravatarHash(owner.Email)})
}

// megabytes is a size as the sentence says it: "5" for five mebibytes.
func megabytes(n int64) string {
	mb := float64(n) / float64(1<<20)
	if mb == float64(int64(mb)) {
		return strconv.FormatInt(int64(mb), 10)
	}
	return strconv.FormatFloat(mb, 'f', 1, 64)
}
