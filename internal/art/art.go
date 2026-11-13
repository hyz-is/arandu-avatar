// Package art draws the avatars nobody uploaded: deterministic SVG pictures
// made from a seed and a name, with no network call and no stored state.
//
// The same seed and palette always draw the same picture, byte for byte, so an
// avatar is stable across requests, servers and releases of a page. The styles
// are written here from the general ideas the market settled on -- a coloured
// tile with initials, a mirrored block pattern, a face on a coloured tile, soft
// overlapping blobs, concentric rings, stacked colour bands, loose geometry --
// and share no code with the libraries that popularised them (DiceBear,
// boring-avatars, Jdenticon, GitHub identicons).
//
// Every picture is flat colour on a square viewBox. There is no filter and no
// clip path, and the one gradient an avatar uses carries an identifier derived
// from its seed, so two avatars of the same seed share a definition that is
// identical and two of different seeds never collide.
package art

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"html"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// Style names one way of drawing an avatar.
type Style string

// The styles.
const (
	// Initials is a coloured tile with the letters of the name.
	Initials Style = "initials"
	// Identicon is a mirrored five-by-five block pattern on a light tile.
	Identicon Style = "identicon"
	// Pattern is a four-by-four tiling of quarter circles and diagonals.
	Pattern Style = "pattern"
	// Beam is a face on a coloured tile.
	Beam Style = "beam"
	// Marble is overlapping soft blobs.
	Marble Style = "marble"
	// Pixel is a mirrored eight-by-eight pixel pattern.
	Pixel Style = "pixel"
	// Ring is concentric rings.
	Ring Style = "ring"
	// Sunset is two blended colour bands, like a horizon.
	Sunset Style = "sunset"
	// Bauhaus is a square, a circle, a bar and a triangle.
	Bauhaus Style = "bauhaus"
	// Shapes is one regular polygon with a dot inside it.
	Shapes Style = "shapes"
	// Silhouette is the neutral head-and-shoulders of somebody unnamed.
	Silhouette Style = "silhouette"
)

// Styles are every style, in the order a picker lists them.
func Styles() []Style {
	return []Style{Initials, Identicon, Pattern, Beam, Marble, Pixel, Ring, Sunset, Bauhaus, Shapes, Silhouette}
}

// Valid reports whether the style is one of the shipped ones.
func Valid(s Style) bool {
	for _, known := range Styles() {
		if s == known {
			return true
		}
	}
	return false
}

// DefaultPalette is what the pictures are coloured with when the application
// passes none: eight mid-tone colours that sit on a light page and on a dark
// one.
var DefaultPalette = []string{
	"#2563eb", "#7c3aed", "#db2777", "#dc2626", "#ea580c", "#16a34a", "#0d9488", "#0891b2",
}

// Input is what a picture is drawn from.
type Input struct {
	// Seed makes the picture: the same seed draws the same picture. It is the
	// identifier of whoever the avatar is for, never something that changes.
	Seed string
	// Name is who it is for. Initials reads it; the other styles ignore it.
	Name string
	// Organization says the name is a company's: its initials are the first
	// letters of its first two words ("Acme Monitoramento Ltda" is AM), because
	// the last word of a company is so often its legal form.
	Organization bool
	// Palette is the colours, as #rgb or #rrggbb. Empty means DefaultPalette.
	// Callers validate it with NormalizePalette first; an entry that is not a
	// colour is replaced by a neutral grey rather than written into the markup.
	Palette []string
}

// NormalizePalette returns the colours as lowercase #rrggbb, or the first one
// that is not a colour.
func NormalizePalette(colours []string) ([]string, error) {
	out := make([]string, 0, len(colours))
	for _, c := range colours {
		n, ok := normalizeColour(c)
		if !ok {
			return nil, fmt.Errorf("%q is not a colour: write it as #rgb or #rrggbb", c)
		}
		out = append(out, n)
	}
	return out, nil
}

func normalizeColour(c string) (string, bool) {
	c = strings.ToLower(strings.TrimSpace(c))
	if len(c) == 4 && c[0] == '#' {
		c = "#" + string([]byte{c[1], c[1], c[2], c[2], c[3], c[3]})
	}
	if len(c) != 7 || c[0] != '#' {
		return "", false
	}
	for i := 1; i < 7; i++ {
		if !strings.ContainsRune("0123456789abcdef", rune(c[i])) {
			return "", false
		}
	}
	return c, true
}

// InitialsOf are the first letter of the first and of the last word of the name,
// upper-cased: "Ada Byron Lovelace" is AL, and "madonna" is M. It walks runes,
// not bytes, and skips words that hold no letter or digit.
func InitialsOf(name string) string {
	var firsts []rune
	for _, word := range strings.Fields(name) {
		for _, r := range word {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				firsts = append(firsts, unicode.ToUpper(r))
				break
			}
		}
	}
	switch len(firsts) {
	case 0:
		return ""
	case 1:
		return string(firsts)
	}
	return string([]rune{firsts[0], firsts[len(firsts)-1]})
}

// OrganizationInitialsOf are the first letters of the first two words of a
// company's name, upper-cased: "Acme Monitoramento Ltda" is AM and "Acme" is A.
func OrganizationInitialsOf(name string) string {
	var firsts []rune
	for _, word := range strings.Fields(name) {
		for _, r := range word {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				firsts = append(firsts, unicode.ToUpper(r))
				break
			}
		}
		if len(firsts) == 2 {
			break
		}
	}
	return string(firsts)
}

// size is the side of the viewBox.
const size = 80

// Render draws the picture as a complete SVG document element. An unknown style
// draws Initials.
func Render(style Style, in Input) string {
	d := newDrawing(in)
	var body string
	switch style {
	case Identicon:
		body = d.identicon()
	case Pattern:
		body = d.pattern()
	case Beam:
		body = d.beam()
	case Marble:
		body = d.marble()
	case Pixel:
		body = d.pixel()
	case Ring:
		body = d.ring()
	case Sunset:
		body = d.sunset()
	case Bauhaus:
		body = d.bauhaus()
	case Shapes:
		body = d.shapes()
	case Silhouette:
		body = d.silhouette()
	default:
		body = d.initials(in)
	}
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 80 80" width="80" height="80" focusable="false" aria-hidden="true">` + body + `</svg>`
}

// DataURI is the picture as an image address a page, an e-mail or an <img> can
// hold without a request.
func DataURI(style Style, in Input) string {
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(Render(style, in)))
}

// drawing is the state of one picture: the colours and the stream of numbers
// the seed makes.
type drawing struct {
	palette []string
	seed    [32]byte
	counter uint32
	pool    []byte
	id      string
}

func newDrawing(in Input) *drawing {
	palette := in.Palette
	if len(palette) == 0 {
		palette = DefaultPalette
	}
	clean := make([]string, 0, len(palette))
	for _, c := range palette {
		if n, ok := normalizeColour(c); ok {
			clean = append(clean, n)
		} else {
			clean = append(clean, "#6b7280")
		}
	}
	seed := sha256.Sum256([]byte("arandu-avatar\x00" + in.Seed))
	return &drawing{palette: clean, seed: seed, pool: seed[:], id: fmt.Sprintf("avs%x", seed[:4])}
}

// next is the next byte of the stream. When the seed's own digest runs out the
// stream continues with the digest of the digest and a counter, so it never
// repeats and never fails.
func (d *drawing) next() int {
	if len(d.pool) == 0 {
		d.counter++
		var c [4]byte
		binary.BigEndian.PutUint32(c[:], d.counter)
		sum := sha256.Sum256(append(d.seed[:], c[:]...))
		d.pool = sum[:]
	}
	b := d.pool[0]
	d.pool = d.pool[1:]
	return int(b)
}

// intn is a number from 0 to n-1.
func (d *drawing) intn(n int) int {
	if n <= 1 {
		return 0
	}
	return (d.next()<<8 | d.next()) % n
}

// span is a number between lo and hi, both included.
func (d *drawing) span(lo, hi int) int { return lo + d.intn(hi-lo+1) }

// colours picks n colours that differ from each other whenever the palette has
// that many, starting at a place the seed chose and stepping by a stride that
// shares no factor with the palette's length.
func (d *drawing) colours(n int) []string {
	size := len(d.palette)
	start := d.intn(size)
	stride := 1 + d.intn(size)
	for gcd(stride, size) != 1 {
		stride++
	}
	out := make([]string, n)
	for i := range out {
		out[i] = d.palette[(start+i*stride)%size]
	}
	return out
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// rgb reads #rrggbb.
func rgb(hex string) (float64, float64, float64) {
	v, _ := strconv.ParseUint(hex[1:], 16, 32)
	return float64(v >> 16 & 0xff), float64(v >> 8 & 0xff), float64(v & 0xff)
}

// mix is a colour moved toward white by t, from 0 (itself) to 1 (white).
func mix(hex string, t float64) string {
	r, g, b := rgb(hex)
	f := func(c float64) int { return int(math.Round(c + (255-c)*t)) }
	return fmt.Sprintf("#%02x%02x%02x", f(r), f(g), f(b))
}

// shade is a colour moved toward black by t.
func shade(hex string, t float64) string {
	r, g, b := rgb(hex)
	f := func(c float64) int { return int(math.Round(c * (1 - t))) }
	return fmt.Sprintf("#%02x%02x%02x", f(r), f(g), f(b))
}

// luminance is the WCAG relative luminance of a colour.
func luminance(hex string) float64 {
	r, g, b := rgb(hex)
	lin := func(c float64) float64 {
		c /= 255
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

// on is white or near-black, whichever reads better over the colour.
func on(hex string) string {
	l := luminance(hex)
	white := 1.05 / (l + 0.05)
	dark := (l + 0.05) / (luminance("#111827") + 0.05)
	if white >= dark {
		return "#ffffff"
	}
	return "#111827"
}

func rect(x, y, w, h float64, fill string) string {
	return fmt.Sprintf(`<rect x="%s" y="%s" width="%s" height="%s" fill="%s"/>`, f(x), f(y), f(w), f(h), fill)
}

// f writes a number without a trailing zero.
func f(v float64) string { return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64) }

func (d *drawing) initials(in Input) string {
	text := InitialsOf(in.Name)
	if in.Organization {
		text = OrganizationInitialsOf(in.Name)
	}
	bg := d.colours(1)[0]
	if text == "" {
		return d.silhouetteOn(bg)
	}
	fg := on(bg)
	fontSize := 34
	if len([]rune(text)) == 1 {
		fontSize = 38
	}
	return rect(0, 0, size, size, bg) +
		fmt.Sprintf(`<text x="40" y="41" text-anchor="middle" dominant-baseline="central" font-family="ui-sans-serif,system-ui,-apple-system,'Segoe UI',Roboto,sans-serif" font-size="%d" font-weight="600" fill="%s">%s</text>`,
			fontSize, fg, html.EscapeString(text))
}

func (d *drawing) identicon() string {
	c := d.colours(1)[0]
	var b strings.Builder
	b.WriteString(rect(0, 0, size, size, mix(c, 0.88)))
	const cell = 12.0
	const margin = 10.0
	var cols [3][5]bool
	for x := range cols {
		for y := range cols[x] {
			cols[x][y] = d.next()%2 == 0
		}
	}
	for x := 0; x < 5; x++ {
		src := x
		if x > 2 {
			src = 4 - x
		}
		for y := 0; y < 5; y++ {
			if cols[src][y] {
				b.WriteString(rect(margin+float64(x)*cell, margin+float64(y)*cell, cell, cell, c))
			}
		}
	}
	return b.String()
}

func (d *drawing) pattern() string {
	c := d.colours(2)
	var b strings.Builder
	b.WriteString(rect(0, 0, size, size, c[0]))
	const tile = 20.0
	for row := 0; row < 4; row++ {
		for col := 0; col < 4; col++ {
			x, y := float64(col)*tile, float64(row)*tile
			switch pick := d.intn(7); {
			case pick < 4:
				b.WriteString(quarter(x, y, tile, pick, c[1]))
			case pick < 6:
				b.WriteString(diagonal(x, y, tile, pick-4, c[1]))
			}
		}
	}
	return b.String()
}

// quarter is a quarter circle whose centre is one corner of the tile.
func quarter(x, y, s float64, corner int, fill string) string {
	type pt struct{ x, y float64 }
	corners := [4]pt{{x, y}, {x + s, y}, {x + s, y + s}, {x, y + s}}
	c := corners[corner]
	next := corners[(corner+1)%4]
	prev := corners[(corner+3)%4]
	// The two edges leaving the corner, as unit steps into the tile.
	d1 := pt{sign(next.x - c.x), sign(next.y - c.y)}
	d2 := pt{sign(prev.x - c.x), sign(prev.y - c.y)}
	sweep := 0
	if d1.x*d2.y-d1.y*d2.x > 0 {
		sweep = 1
	}
	return fmt.Sprintf(`<path d="M%s %sL%s %sA%s %s 0 0 %d %s %sZ" fill="%s"/>`,
		f(c.x), f(c.y), f(c.x+d1.x*s), f(c.y+d1.y*s), f(s), f(s), sweep, f(c.x+d2.x*s), f(c.y+d2.y*s), fill)
}

func sign(v float64) float64 {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

// diagonal is half a tile, cut corner to corner.
func diagonal(x, y, s float64, which int, fill string) string {
	var pts string
	if which == 0 {
		pts = fmt.Sprintf("%s,%s %s,%s %s,%s", f(x), f(y), f(x+s), f(y), f(x), f(y+s))
	} else {
		pts = fmt.Sprintf("%s,%s %s,%s %s,%s", f(x+s), f(y), f(x+s), f(y+s), f(x), f(y+s))
	}
	return fmt.Sprintf(`<polygon points="%s" fill="%s"/>`, pts, fill)
}

func (d *drawing) beam() string {
	c := d.colours(2)
	var b strings.Builder
	b.WriteString(rect(0, 0, size, size, c[0]))
	// A rotated tile of the second colour behind the face, off-centre.
	tx, ty, rot := d.span(-8, 8), d.span(-8, 8), d.span(0, 359)
	b.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="80" height="80" rx="%d" fill="%s" transform="rotate(%d 40 40)"/>`,
		tx, ty, d.span(10, 40), c[1], rot))
	ink := on(c[1])
	fx, fy := d.span(-6, 6), d.span(-4, 6)
	b.WriteString(fmt.Sprintf(`<g transform="translate(%d %d)">`, fx, fy))
	gap := d.span(0, 6)
	eye := 3.2 + float64(d.span(0, 12))/10
	if d.intn(2) == 0 {
		b.WriteString(fmt.Sprintf(`<circle cx="%s" cy="34" r="%s" fill="%s"/><circle cx="%s" cy="34" r="%s" fill="%s"/>`,
			f(float64(29-gap/2)), f(eye), ink, f(float64(51+gap/2)), f(eye), ink))
	} else {
		w := eye * 2
		b.WriteString(fmt.Sprintf(`<rect x="%s" y="%s" width="%s" height="%s" rx="1.5" fill="%s"/><rect x="%s" y="%s" width="%s" height="%s" rx="1.5" fill="%s"/>`,
			f(float64(29-gap/2)-eye), f(34-eye), f(w), f(w), ink, f(float64(51+gap/2)-eye), f(34-eye), f(w), f(w), ink))
	}
	if d.intn(2) == 0 {
		b.WriteString(fmt.Sprintf(`<path d="M30 49Q40 %dQ50 49" fill="none" stroke="%s" stroke-width="3" stroke-linecap="round"/>`, d.span(55, 61), ink))
	} else {
		b.WriteString(fmt.Sprintf(`<path d="M30 50H50A10 %d 0 0 1 30 50Z" fill="%s"/>`, d.span(5, 10), ink))
	}
	b.WriteString(`</g>`)
	return b.String()
}

func (d *drawing) marble() string {
	c := d.colours(4)
	var b strings.Builder
	b.WriteString(rect(0, 0, size, size, c[0]))
	for i := 1; i < 4; i++ {
		for k := 0; k < 2; k++ {
			b.WriteString(fmt.Sprintf(`<ellipse cx="%d" cy="%d" rx="%d" ry="%d" fill="%s" transform="rotate(%d %d %d)"/>`,
				d.span(10, 70), d.span(10, 70), d.span(18, 38), d.span(10, 24), c[i], d.span(0, 179), 40, 40))
		}
	}
	return b.String()
}

func (d *drawing) pixel() string {
	c := d.colours(4)
	var b strings.Builder
	b.WriteString(rect(0, 0, size, size, c[0]))
	var cols [4][8]int
	for x := range cols {
		for y := range cols[x] {
			cols[x][y] = d.intn(5)
		}
	}
	for x := 0; x < 8; x++ {
		src := x
		if x > 3 {
			src = 7 - x
		}
		for y := 0; y < 8; y++ {
			// Index 0 and 1 leave the tile as it is: a pixel picture of solid
			// cells looks like noise, and one with gaps looks drawn.
			if v := cols[src][y]; v >= 2 {
				b.WriteString(rect(float64(x)*10, float64(y)*10, 10, 10, c[v-1]))
			}
		}
	}
	return b.String()
}

func (d *drawing) ring() string {
	c := d.colours(5)
	var b strings.Builder
	b.WriteString(rect(0, 0, size, size, c[0]))
	cx, cy := 40+d.span(-4, 4), 40+d.span(-4, 4)
	for i, r := range []int{36, 28, 20, 12} {
		b.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%d" r="%d" fill="%s"/>`, cx, cy, r, c[i+1]))
	}
	return b.String()
}

func (d *drawing) sunset() string {
	c := d.colours(4)
	top, bottom := d.id+"t", d.id+"b"
	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<defs><linearGradient id="%s" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="%s"/><stop offset="1" stop-color="%s"/></linearGradient>`+
		`<linearGradient id="%s" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="%s"/><stop offset="1" stop-color="%s"/></linearGradient></defs>`,
		top, c[0], c[1], bottom, c[2], c[3]))
	b.WriteString(fmt.Sprintf(`<rect x="0" y="0" width="80" height="40" fill="url(#%s)"/><rect x="0" y="40" width="80" height="40" fill="url(#%s)"/>`, top, bottom))
	return b.String()
}

func (d *drawing) bauhaus() string {
	c := d.colours(5)
	var b strings.Builder
	b.WriteString(rect(0, 0, size, size, c[0]))
	b.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" fill="%s" transform="rotate(%d 40 40)"/>`,
		d.span(2, 22), d.span(2, 22), d.span(34, 52), d.span(34, 52), c[1], d.span(0, 89)))
	b.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%d" r="%d" fill="%s"/>`, d.span(20, 60), d.span(20, 60), d.span(12, 22), c[2]))
	b.WriteString(fmt.Sprintf(`<rect x="0" y="%d" width="80" height="%d" fill="%s" transform="rotate(%d 40 40)"/>`,
		d.span(24, 48), d.span(4, 9), c[3], d.span(0, 179)))
	tx, ty := d.span(10, 50), d.span(10, 50)
	b.WriteString(fmt.Sprintf(`<polygon points="%d,%d %d,%d %d,%d" fill="%s"/>`, tx, ty+26, tx+13, ty, tx+26, ty+26, c[4]))
	return b.String()
}

func (d *drawing) shapes() string {
	c := d.colours(3)
	var b strings.Builder
	b.WriteString(rect(0, 0, size, size, mix(c[0], 0.82)))
	sides := d.span(3, 7)
	rot := float64(d.span(0, 359))
	var pts []string
	for i := 0; i < sides; i++ {
		a := (rot + float64(i)*360/float64(sides)) * math.Pi / 180
		pts = append(pts, fmt.Sprintf("%s,%s", f(40+30*math.Sin(a)), f(40-30*math.Cos(a))))
	}
	b.WriteString(fmt.Sprintf(`<polygon points="%s" fill="%s"/>`, strings.Join(pts, " "), c[1]))
	b.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%d" r="%d" fill="%s"/>`, 40+d.span(-5, 5), 40+d.span(-5, 5), d.span(7, 12), c[2]))
	return b.String()
}

func (d *drawing) silhouette() string { return d.silhouetteOn(d.colours(1)[0]) }

func (d *drawing) silhouetteOn(bg string) string {
	figure := mix(bg, 0.72)
	return rect(0, 0, size, size, bg) +
		fmt.Sprintf(`<circle cx="40" cy="31" r="13" fill="%s"/><path d="M14 84C14 62 25 52 40 52S66 62 66 84Z" fill="%s"/>`, figure, figure)
}
