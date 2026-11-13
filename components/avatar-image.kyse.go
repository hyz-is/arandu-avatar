//go:build kyse

package components

import (
	"strconv"

	kyse "github.com/arandu-io/kyse/components"

	avatar "github.com/hyz-is/arandu-avatar"
)

@go
// AvatarImageBehavior is the name the client behaviour is registered under with
// arandu.ui.define.
const AvatarImageBehavior = "avatar-image"

// AvatarImageProps is who somebody is, in a circle: their photo when they have
// one, and a drawing made from their name when they do not.
//
// It is Basecoat's avatar -- the same class, the same ring, the same sizes --
// with two additions. The drawing is always written into the circle, as an
// inline SVG, and the photo is laid over it: a photo that is slow, or fails, shows
// the drawing, and a photo that loads covers it, so there is no flash and no
// layout shift, and the fallback chain needs no script to work. The photo is
// served at every stored size through srcset, with its width and height from the
// size of the circle, loaded lazily unless Eager says it is above the fold.
//
// It needs avatar.Stylesheet() in the layout's head, once, or WithAssets on the
// call that is the only avatar of the page.
//
// It publishes root, image and name.
type AvatarImageProps struct {
	// ComponentProps is the class, attributes and parts the caller adds.
	kyse.ComponentProps
	// Picture is how the owner is drawn: Module.Resolve answers it.
	Picture avatar.Picture
	// Size is "xs", "sm", "md", "lg", "xl" or "2xl". Empty is "md".
	Size string
	// Alt replaces the owner's name as the text a screen reader is given. A
	// picture beside the name it shows is decorative, and writes "-" to say so:
	// the name is then not read twice.
	Alt string
	// Eager loads the photo at once, for the avatar at the top of the page. The
	// rest are lazy.
	Eager bool
	// WithAssets writes the stylesheet and the script beside this avatar, for a
	// page whose layout does not carry them. A list writes them once in the head
	// instead.
	WithAssets bool
}

// SizeName is the size, defaulting to md.
func (p AvatarImageProps) SizeName() string {
	switch p.Size {
	case "xs", "sm", "lg", "xl", "2xl":
		return p.Size
	}
	return "md"
}

// Pixels is the side of the circle, in CSS pixels, for the sizes attribute.
func (p AvatarImageProps) Pixels() int {
	switch p.SizeName() {
	case "xs":
		return 20
	case "sm":
		return 24
	case "lg":
		return 40
	case "xl":
		return 64
	case "2xl":
		return 96
	}
	return 32
}

// SizesAttr is the sizes attribute: the circle is a fixed number of pixels wide.
func (p AvatarImageProps) SizesAttr() string { return strconv.Itoa(p.Pixels()) + "px" }

// Decorative says the picture sits beside the name it shows.
func (p AvatarImageProps) Decorative() bool { return p.Alt == "-" }

// Name is the text a screen reader is given in place of the picture.
func (p AvatarImageProps) Name() string {
	if p.Alt != "" && p.Alt != "-" {
		return p.Alt
	}
	return p.Picture.Alt()
}

// Loading is the loading attribute of the photo.
func (p AvatarImageProps) Loading() string {
	if p.Eager {
		return "eager"
	}
	return "lazy"
}

// RootAttrs are the outermost element's attributes, with the client bridge
// filled in when the caller named no behaviour of their own.
func (p AvatarImageProps) RootAttrs() map[string]string {
	if p.Behavior.Name == "" {
		p.Behavior = kyse.Behavior{Name: AvatarImageBehavior}
	}
	source := string(p.Picture.Source)
	if source == "" {
		source = string(avatar.SourceGenerated)
	}
	attrs := map[string]string{"data-avatar": "", "data-source": source}
	for name, value := range p.Attrs {
		attrs[name] = value
	}
	p.Attrs = attrs
	return p.ComponentProps.RootAttrs()
}

// PartNames are the parts this component publishes.
func (p AvatarImageProps) PartNames() []string { return []string{"root", "image", "name"} }
@endgo

@if(.WithAssets)
	{!! avatar.Assets() !!}
@endif
<span
	data-part="root"
	class="{{ .RootClass("avatar") }}"
	data-size="{{ .SizeName() }}"
	@attributes(.RootAttrs())
>
	{{-- The drawing is a direct child of the circle, under the photo: the
	     stylesheet lays both over each other. --}}
	{!! .Picture.SVG() !!}
	@if(.Picture.HasImage())
		<img
			data-part="image"
			@if(.PartClass("image") != "")
				class="{{ .PartClass("image") }}"
			@endif
			src="{{ .Picture.URL(.Pixels() * 2) }}"
			srcset="{{ .Picture.Srcset() }}"
			sizes="{{ .SizesAttr() }}"
			width="{{ .Pixels() }}"
			height="{{ .Pixels() }}"
			alt=""
			loading="{{ .Loading() }}"
			decoding="async"
			@attributes(.PartAttrs("image"))
		>
	@endif
	@if(!.Decorative())
		<span
			data-part="name"
			class="sr-only"
		>{{ .Name() }}</span>
	@endif
</span>
