package avatar

import (
	"embed"
	"io/fs"
	"sort"
	"strings"

	"github.com/arandu-io/framework/validation"
	"github.com/arandu-io/hesape/translation"
)

// The sentences of this package's editor, messages and activity descriptions.
// They are embedded and not published: an application that wants other words
// writes them in its own catalogue, under the same keys, and Config.Translator
// is asked first. One override path, the one the application already has.

//go:embed resources/lang
var langSources embed.FS

// TranslationGroup is the file every key of this package lives in, and the
// prefix of every key: "avatar.editor.title".
const TranslationGroup = "avatar"

var shipped, shippedLocales = mustShipped()

func mustShipped() (*translation.Translator, []string) {
	root, err := fs.Sub(langSources, "resources/lang")
	if err != nil {
		panic("avatar: the embedded catalogue has no lang directory: " + err.Error())
	}
	loader, err := translation.NewFileLoader(root, ".")
	if err != nil {
		panic("avatar: the embedded catalogue does not parse: " + err.Error())
	}
	entries, err := fs.ReadDir(root, ".")
	if err != nil {
		panic("avatar: the embedded catalogue cannot be listed: " + err.Error())
	}
	locales := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			locales = append(locales, entry.Name())
		}
	}
	sort.Strings(locales)
	if len(locales) == 0 {
		panic("avatar: the embedded catalogue holds no locale, so every screen would draw its keys")
	}
	return translation.New(loader, FallbackLocale, FallbackLocale), locales
}

// Locales are the languages the package ships.
func Locales() []string { return append([]string(nil), shippedLocales...) }

// Lines are the sentences this package ships for one locale, keyed the way a
// translator asks for them ("avatar.validation.file_type"), and nil for a locale
// it does not ship. An application reads them to see what there is to override,
// or to check a catalogue of its own against them.
func Lines(locale string) translation.Lines {
	lines := shipped.GetLoader().Load(locale, TranslationGroup, "*")
	if len(lines) == 0 {
		return nil
	}
	out := make(translation.Lines, len(lines))
	for item, line := range lines {
		out[TranslationGroup+"."+item] = line
	}
	return out
}

// Labels are the sentences of this package in one locale.
type Labels struct {
	Locale     string
	translator *translation.Translator
}

// labelsOf are the sentences the configuration asks for.
func labelsOf(c Config) Labels {
	locale := strings.TrimSpace(c.Locale)
	if locale == "" {
		locale = DefaultLocale
	}
	return Labels{Locale: locale, translator: c.Translator}
}

// Labels answers the sentences of the module's locale.
func (m *Module) Labels() Labels { return labelsOf(m.cfg) }

// T reads a key of this package: "editor.title".
func (l Labels) T(key string) string { return l.line(key, nil) }

// With reads a key with its placeholders filled: With("editor.photo_hint",
// "mb", "5").
func (l Labels) With(key string, pairs ...string) string {
	replace := translation.Replace{}
	for i := 0; i+1 < len(pairs); i += 2 {
		replace[pairs[i]] = pairs[i+1]
	}
	return l.line(key, replace)
}

// line reads a key with its placeholders filled, asking the application's
// translator first and the shipped catalogue after it.
func (l Labels) line(key string, replace translation.Replace) string {
	full := TranslationGroup + "." + key
	if l.translator != nil && l.translator.Has(l.Locale, full) {
		return l.translator.Get(l.Locale, full, replace)
	}
	return shipped.Get(l.Locale, full, replace)
}

// Style is the name of a generated style: "Iniciais".
func (l Labels) Style(s Style) string { return l.T("style." + string(s)) }

// Field is what a person reads in place of an input's name.
func (l Labels) Field(name string) string {
	full := "field." + name
	if l.translator != nil && l.translator.Has(l.Locale, TranslationGroup+"."+full) || shipped.Has(l.Locale, TranslationGroup+"."+full) {
		return l.T(full)
	}
	return name
}

// Summary is one line per rejected message, the field named before it. Nil when
// nothing was rejected.
func (l Labels) Summary(errs validation.Errors) []string {
	if len(errs) == 0 {
		return nil
	}
	out := make([]string, 0, len(errs))
	for _, field := range errs.Keys() {
		for _, msg := range errs.Get(field) {
			out = append(out, l.With("validation.summary", "field", l.Field(field), "message", msg))
		}
	}
	return out
}

// message is a validation message of this package, in the locale of the labels.
func (l Labels) message(key string, pairs ...string) string {
	return l.With("validation."+key, pairs...)
}

// invalid is a rejected input with one message on one field.
func (l Labels) invalid(field, key string, pairs ...string) validation.Errors {
	return validation.Errors{field: {l.message(key, pairs...)}}
}
