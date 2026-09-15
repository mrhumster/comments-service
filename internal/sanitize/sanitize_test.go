package sanitize

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBodyPlainText(t *testing.T) {
	require.Equal(t, "hello world", Body("  hello world  "))
}

func TestBodyMarkdownSurvives(t *testing.T) {
	in := "**bold** and `code`\n\n- item\n\n[link](https://example.com)"
	out := Body(in)
	require.Contains(t, out, "**bold**")
	require.Contains(t, out, "`code`")
	require.Contains(t, out, "- item")
	require.Contains(t, out, "[link](https://example.com)")
}

func TestBodyStripsScript(t *testing.T) {
	out := Body("hello <script>alert(1)</script> world")
	require.NotContains(t, out, "<script>")
	require.Contains(t, out, "hello")
	require.Contains(t, out, "world")
}

func TestBodyStripsEventHandler(t *testing.T) {
	out := Body(`<a href="https://x.com" onclick="steal()">click</a>`)
	require.NotContains(t, out, "onclick")
	require.Contains(t, out, "click")
}

func TestBodyStripsJavaScriptURL(t *testing.T) {
	out := Body(`<a href="javascript:alert(1)">x</a>`)
	require.NotContains(t, out, "javascript:")
}

func TestBodyStripsImgHandlers(t *testing.T) {
	out := Body(`<img src="x" onerror="alert(1)">`)
	require.NotContains(t, out, "onerror")
}

func TestBodyTruncates(t *testing.T) {
	in := strings.Repeat("a", MaxBodyLength+100)
	require.Len(t, Body(in), MaxBodyLength)
}

func TestBodyEmpty(t *testing.T) {
	require.Equal(t, "", Body(""))
	require.Equal(t, "", Body("   "))
	require.Equal(t, "", Body("<script>alert(1)</script>"))
}

func TestBodyEmojiPreserved(t *testing.T) {
	out := Body("great video \u2764\uFE0F \U0001F389")
	require.Contains(t, out, "\U0001F389")
}

func TestSnippet(t *testing.T) {
	require.Equal(t, "abcdef", Snippet("abcdef", 10))
	require.Equal(t, "abc", Snippet("abcdef", 3))
	require.Equal(t, "", Snippet("", 3))
}
