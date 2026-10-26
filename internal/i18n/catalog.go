// Package i18n owns language-neutral message identifiers and the first
// candidate English, German and Thai product catalogs. These catalogs are
// not yet registered as a language selector or production claim.
package i18n

import (
    "embed"
    "encoding/json"
    "errors"
    "fmt"
    "regexp"
    "sort"
    "strconv"
    "strings"
    "sync"
)

//go:embed catalog/*.json
var catalogFiles embed.FS

type Locale string

const (
    English Locale = "en"
    German  Locale = "de"
    Thai    Locale = "th"
)

var ErrInvalidCatalog = errors.New("product localization catalog invalid")
var templateSlot = regexp.MustCompile(`\{([a-z][a-z0-9_]*)\}`)

var allLocales = []Locale{English, German, Thai}
var loadOnce sync.Once
var localized map[Locale]map[string]string
var loadError error

func Supported() []Locale {
    return append([]Locale(nil), allLocales...)
}

// Normalize accepts only explicit supported language subtags. Unsupported
// values, invalid BCP47-like tokens and wildcards fall back to English.
func Normalize(input string) Locale {
    if len(input) == 0 || len(input) > 64 {
        return English
    }
    input = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(input, "_", "-")))
    parts := strings.Split(input, "-")
    for _, part := range parts {
        if part == "" || len(part) > 8 { return English }
        for _, r := range part {
            if r < 'a' || r > 'z' && (r < '0' || r > '9') {
                return English
            }
        }
    }
    switch parts[0] {
    case "en": return English
    case "de": return German
    case "th": return Thai
    default: return English
    }
}

// Negotiate parses an untrusted Accept-Language header. It never yields an
// implicit permission/actor identity, never reads user storage and never
// changes canonical document metadata. Tie-break follows header order.
func Negotiate(header string) Locale {
    if len(header) > 1024 { return English }
    bestQ := -1.0
    best := English
    for i, raw := range strings.Split(header, ",") {
        if i >= 32 { break }
        fields := strings.Split(strings.TrimSpace(raw), ";")
        if len(fields) > 2 { continue }
        code := strings.TrimSpace(fields[0])
        if code == "" { continue }
        q := 1.0
        if len(fields) == 2 {
            flag := strings.TrimSpace(fields[1])
            if len(flag) < 3 || !strings.HasPrefix(strings.ToLower(flag), "q=") {
                continue
            }
            var err error
            q, err = strconv.ParseFloat(strings.TrimSpace(flag[2:]), 64)
            if err != nil || q < 0 || q > 1 { continue }
        }
        if q == 0 { continue }
        // Unknown languages don't defeat a lower-priority supported locale.
        normalized := Normalize(code)
        primary := strings.ToLower(strings.Split(strings.ReplaceAll(code, "_", "-"), "-")[0])
        if primary != string(normalized) || code == "*" { continue }
        if q > bestQ { bestQ, best = q, normalized }
    }
    return best
}

func load() {
    localized = make(map[Locale]map[string]string, len(allLocales))
    for _, locale := range allLocales {
        b, err := catalogFiles.ReadFile("catalog/" + string(locale) + ".json")
        if err != nil { loadError = fmt.Errorf("%w: read %s: %v",ErrInvalidCatalog,locale,err); return }
        var dictionary map[string]string
        if err = json.Unmarshal(b, &dictionary); err != nil {
            loadError = fmt.Errorf("%w: parse %s: %v",ErrInvalidCatalog,locale,err)
            return
        }
        localized[locale] = dictionary
    }
    loadError = ValidateCatalogs(localized)
}

// ValidateCatalogs makes omissions and placeholder drift a deterministic
// qualification failure, rather than leaking partial translated journeys.
func ValidateCatalogs(all map[Locale]map[string]string) error {
    base := all[English]
    if len(base) == 0 { return fmt.Errorf("%w: missing English baseline",ErrInvalidCatalog) }
    for _, lang := range allLocales {
        entries := all[lang]
        if len(entries) != len(base) {
            return fmt.Errorf("%w: %s has %d keys, expected %d",ErrInvalidCatalog,lang,len(entries),len(base))
        }
        for key, text := range base {
            translated, ok := entries[key]
            if !ok || key == "" || text == "" || translated == "" {
                return fmt.Errorf("%w: missing or empty %s/%s",ErrInvalidCatalog,lang,key)
            }
            if strings.ContainsAny(translated, "<>") {
                return fmt.Errorf("%w: HTML delimiter in text-only %s/%s",ErrInvalidCatalog,lang,key)
            }
            left, right := placeholders(text), placeholders(translated)
            if strings.Join(left,",") != strings.Join(right,",") {
                return fmt.Errorf("%w: placeholder mismatch %s/%s",ErrInvalidCatalog,lang,key)
            }
        }
    }
    return nil
}

func placeholders(source string) []string {
    seen := map[string]bool{}
    for _, parts := range templateSlot.FindAllStringSubmatch(source, -1) { seen[parts[1]] = true }
    result := make([]string,0,len(seen))
    for key := range seen { result=append(result,key) }
    sort.Strings(result)
    return result
}

// Catalog returns a defensive copy for the UI/automation/agent adaptation
// layer. The same immutable catalog keys, not localized prose, remain the
// stable diagnostic and durable-effect authority.
func Catalog(locale Locale) (map[string]string,error) {
    loadOnce.Do(load)
    if loadError != nil { return nil,loadError }
    lang := locale
    if lang != English && lang != German && lang != Thai { lang=English }
    out := make(map[string]string,len(localized[lang]))
    for k,v := range localized[lang] { out[k]=v }
    return out,nil
}

// Message performs single-pass, plain-text placeholder interpolation.
// Callers MUST insert the returned string as textContent (or an escaped HTML
// text node), never innerHTML. Replacement values are never recursively
// interpreted as templates or as authorization policy.
func Message(locale Locale, key string, values map[string]string) (string,error) {
    dict,err:=Catalog(locale)
    if err!=nil{return "",err}
    candidate,ok:=dict[key]
    if !ok {return "",fmt.Errorf("%w: unknown message key %q",ErrInvalidCatalog,key)}
    missing:=false
    text:=templateSlot.ReplaceAllStringFunc(candidate,func(found string)string{
        name:=found[1:len(found)-1]
        replacement,ok:=values[name]
        if !ok {missing=true;return found}
        return replacement
    })
    if missing {return "",fmt.Errorf("%w: missing placeholder for %s",ErrInvalidCatalog,key)}
    return text,nil
}
