package avatar

import (
	"crypto/sha256"
	"encoding/hex"
	"html/template"
	"net/url"
	"strconv"
	"strings"

	"github.com/hyz-is/arandu-avatar/internal/art"
)

// Picture is how one owner is drawn: where the picture comes from, the
// addresses of the photo when there is one, and the drawing that stands in when
// there is not.
//
// It is a snapshot, holds no connection and carries nothing about the tenant.
// The zero Picture is valid and draws an unnamed initials tile.
type Picture struct {
	// Owner is who it is for.
	Owner Owner
	// Source is where the picture comes from: the photo, the Gravatar copy or
	// the drawing.
	Source Source
	// Style is the generated style, which is also what draws the picture while a
	// photo loads and when it cannot.
	Style Style
	// Version is the version in the address of the photo, empty for a drawing.
	Version string
	// Type is the content type of the photo.
	Type string
	// Sizes are the sides the photo is kept at, ascending.
	Sizes []int

	base    string
	query   string
	palette []string
}

// HasImage reports whether the picture is a photo. A drawing is not an image
// anybody fetches: SVG writes it into the page.
func (p Picture) HasImage() bool {
	return p.Source != SourceGenerated && p.Version != "" && len(p.Sizes) > 0
}

// URL is the address of the photo at the stored size nearest to size, never
// below it when a larger one exists. It is empty for a drawing.
func (p Picture) URL(size int) string {
	if !p.HasImage() {
		return ""
	}
	chosen := p.Sizes[len(p.Sizes)-1]
	for _, s := range p.Sizes {
		if s >= size {
			chosen = s
			break
		}
	}
	return p.base + "/" + p.Version + "/" + strconv.Itoa(chosen) + p.query
}

// Srcset is the photo at every size, for an <img srcset>. It is empty for a
// drawing.
func (p Picture) Srcset() string {
	if !p.HasImage() {
		return ""
	}
	parts := make([]string, len(p.Sizes))
	for i, s := range p.Sizes {
		parts[i] = p.base + "/" + p.Version + "/" + strconv.Itoa(s) + p.query + " " + strconv.Itoa(s) + "w"
	}
	return strings.Join(parts, ", ")
}

// Largest is the side of the biggest stored photo, 0 for a drawing.
func (p Picture) Largest() int {
	if !p.HasImage() {
		return 0
	}
	return p.Sizes[len(p.Sizes)-1]
}

// SVG is the drawing, as an <svg> element for the page: it needs no request,
// shifts no layout and follows the size of its container.
func (p Picture) SVG() template.HTML {
	return template.HTML(art.Render(p.style(), p.input()))
}

// DataURI is the drawing as an address an <img>, an e-mail or a stylesheet can
// hold. A page whose Content-Security-Policy has no data: in img-src cannot show
// it; SVG does not need one.
func (p Picture) DataURI() string { return art.DataURI(p.style(), p.input()) }

// Initials are the letters of the owner's name: the first and the last word of a
// person's, and the first two words of an organization's.
func (p Picture) Initials() string {
	if p.Owner.Kind == KindOrganization {
		return art.OrganizationInitialsOf(p.Owner.Name)
	}
	return art.InitialsOf(p.Owner.Name)
}

// Alt is the name the picture is described with.
func (p Picture) Alt() string { return p.Owner.Name }

func (p Picture) style() Style {
	if art.Valid(p.Style) {
		return p.Style
	}
	return DefaultStyle
}

func (p Picture) input() art.Input {
	return art.Input{Seed: p.Owner.Ref.String(), Name: p.Owner.Name, Organization: p.Owner.Kind == KindOrganization, Palette: p.palette}
}

// Pictures are the pictures of several owners, by owner.
type Pictures map[Ref]Picture

// Of is the picture of one owner. An owner that was not asked for, or has no
// row, gets the zero Picture, which draws the initials of nobody: ask for every
// owner a page draws.
func (ps Pictures) Of(ref Ref) Picture { return ps[ref] }

// buildPicture makes the picture of an owner from the row that holds their
// choices, or from none.
func buildPicture(cfg Config, o Owner, row *Avatar, team bool, space string) Picture {
	base := cfg.Prefix
	if team {
		base = cfg.TeamPrefix
	}
	base += cfg.Paths.segment(o.Kind) + "/" + url.PathEscape(o.ID)
	p := Picture{Owner: o, Source: SourceGenerated, Style: cfg.DefaultStyle, base: base, palette: cfg.Palette}
	if team && space != "" {
		p.query = "?space=" + url.QueryEscape(space)
	}
	if row == nil {
		return p
	}
	if s := Style(row.Style); art.Valid(s) && styleOffered(cfg, s) {
		p.Style = s
	}
	for _, source := range cfg.Chain {
		switch source {
		case SourceUpload:
			if row.HasUpload() && cfg.Files != nil {
				p.Source, p.Version, p.Type, p.Sizes = SourceUpload, row.UploadVersion, row.UploadType, parseSizes(row.UploadSizes)
				return p
			}
		case SourceGravatar:
			if row.HasGravatar() && cfg.Files != nil && (o.Email == "" || gravatarHash(o.Email) == row.GravatarHash) {
				p.Source, p.Version, p.Type, p.Sizes = SourceGravatar, row.GravatarVersion, row.GravatarType, parseSizes(row.GravatarSizes)
				return p
			}
		}
	}
	return p
}

func styleOffered(cfg Config, s Style) bool {
	for _, offered := range cfg.Styles {
		if offered == s {
			return true
		}
	}
	return false
}

// gravatarHash is the SHA-256 of the e-mail, trimmed and lower-cased, which is
// what Gravatar's current API takes in place of the e-mail.
func gravatarHash(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(sum[:])
}
