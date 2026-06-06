package core

import "testing"

// antibot_test.go verifies isAntiBotPage's marker coverage against a
// corpus of real-world anti-bot challenge page snippets. The
// markers live in renderer.go; this file pins their behavior so
// future changes to the marker set are caught by CI.
//
// We test against excerpts of actual challenge pages (not full
// pages) because (1) the check is a substring search over the
// first KB or so of body HTML and (2) real challenge pages change
// over time but their identifying phrases are stable.

func TestIsAntiBotPage_PositiveMarkers(t *testing.T) {
	cases := []struct {
		name string
		html string
	}{
		{
			name: "reddit_cloudflare_block",
			html: `<html><body><div>You've been blocked by network security.</div><span>File a ticket</span></body></html>`,
		},
		{
			name: "cloudflare_just_a_moment",
			html: `<html><body><div id="cf-wrapper">Just a moment...</div></body></html>`,
		},
		{
			name: "cloudflare_attention_required",
			html: `<html><body><h1>Attention Required! | Cloudflare</h1></body></html>`,
		},
		{
			name: "cloudflare_cf_browser_verification",
			html: `<html><body><form id="cf-browser-verification-form"></form></body></html>`,
		},
		{
			name: "cloudflare_cf_challenge",
			html: `<html><body><form id="cf-challenge-form"></form></body></html>`,
		},
		{
			name: "cloudflare_checking_browser",
			html: `<html><body><p>Checking your browser before accessing example.com.</p></body></html>`,
		},
		{
			name: "cloudflare_security_service",
			html: `<html><body><p>This site is using a security service to protect itself from online attacks.</p></body></html>`,
		},
		{
			name: "cloudflare_ray_id",
			html: `<html><body><p>Ray ID: 8a1b2c3d4e5f6789</p></body></html>`,
		},
		{
			name: "cloudflare_generic",
			html: `<html><body><p>Cloudflare</p></body></html>`,
		},
		{
			name: "request_blocked_generic",
			html: `<html><body><h1>Your request has been blocked.</h1></body></html>`,
		},
		{
			name: "captcha",
			html: `<html><body><div class="g-recaptcha" data-sitekey="..."></div></body></html>`,
		},
		{
			name: "access_denied",
			html: `<html><body><h1>Access Denied</h1></body></html>`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !isAntiBotPage(c.html) {
				t.Errorf("expected isAntiBotPage(%q) to be true, got false", c.name)
			}
		})
	}
}

func TestIsAntiBotPage_NegativeCases(t *testing.T) {
	// Real content pages and edge cases that must NOT trip the
	// detection. False positives here would short-circuit real
	// fetches and cause users to lose their content.
	cases := []struct {
		name string
		html string
	}{
		{
			name: "ordinary_blog",
			html: `<html><body><article><h1>How to Deploy a Web App</h1><p>This article covers the steps to deploy a web app to production.</p></article></body></html>`,
		},
		{
			name: "example_com",
			html: `<html><body><h1>Example Domain</h1><p>This domain is for use in documentation examples without needing permission.</p></body></html>`,
		},
		{
			name: "github_readme",
			html: `<html><body><main><h1>my-project</h1><p>A library for parsing CSV files in Go.</p></main></body></html>`,
		},
		{
			name: "case_insensitive_match",
			html: `<html><body><p>JOURNALISM IS BLOCKED BY CENSORSHIP</p></body></html>`,
		},
		{
			name: "forbidden_book_title",
			html: `<html><body><h1>Forbidden Love: A Romance Novel Review</h1><p>This is a book review of the romance novel Forbidden Love.</p></body></html>`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if isAntiBotPage(c.html) {
				t.Errorf("expected isAntiBotPage(%q) to be false, got true", c.name)
			}
		})
	}
}
