package utils

import "testing"

func TestPageNeedsJavaScript_NextShell(t *testing.T) {
	html := `<!doctype html><html><head><title>x</title></head><body><div id="__next"></div></body></html>`
	if !PageNeedsJavaScript(html) {
		t.Errorf("expected Next.js shell to be detected as needing JS")
	}
}

func TestPageNeedsJavaScript_StaticArticle(t *testing.T) {
	html := `<!doctype html><html><head><title>Article</title></head><body>` +
		`<h1>Title</h1><p>` + repeat("lorem ipsum dolor sit amet, ", 50) + `</p></body></html>`
	if PageNeedsJavaScript(html) {
		t.Errorf("static article with substantial body text should not be flagged as needing JS")
	}
}

func TestPageNeedsJavaScript_LargeHTMLShortCircuit(t *testing.T) {
	html := make([]byte, 600000)
	for i := range html {
		html[i] = 'a'
	}
	if PageNeedsJavaScript(string(html)) {
		t.Errorf("HTML over 500000 bytes should short-circuit to false")
	}
}

func TestPageLooksLikeGenericBotWall_True(t *testing.T) {
	html := `<html><body><p>Checking your browser before accessing example.com.</p></body></html>`
	if !PageLooksLikeGenericBotWall(html) {
		t.Errorf("expected bot-wall phrase to be detected")
	}
}

func TestPageLooksLikeGenericBotWall_FalseOnRichContent(t *testing.T) {
	html := `<html><body><p>` + repeat("normal prose, ", 100) + `</p></body></html>`
	if PageLooksLikeGenericBotWall(html) {
		t.Errorf("rich content with >600 visible runes should not be flagged")
	}
}

func TestPageLooksLikeVendorBlock_Cloudflare(t *testing.T) {
	html := `<html><head><script src="/cdn-cgi/challenge-platform/x.js"></script></head><body></body></html>`
	if got := PageLooksLikeVendorBlock(html); got != "cloudflare" {
		t.Errorf("expected cloudflare, got %q", got)
	}
}

func TestPageLooksLikeVendorBlock_AkamaiRef(t *testing.T) {
	html := `<html><body>Reference #18.4e3a.1234.abcd</body></html>`
	if got := PageLooksLikeVendorBlock(html); got != "akamai" {
		t.Errorf("expected akamai, got %q", got)
	}
}

func TestPageLooksLikeVendorBlock_Plain(t *testing.T) {
	html := `<html><body><p>hello world</p></body></html>`
	if got := PageLooksLikeVendorBlock(html); got != "" {
		t.Errorf("plain HTML should yield empty vendor, got %q", got)
	}
}

func TestPageLooksLikeThinHTML(t *testing.T) {
	thin := `<html><body><p>hi</p></body></html>`
	if !PageLooksLikeThinHTML(thin) {
		t.Errorf("short body should be flagged as thin")
	}
	rich := `<html><body><p>` + repeat("word ", 300) + `</p></body></html>`
	if PageLooksLikeThinHTML(rich) {
		t.Errorf("rich body should not be flagged as thin")
	}
}

func TestPageLooksLikeFailedRender_NextError(t *testing.T) {
	html := `<html><body><div id="__next-error-123">oops</div></body></html>`
	reason, ok := PageLooksLikeFailedRender(html)
	if !ok || reason != FailedRenderNextJsClientError {
		t.Errorf("expected Next.js client error, got reason=%q ok=%v", reason, ok)
	}
}

func TestPageLooksLikeFailedRender_EmptyNextRoot(t *testing.T) {
	html := `<html><body><div id="__next">` + repeat(" ", 5) + `</div></body></html>`
	reason, ok := PageLooksLikeFailedRender(html)
	if !ok || reason != FailedRenderEmptyNextRoot {
		t.Errorf("expected empty next root, got reason=%q ok=%v", reason, ok)
	}
}

func TestPageLooksLikeLoadingPlaceholder_LoadingText(t *testing.T) {
	html := `<html><body><div class="spinner"></div><p>Loading...</p></body></html>`
	if !PageLooksLikeLoadingPlaceholder(html) {
		t.Errorf("expected 'Loading...' placeholder to be detected")
	}
}

func TestPageHasBlockInterstitial_CloudflareChallenge(t *testing.T) {
	html := `<html><body><p>Just a moment...</p></body></html>`
	if !PageHasBlockInterstitial(html) {
		t.Errorf("expected 'Just a moment' to be detected as a block")
	}
}

func TestPageHasClientSideCrash_NextErrorShell(t *testing.T) {
	html := `<html><body><div data-nextjs-error="boom"></div></body></html>`
	if !PageHasClientSideCrash(html) {
		t.Errorf("expected Next.js error marker to be detected as a crash")
	}
}

func TestPageHasMinimalContent_TrueOnThin(t *testing.T) {
	if !PageHasMinimalContent(`<html><body>hi</body></html>`) {
		t.Errorf("expected thin content flag")
	}
}

func TestVisibleTextLength_StripsTags(t *testing.T) {
	html := `<html><body><p>hello world</p></body></html>`
	if got := VisibleTextLength(html); got < 5 {
		t.Errorf("expected at least 5 visible runes, got %d", got)
	}
}

func TestVisibleTextFromStrippedHTML(t *testing.T) {
	in := `<p>hello <b>world</b>!</p>`
	got := VisibleTextFromStrippedHTML(in)
	if got != "hello world!" {
		t.Errorf("got %q, want %q", got, "hello world!")
	}
}

func TestExtractBodyTextLength_NoBody(t *testing.T) {
	if got := ExtractBodyTextLength(`<html></html>`); got != 1000 {
		t.Errorf("documents without <body> should short-circuit to 1000, got %d", got)
	}
}

func TestCopyHeaders(t *testing.T) {
	src := map[string]string{"A": "1", "B": "2"}
	dst := CopyHeaders(src)
	dst["A"] = "changed"
	if src["A"] != "1" {
		t.Errorf("CopyHeaders should not alias the source map")
	}

	empty := CopyHeaders(nil)
	if empty == nil {
		t.Errorf("CopyHeaders should return a non-nil empty map for nil input")
	}
}

func TestCountVisibleRunes(t *testing.T) {
	if got := countVisibleRunes("  a b\tc\nd "); got != 4 {
		t.Errorf("expected 4 visible runes, got %d", got)
	}
}

func repeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
