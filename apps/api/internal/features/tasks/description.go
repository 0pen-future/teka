package tasks

import (
	"errors"
	"html"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/microcosm-cc/bluemonday"
)

// maxDescriptionRunes caps the text a description may carry once its markup
// is stripped, so a client cannot hide an oversized payload behind tags.
const maxDescriptionRunes = 4000

// errDescriptionTooLong is the feature-level sentinel for a description
// whose plain text exceeds maxDescriptionRunes. It stays out of pkg/kanban:
// the core treats a description as an opaque string and knows nothing about
// HTML or its text length.
var errDescriptionTooLong = errors.New("task description exceeds the text limit")

// descriptionPolicy is the single allowlist for task descriptions and the
// trust boundary for them: whatever a client sends, only this subset is
// stored. Links keep href for http, https and mailto only, always carry
// rel="nofollow noreferrer" and, when fully qualified, also gain noopener
// and open in a new tab. Every other element and attribute (style, class,
// on*, data: or javascript: URLs) is dropped while its text content is
// kept. The rel/target attributes the policy adds make the stored markup
// grow past the raw input (a link-only 20000-character body sanitizes to
// roughly 52 KB); the DTO's binding cap bounds the raw input and, through
// it, that growth, so no second cap is applied to the stored form.
var descriptionPolicy = func() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("p", "br", "strong", "em", "u", "s", "ul", "ol", "li")
	p.AllowAttrs("href").OnElements("a")
	p.AllowURLSchemes("http", "https", "mailto")
	p.RequireParseableURLs(true)
	p.RequireNoFollowOnLinks(true)
	p.RequireNoReferrerOnLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)
	return p
}()

// textOnlyPolicy strips every tag; it is how the text of a description is
// measured and how an all-markup document is recognised as empty.
var textOnlyPolicy = bluemonday.StrictPolicy()

// plainTextWrapper mirrors migration 000023_task_description_html: a
// plain-text description becomes one escaped paragraph with line breaks
// turned into <br>. Listing "\r\n" before "\n" makes the replacer match the
// pair first, so a Windows line ending yields a single <br>.
var plainTextWrapper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	"\r\n", "<br>",
	"\n", "<br>",
)

// markupStart recognises a document that starts with a tag, a closing tag
// or a comment/doctype. A plain text that merely begins with "<" ("<3 you",
// "< 5 minutes") must still be wrapped like any other plain text, or its
// line breaks would be lost and no paragraph would surround it.
var markupStart = regexp.MustCompile(`^<[A-Za-z!/]`)

// normalizeDescription turns whatever a client sent into the stored form: a
// document that does not start with a tag is treated as plain text and wrapped
// as the migration wraps legacy rows, then everything is sanitized to
// descriptionPolicy, an all-markup or whitespace-only result collapses to
// "", and a text longer than maxDescriptionRunes is rejected with
// errDescriptionTooLong. Wrapping before sanitizing means a curl user or an
// old client sending "a\nb" keeps its line break instead of ending up with
// a half-HTML "a\nb" text node.
func normalizeDescription(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	if !markupStart.MatchString(trimmed) {
		trimmed = "<p>" + plainTextWrapper.Replace(trimmed) + "</p>"
	}
	sanitized := strings.TrimSpace(descriptionPolicy.Sanitize(trimmed))
	text := descriptionText(sanitized)
	if strings.TrimSpace(text) == "" {
		return "", nil
	}
	if utf8.RuneCountInString(text) > maxDescriptionRunes {
		return "", errDescriptionTooLong
	}
	return sanitized, nil
}

// descriptionText is the plain text of a sanitized description: tags
// stripped and entities decoded, so "&amp;" counts as one rune, not five.
func descriptionText(sanitized string) string {
	return html.UnescapeString(textOnlyPolicy.Sanitize(sanitized))
}
