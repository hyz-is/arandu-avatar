package unit_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	avatar "github.com/hyz-is/arandu-avatar"
)

// flatten reads a catalogue into dotted keys.
func flatten(t *testing.T, locale string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(packageRoot(t), "resources", "lang", locale, "avatar.json"))
	if err != nil {
		t.Fatal(err)
	}
	var tree map[string]any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatalf("%s: %v", locale, err)
	}
	out := map[string]string{}
	var walk func(prefix string, node map[string]any)
	walk = func(prefix string, node map[string]any) {
		for key, value := range node {
			switch v := value.(type) {
			case map[string]any:
				walk(prefix+key+".", v)
			case string:
				out[prefix+key] = v
			default:
				t.Errorf("%s: %s%s is not a string", locale, prefix, key)
			}
		}
	}
	walk("", tree)
	return out
}

func TestEveryLocaleHasTheSameKeysAndTheSamePlaceholders(t *testing.T) {
	t.Parallel()

	en, pt := flatten(t, "en"), flatten(t, "pt-BR")
	placeholder := regexp.MustCompile(`:[a-z]+`)
	var missing []string
	for key, line := range en {
		other, ok := pt[key]
		if !ok {
			missing = append(missing, "pt-BR lacks "+key)
			continue
		}
		a, b := placeholder.FindAllString(line, -1), placeholder.FindAllString(other, -1)
		sort.Strings(a)
		sort.Strings(b)
		if strings.Join(a, ",") != strings.Join(b, ",") {
			t.Errorf("%s: placeholders %v in en and %v in pt-BR", key, a, b)
		}
	}
	for key := range pt {
		if _, ok := en[key]; !ok {
			missing = append(missing, "en lacks "+key)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("the catalogues disagree:\n%s", strings.Join(missing, "\n"))
	}
}

func TestEveryStyleHasAName(t *testing.T) {
	t.Parallel()

	for _, locale := range []string{"en", "pt-BR"} {
		lines := flatten(t, locale)
		for _, style := range avatar.Styles() {
			if lines["style."+string(style)] == "" {
				t.Errorf("%s has no name for %s", locale, style)
			}
		}
	}
}

func TestTheShippedLocalesAreListedAndTheLinesAreKeyedByTheGroup(t *testing.T) {
	t.Parallel()

	if got := strings.Join(avatar.Locales(), ","); got != "en,pt-BR" {
		t.Errorf("Locales() = %s", got)
	}
	if line := avatar.Lines("pt-BR")["avatar.editor.title"]; line != "Foto de perfil" {
		t.Errorf("avatar.editor.title = %q", line)
	}
	if avatar.Lines("xx") != nil {
		t.Error("an unshipped locale has lines")
	}
}

func TestEveryKeyTheCodeReadsIsInTheCatalogue(t *testing.T) {
	t.Parallel()

	en := flatten(t, "en")
	call := regexp.MustCompile(`(?:\.T|\.With|invalid|message|line)\(\s*(?:"[a-z_]+",\s*)?"([a-z_]+\.[a-z_.]+|[a-z_]+)"`)
	for _, source := range []string{"service.go", "handlers.go", "editor.go", "activity.go", "components/avatar-editor.kyse.go"} {
		body, err := os.ReadFile(filepath.Join(packageRoot(t), filepath.FromSlash(source)))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range call.FindAllStringSubmatch(string(body), -1) {
			key := m[1]
			if strings.Contains(key, ".") && !strings.HasSuffix(key, "_") && en[key] == "" {
				t.Errorf("%s reads %q, which the catalogue does not have", source, key)
			}
		}
	}
	for _, key := range []string{"validation.file_type", "validation.gravatar_none", "notice.gravatar_refreshed", "activity.avatar_gravatar_refreshed"} {
		if en[key] == "" {
			t.Errorf("the catalogue lacks %s", key)
		}
	}
}
