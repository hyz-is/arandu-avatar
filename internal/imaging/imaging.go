// Package imaging turns the bytes somebody uploaded into the few square
// pictures an avatar is served as.
//
// What arrives is checked by what the bytes are -- JPEG, PNG or WebP, by their
// magic numbers -- and never by the name or the type the sender announced. The
// header is read first and the size it declares is held against a ceiling
// before a single pixel is decoded, so a few dozen bytes cannot ask the
// decoder for gigabytes. The picture is turned to the way its EXIF orientation
// says is up, cut to a square, scaled to each size and encoded again, which is
// what removes every byte of metadata -- EXIF, GPS, thumbnails, comments --
// because nothing but pixels is written back.
//
// It uses the standard library for JPEG and PNG and golang.org/x/image for
// reading WebP and for the scaling, which is pure Go: nothing here links C, so
// the binary of an application stays static.
package imaging

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // registers the WebP decoder with image.Decode
)

// What a rejected upload is rejected for.
var (
	// ErrType is bytes that are not a JPEG, a PNG or a WebP.
	ErrType = errors.New("imaging: the file is not a JPEG, PNG or WebP picture")
	// ErrCorrupt is a file that starts like a picture and cannot be read as one.
	ErrCorrupt = errors.New("imaging: the picture cannot be read")
	// ErrTooLarge is a picture that declares more pixels than the ceiling.
	ErrTooLarge = errors.New("imaging: the picture has too many pixels")
	// ErrTooSmall is a picture whose cut is smaller than the smallest size.
	ErrTooSmall = errors.New("imaging: the picture is too small")
)

// Crop is the square a person chose, in fractions of the picture as it is
// shown upright: X and Y are the top-left corner as a share of the width and
// of the height, and Size is the side as a share of the shorter side. The zero
// Crop is the largest centred square. Whatever is written here is clamped into
// the picture, so no value makes a request fail or reaches outside it.
type Crop struct {
	X, Y, Size float64
}

// Kind is what the bytes are.
type Kind string

// The kinds that are accepted.
const (
	JPEG Kind = "image/jpeg"
	PNG  Kind = "image/png"
	WebP Kind = "image/webp"
)

// Detect says what the bytes are, from their first bytes alone.
func Detect(b []byte) (Kind, bool) {
	switch {
	case len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF:
		return JPEG, true
	case len(b) >= 8 && bytes.Equal(b[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}):
		return PNG, true
	case len(b) >= 12 && bytes.Equal(b[:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")):
		return WebP, true
	}
	return "", false
}

// Variant is the picture at one size.
type Variant struct {
	// Size is the side in pixels.
	Size int
	// Body is the encoded picture.
	Body []byte
}

// Result is what an upload became.
type Result struct {
	// Type is the content type of every variant: image/jpeg, or image/png when
	// the picture has transparency worth keeping.
	Type string
	// Extension is the file extension that goes with Type, with its dot.
	Extension string
	// Variants are the pictures, one per requested size, in the order asked.
	Variants []Variant
}

// Limits are what Process holds a picture to.
type Limits struct {
	// MaxPixels is the most pixels a picture may declare, width times height.
	MaxPixels int
	// JPEGQuality is the quality JPEG variants are encoded at, 1 to 100.
	JPEGQuality int
}

// Process checks, decodes, orients, cuts and scales an upload into one variant
// per size. sizes are side lengths in pixels, ascending.
func Process(src []byte, crop Crop, sizes []int, limits Limits) (Result, error) {
	kind, ok := Detect(src)
	if !ok {
		return Result{}, ErrType
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(src))
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	if cfg.Width < 1 || cfg.Height < 1 {
		return Result{}, ErrCorrupt
	}
	if limits.MaxPixels > 0 && int64(cfg.Width)*int64(cfg.Height) > int64(limits.MaxPixels) {
		return Result{}, ErrTooLarge
	}
	decoded, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}

	upright := image.Image(decoded)
	if kind == JPEG {
		upright = orient(decoded, exifOrientation(src))
	}

	region := cut(upright.Bounds(), crop)
	if region.Dx() < sizes[0] {
		return Result{}, ErrTooSmall
	}

	// The largest size is scaled from the source once; the others from that,
	// which is already small and in a type the scaler has a fast path for.
	largest := sizes[len(sizes)-1]
	base := image.NewNRGBA(image.Rect(0, 0, largest, largest))
	xdraw.CatmullRom.Scale(base, base.Bounds(), upright, region, xdraw.Src, nil)

	transparent := hasTransparency(base)
	out := Result{Type: "image/jpeg", Extension: ".jpg"}
	if transparent {
		out.Type, out.Extension = "image/png", ".png"
	}
	for _, size := range sizes {
		scaled := base
		if size != largest {
			scaled = image.NewNRGBA(image.Rect(0, 0, size, size))
			xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), base, base.Bounds(), xdraw.Src, nil)
		}
		body, err := encode(scaled, transparent, limits.JPEGQuality)
		if err != nil {
			return Result{}, err
		}
		out.Variants = append(out.Variants, Variant{Size: size, Body: body})
	}
	return out, nil
}

// cut is the square of the picture the crop names, inside bounds.
func cut(b image.Rectangle, c Crop) image.Rectangle {
	w, h := b.Dx(), b.Dy()
	short := min(w, h)
	side := short
	if c.Size > 0 && c.Size < 1 {
		side = max(1, min(short, int(c.Size*float64(short)+0.5)))
	}
	x, y := (w-side)/2, (h-side)/2
	if c.Size > 0 {
		x, y = int(clamp(c.X)*float64(w)+0.5), int(clamp(c.Y)*float64(h)+0.5)
	}
	x = max(0, min(x, w-side))
	y = max(0, min(y, h-side))
	return image.Rect(b.Min.X+x, b.Min.Y+y, b.Min.X+x+side, b.Min.Y+y+side)
}

func clamp(v float64) float64 {
	if v != v || v < 0 { // NaN or negative
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// hasTransparency reports whether any pixel is visibly see-through. The scaler
// can leave an opaque picture one or two steps short of fully opaque, which is
// not transparency, so the threshold sits a little under 255.
func hasTransparency(img *image.NRGBA) bool {
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] < 0xF0 {
			return true
		}
	}
	return false
}

func encode(img *image.NRGBA, transparent bool, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if transparent {
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		if err := enc.Encode(&buf, img); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
	if quality < 1 || quality > 100 {
		quality = 85
	}
	// An opaque picture is flattened onto white first, which changes nothing
	// for a pixel that is already opaque and keeps the encoder from reading an
	// alpha it has no use for.
	flat := image.NewRGBA(img.Bounds())
	draw.Draw(flat, flat.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(flat, flat.Bounds(), img, img.Bounds().Min, draw.Over)
	if err := jpeg.Encode(&buf, flat, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// orientedImage is a picture seen through one of the eight EXIF orientations,
// without copying a pixel: At maps a point of the upright picture to the point
// of the stored one.
type orientedImage struct {
	src         image.Image
	orientation int
	w, h        int // the stored picture's size
}

func orient(src image.Image, orientation int) image.Image {
	if orientation < 2 || orientation > 8 {
		return src
	}
	b := src.Bounds()
	return orientedImage{src: src, orientation: orientation, w: b.Dx(), h: b.Dy()}
}

func (o orientedImage) ColorModel() color.Model { return o.src.ColorModel() }

func (o orientedImage) Bounds() image.Rectangle {
	if o.orientation >= 5 {
		return image.Rect(0, 0, o.h, o.w)
	}
	return image.Rect(0, 0, o.w, o.h)
}

func (o orientedImage) At(x, y int) color.Color {
	var sx, sy int
	switch o.orientation {
	case 2: // mirrored horizontally
		sx, sy = o.w-1-x, y
	case 3: // rotated 180
		sx, sy = o.w-1-x, o.h-1-y
	case 4: // mirrored vertically
		sx, sy = x, o.h-1-y
	case 5: // transposed
		sx, sy = y, x
	case 6: // rotated 90 clockwise
		sx, sy = y, o.h-1-x
	case 7: // transversed
		sx, sy = o.w-1-y, o.h-1-x
	case 8: // rotated 90 counter-clockwise
		sx, sy = o.w-1-y, x
	default:
		sx, sy = x, y
	}
	b := o.src.Bounds()
	return o.src.At(b.Min.X+sx, b.Min.Y+sy)
}

// exifOrientation reads the orientation tag of a JPEG, 1 when there is none or
// it cannot be read. It walks the segments up to the start of the scan and
// reads the first Exif block's first directory, nothing more.
func exifOrientation(b []byte) int {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return 1
	}
	i := 2
	for i+4 <= len(b) {
		if b[i] != 0xFF {
			return 1
		}
		marker := b[i+1]
		if marker == 0xFF { // padding
			i++
			continue
		}
		if marker == 0xDA || marker == 0xD9 { // start of scan, end of image
			return 1
		}
		if marker >= 0xD0 && marker <= 0xD7 || marker == 0x01 {
			i += 2
			continue
		}
		n := int(b[i+2])<<8 | int(b[i+3])
		if n < 2 || i+2+n > len(b) {
			return 1
		}
		if marker == 0xE1 {
			if o, ok := tiffOrientation(b[i+4 : i+2+n]); ok {
				return o
			}
		}
		i += 2 + n
	}
	return 1
}

func tiffOrientation(seg []byte) (int, bool) {
	if len(seg) < 14 || !bytes.Equal(seg[:6], []byte("Exif\x00\x00")) {
		return 0, false
	}
	t := seg[6:]
	var u16 func([]byte) int
	var u32 func([]byte) int
	switch string(t[:2]) {
	case "II":
		u16 = func(p []byte) int { return int(p[0]) | int(p[1])<<8 }
		u32 = func(p []byte) int { return int(p[0]) | int(p[1])<<8 | int(p[2])<<16 | int(p[3])<<24 }
	case "MM":
		u16 = func(p []byte) int { return int(p[0])<<8 | int(p[1]) }
		u32 = func(p []byte) int { return int(p[0])<<24 | int(p[1])<<16 | int(p[2])<<8 | int(p[3]) }
	default:
		return 0, false
	}
	if u16(t[2:]) != 42 {
		return 0, false
	}
	ifd := u32(t[4:])
	if ifd < 8 || ifd+2 > len(t) {
		return 0, false
	}
	count := u16(t[ifd:])
	for k := 0; k < count; k++ {
		e := ifd + 2 + k*12
		if e+12 > len(t) {
			return 0, false
		}
		if u16(t[e:]) == 0x0112 { // Orientation
			if v := u16(t[e+8:]); v >= 1 && v <= 8 {
				return v, true
			}
			return 0, false
		}
	}
	return 0, false
}
