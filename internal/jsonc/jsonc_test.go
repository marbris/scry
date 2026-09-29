package jsonc

import "testing"

func TestStripDropsCommentsButNotStrings(t *testing.T) {
	src := `{
  // the theme
  "theme": "nord", // trailing
  "url": "https://example.com/a//b",
  "quote": "say \"//hi\""
}`
	want := "{\n  \n  \"theme\": \"nord\", \n" +
		"  \"url\": \"https://example.com/a//b\",\n" +
		"  \"quote\": \"say \\\"//hi\\\"\"\n}"
	if got := string(Strip([]byte(src))); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestACommentedOutFileIsEmpty(t *testing.T) {
	if !Empty([]byte("// {\n//   \"theme\": \"nord\"\n// }\n")) {
		t.Error("a file of comments isn't empty")
	}
	if Empty([]byte("{}")) {
		t.Error("{} is empty")
	}
}
