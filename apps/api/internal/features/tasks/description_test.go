package tasks

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeDescriptionKeepsOnlyTheAllowlist(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "script and event handlers are dropped, text kept",
			in:   `<p>Hi</p><script>alert(1)</script><a href="javascript:x" onclick="y">l</a>`,
			want: `<p>Hi</p>l`,
		},
		{
			name: "style, class, data attributes and img are dropped",
			in:   `<p style="color:red" class="x" data-a="1">t</p><img src="x" onerror="alert(1)">`,
			want: `<p>t</p>`,
		},
		{
			name: "block elements outside the allowlist unwrap to their text",
			in:   `<div><h1>head</h1><blockquote>q</blockquote></div>`,
			want: `headq`,
		},
		{
			name: "nested lists and inline marks pass through unchanged",
			in:   `<ul><li>one<ul><li>nested</li></ul></li><li><strong>b</strong> <em>i</em> <u>u</u> <s>s</s></li></ul>`,
			want: `<ul><li>one<ul><li>nested</li></ul></li><li><strong>b</strong> <em>i</em> <u>u</u> <s>s</s></li></ul>`,
		},
		{
			name: "comments are dropped and outer whitespace trimmed",
			in:   "  <p>x</p><!-- c --><p>y</p>  ",
			want: `<p>x</p><p>y</p>`,
		},
		{
			name: "https link gains rel and target",
			in:   `<a href="https://example.com/x?a=1&b=2">site</a>`,
			want: `<a href="https://example.com/x?a=1&amp;b=2" rel="nofollow noreferrer noopener" target="_blank">site</a>`,
		},
		{
			name: "mailto link gains rel but no target",
			in:   `<a href="mailto:a@b.co">mail</a>`,
			want: `<a href="mailto:a@b.co" rel="nofollow noreferrer">mail</a>`,
		},
		{
			name: "ftp and relative links lose the whole anchor, text stays",
			in:   `<p><a href="ftp://x">ftp</a> and <a href="/rel">rel</a></p>`,
			want: `<p>ftp and rel</p>`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeDescription(tc.in)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestNormalizeDescriptionWrapsPlainText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "line breaks become br inside one paragraph", in: "a\nb", want: `<p>a<br>b</p>`},
		{name: "windows line ending is one br", in: "a\r\nb", want: `<p>a<br>b</p>`},
		{name: "a leading < that is not a tag is still plain text", in: "<3 me\nline two", want: `<p>&lt;3 me<br>line two</p>`},
		{name: "markup in plain text is escaped, not interpreted", in: `plain <b>bold</b> & more`, want: `<p>plain &lt;b&gt;bold&lt;/b&gt; &amp; more</p>`},
		{name: "surrounding whitespace is trimmed before wrapping", in: "  hello  \n", want: `<p>hello</p>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeDescription(tc.in)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestNormalizeDescriptionCollapsesEmptyDocuments(t *testing.T) {
	for _, in := range []string{"", "   ", "\n", `<p></p>`, `<p><br></p>`, `<ul><li></li></ul>`, `<p>&nbsp;</p>`, `<script>x</script>`} {
		t.Run(in, func(t *testing.T) {
			got, err := normalizeDescription(in)
			require.NoError(t, err)
			require.Equal(t, "", got, "a document with no text must be stored as the empty string")
		})
	}
}

func TestNormalizeDescriptionLimitsTextNotMarkup(t *testing.T) {
	// Vietnamese letters are multi-byte: the limit must count runes.
	atLimit := strings.Repeat("ă", maxDescriptionRunes)
	got, err := normalizeDescription(atLimit)
	require.NoError(t, err)
	require.Equal(t, "<p>"+atLimit+"</p>", got)

	_, err = normalizeDescription(atLimit + "ă")
	require.ErrorIs(t, err, errDescriptionTooLong)

	// Markup does not count: 4000 letters wrapped in many tags is fine.
	items := strings.Repeat("<li><strong>"+strings.Repeat("x", 100)+"</strong></li>", 40)
	_, err = normalizeDescription("<ul>" + items + "</ul>")
	require.NoError(t, err)

	// An entity is one character of text, not five.
	amps := strings.Repeat("&amp;", maxDescriptionRunes)
	_, err = normalizeDescription("<p>" + amps + "</p>")
	require.NoError(t, err)
	_, err = normalizeDescription("<p>" + amps + "&amp;</p>")
	require.ErrorIs(t, err, errDescriptionTooLong)
}
