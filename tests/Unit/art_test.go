package unit_test

import (
	"encoding/base64"
	"encoding/xml"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/hyz-is/arandu-avatar/internal/art"
)

func TestEveryStyleDrawsTheSameSVGForTheSameSeed(t *testing.T) {
	t.Parallel()

	for _, style := range art.Styles() {
		in := art.Input{Seed: "user:ada", Name: "Ada Lovelace"}
		first, second := art.Render(style, in), art.Render(style, in)
		if first != second {
			t.Errorf("%s: the same seed drew two different pictures", style)
		}
		other := art.Render(style, art.Input{Seed: "user:grace", Name: "Ada Lovelace"})
		if style != art.Initials && style != art.Silhouette && first == other {
			t.Errorf("%s: two seeds drew the same picture", style)
		}
	}
}

func TestEveryStyleIsWellFormedSVGWithNothingACSPWouldBlock(t *testing.T) {
	t.Parallel()

	forbidden := regexp.MustCompile(`(?i)<script|<style|\sstyle=|\son[a-z]+=|javascript:|<foreignObject|<image|href=`)
	for _, style := range art.Styles() {
		for _, seed := range []string{"user:1", "user:ada", "organization:acme", "user:" + strings.Repeat("z", 64), "user:ünï:cödé"} {
			svg := art.Render(style, art.Input{Seed: seed, Name: `Ada "<b>" & Lovelace`})
			decoder := xml.NewDecoder(strings.NewReader(svg))
			for {
				if _, err := decoder.Token(); err != nil {
					if err != io.EOF {
						t.Fatalf("%s/%s: not well-formed XML: %v\n%s", style, seed, err, svg)
					}
					break
				}
			}
			if forbidden.MatchString(svg) {
				t.Errorf("%s/%s: the drawing carries something a strict policy blocks: %s", style, seed, svg)
			}
			if !strings.HasPrefix(svg, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 80 80"`) {
				t.Errorf("%s: the drawing is not a square SVG with a viewBox", style)
			}
		}
	}
}

func TestInitialsAreTheFirstAndTheLastWord(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]string{
		"Ada Byron Lovelace": "AL", "madonna": "M", "  ": "", "": "", "érica silva": "ÉS",
		"(Acme) Ltda.": "AL", "José": "J", "João da Silva Sauro": "JS", "👩‍💻 Ada": "A",
	} {
		if got := art.InitialsOf(name); got != want {
			t.Errorf("InitialsOf(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestTheInitialsAreWrittenEscaped(t *testing.T) {
	t.Parallel()

	svg := art.Render(art.Initials, art.Input{Seed: "x", Name: "<b>& <i>"})
	if strings.Contains(svg, "<b>") || strings.Contains(svg, "<i>") {
		t.Errorf("the name reached the markup unescaped: %s", svg)
	}
}

func TestAPaletteEntryThatIsNotAColourIsNeverWrittenIntoTheMarkup(t *testing.T) {
	t.Parallel()

	svg := art.Render(art.Marble, art.Input{Seed: "x", Palette: []string{`red" onload="x`, "#abc", "#12345g"}})
	if strings.Contains(svg, "onload") || strings.Contains(svg, "red") {
		t.Errorf("a palette entry was written into the markup: %s", svg)
	}
	if _, err := art.NormalizePalette([]string{"#fff", `"><script>`}); err == nil {
		t.Error("NormalizePalette accepted something that is not a colour")
	}
	got, err := art.NormalizePalette([]string{"#ABC", " #00FF7f "})
	if err != nil || got[0] != "#aabbcc" || got[1] != "#00ff7f" {
		t.Errorf("NormalizePalette = %v, %v", got, err)
	}
}

func TestAnUnknownStyleDrawsInitialsAndTheListIsClosed(t *testing.T) {
	t.Parallel()

	in := art.Input{Seed: "user:ada", Name: "Ada Lovelace"}
	if art.Render("nonsense", in) != art.Render(art.Initials, in) {
		t.Error("an unknown style does not draw initials")
	}
	if art.Valid("nonsense") || !art.Valid(art.Beam) {
		t.Error("Valid does not tell the shipped styles from the rest")
	}
	if len(art.Styles()) < 10 {
		t.Errorf("only %d styles are shipped", len(art.Styles()))
	}
}

func TestTwoAvatarsOfOneSeedShareGradientIdsAndOfTwoDoNot(t *testing.T) {
	t.Parallel()

	id := regexp.MustCompile(`id="([^"]+)"`)
	a := id.FindString(art.Render(art.Sunset, art.Input{Seed: "user:1"}))
	b := id.FindString(art.Render(art.Sunset, art.Input{Seed: "user:2"}))
	if a == "" || a == b {
		t.Errorf("gradient ids %q and %q collide or are missing", a, b)
	}
}

func TestTheDataURIIsTheDrawingBase64(t *testing.T) {
	t.Parallel()

	in := art.Input{Seed: "user:ada", Name: "Ada Lovelace"}
	uri := art.DataURI(art.Beam, in)
	raw, ok := strings.CutPrefix(uri, "data:image/svg+xml;base64,")
	if !ok {
		t.Fatalf("not an SVG data URI: %.40s", uri)
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || string(decoded) != art.Render(art.Beam, in) {
		t.Errorf("the data URI does not carry the drawing: %v", err)
	}
}

func TestTheTextOfInitialsReadsOnTheTile(t *testing.T) {
	t.Parallel()

	// Whatever the palette, the letters are white or near-black, whichever
	// contrasts more with the tile.
	for _, colour := range []string{"#ffffff", "#000000", "#facc15", "#1e3a8a"} {
		svg := art.Render(art.Initials, art.Input{Seed: "x", Name: "Ada Lovelace", Palette: []string{colour, colour}})
		want := "#ffffff"
		if colour == "#ffffff" || colour == "#facc15" {
			want = "#111827"
		}
		if !strings.Contains(svg, `fill="`+want+`"`) {
			t.Errorf("on %s the letters are not %s: %s", colour, want, svg)
		}
	}
}

func TestAnOrganizationsInitialsAreItsFirstTwoWords(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]string{"Acme Monitoramento Ltda": "AM", "Acme": "A", "": "", "  (Acme)  Rastreio  S.A.": "AR", "ótica da vila": "ÓD"} {
		if got := art.OrganizationInitialsOf(name); got != want {
			t.Errorf("OrganizationInitialsOf(%q) = %q, want %q", name, got, want)
		}
	}
	person := art.Render(art.Initials, art.Input{Seed: "s", Name: "Acme Monitoramento Ltda"})
	company := art.Render(art.Initials, art.Input{Seed: "s", Name: "Acme Monitoramento Ltda", Organization: true})
	if !strings.Contains(person, ">AL<") || !strings.Contains(company, ">AM<") {
		t.Errorf("person %s, company %s", person, company)
	}
}

// pathArguments is how many numbers each path command takes. A command followed
// by a run of numbers repeats, so the run has to be a whole multiple of it.
var pathArguments = map[byte]int{
	'M': 2, 'L': 2, 'H': 1, 'V': 1, 'C': 6, 'S': 4, 'Q': 4, 'T': 2, 'A': 7, 'Z': 0,
}

// checkPathData reports what is wrong with the `d` of a path, or nothing. A
// browser that meets a bad one draws nothing of the path and writes an error in
// the console: "<path> attribute d: Expected number".
func checkPathData(d string) string {
	token := regexp.MustCompile(`[MmLlHhVvCcSsQqTtAaZz]|-?\d*\.?\d+(?:[eE][-+]?\d+)?`)
	rest := strings.TrimSpace(token.ReplaceAllString(d, ""))
	rest = strings.NewReplacer(",", "", " ", "", "\n", "", "\t", "").Replace(rest)
	if rest != "" {
		return "unexpected " + rest
	}
	tokens := token.FindAllString(d, -1)
	if len(tokens) == 0 || !strings.ContainsAny(tokens[0], "Mm") {
		return "does not start with a moveto"
	}
	for at := 0; at < len(tokens); {
		command := strings.ToUpper(tokens[at])[0]
		want, known := pathArguments[command]
		if !known {
			return "a number where a command was expected: " + tokens[at]
		}
		at++
		count := 0
		for at < len(tokens) && !strings.ContainsAny(tokens[at][:1], "MmLlHhVvCcSsQqTtAaZz") {
			count++
			at++
		}
		switch {
		case want == 0 && count != 0:
			return "Z takes no numbers"
		case want != 0 && (count == 0 || count%want != 0):
			return string(command) + " has " + strings.Repeat("#", count) + " numbers, which is not a multiple of " + string(rune('0'+want))
		}
	}
	return ""
}

func TestThePathDataCheckerCatchesWhatABrowserRefuses(t *testing.T) {
	t.Parallel()

	for d, want := range map[string]bool{
		"M30 49Q40 56 50 49":          true,
		"M30 49Q40 56Q50 49":          false, // the first Q has two numbers where it needs four
		"M30 50H50A10 7 0 0 1 30 50Z": true,
		"M0 0L10":                     false,
		"L1 1":                        false,
	} {
		if got := checkPathData(d) == ""; got != want {
			t.Errorf("%q: valid is %v, want %v (%s)", d, got, want, checkPathData(d))
		}
	}
}

// Every path of every drawing is path data a browser can read, across many
// seeds because the drawings draw different shapes for different seeds. The
// "Carinha" (Beam) drew its mouth as `Q40 56Q50 49` for half of them, which has
// two numbers where a quadratic curve needs four.
func TestEveryPathOfEveryDrawingIsReadableByABrowser(t *testing.T) {
	t.Parallel()

	paths := regexp.MustCompile(`<path\b[^>]*\sd="([^"]*)"`)
	for _, style := range art.Styles() {
		for n := 0; n < 300; n++ {
			svg := art.Render(style, art.Input{Seed: "user:" + strings.Repeat("a", n%7) + string(rune('A'+n%26)) + string(rune('0'+n%10)) + strings.Repeat("b", n/26), Name: "Ada Lovelace"})
			for _, found := range paths.FindAllStringSubmatch(svg, -1) {
				if problem := checkPathData(found[1]); problem != "" {
					t.Fatalf("%s: the path %q is not readable: %s", style, found[1], problem)
				}
			}
		}
	}
}

func TestTheCarinhasMouthIsAQuadraticCurveWithItsControlAndItsEndPoint(t *testing.T) {
	t.Parallel()

	mouth := regexp.MustCompile(`d="M30 49Q40 (\d+) 50 49"`)
	smiles := 0
	for n := 0; n < 200 && smiles < 5; n++ {
		svg := art.Render(art.Beam, art.Input{Seed: "user:carinha-" + string(rune('A'+n%26)) + strings.Repeat("x", n/26), Name: "Ada"})
		if strings.Contains(svg, `d="M30 49Q`) {
			if !mouth.MatchString(svg) {
				t.Fatalf("the mouth is not `M30 49Q40 N 50 49`: %s", svg)
			}
			smiles++
		}
	}
	if smiles == 0 {
		t.Fatal("no seed drew the curved mouth, so the test checked nothing")
	}
}
