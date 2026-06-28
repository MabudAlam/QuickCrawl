package core

import (
	"context"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// BrandDesignTokens is the result of the in-browser brand design-token
// extractor. HTML is the full rendered HTML of the page (same payload that
// the static extractors in internal/brand consume). Tokens is the raw JSON
// returned by the extractor; it is unmarshalled by
// brand.ExtractDesignTokens on the Go side. Tokens may be empty when the
// browser path failed but HTML was still captured.
type BrandDesignTokens struct {
	HTML   string
	Tokens []byte
}

// brandTokenJS is the JavaScript that runs inside the rendered page to
// extract font + styleguide design tokens. It is intentionally a single
// self-contained IIFE so it can be passed verbatim to chromedp.Evaluate.
//
// The payload shape mirrors the reference services the user provided:
//
//	{
//	  fonts:      { fonts: BrandFont[], fontLinks: {...} },
//	  styleguide: { mode, colors, typography, elementSpacing, shadows,
//	                components, fontLinks }
//	}
//
// Field semantics follow the reference data:
//   - BrandFont.uses            — distinct tag names using the font
//   - BrandFont.fallbacks       — full font-family stack after the primary
//   - BrandFont.percent_*       — share of all elements / all words
//   - BrandStyleguide.colors    — accent / background / text derived from
//     the most-used colors and computed body styles
//   - BrandStyleguide.typography — computed styles for h1–h4 and the
//     first <p>
//   - BrandStyleguide.components — primary/secondary/link button variants
//     and the dominant card surface, each with the actual rendered CSS
//
// The extractor bounds work by capping element iteration and CSS-rule
// parsing so even very large pages complete quickly.
const brandTokenJS = `
(function () {
  'use strict';
  if (typeof window === 'undefined' || !document || !document.documentElement) {
    return JSON.stringify({ fonts: null, styleguide: null });
  }

  var MAX_ELEMENTS = 4000;
  var MAX_SHEETS = 32;
  var MAX_RULES_PER_SHEET = 4000;

  // ---------- helpers ----------
  function rgbToHex(rgb) {
    if (!rgb) return '';
    var m = /rgba?\((\d+)\s*,\s*(\d+)\s*,\s*(\d+)/.exec(rgb);
    if (!m) return rgb;
    function h(n) { var v = parseInt(n, 10).toString(16); return v.length === 1 ? '0' + v : v; }
    return '#' + h(m[1]) + h(m[2]) + h(m[3]);
  }
// Parse CSS color() / lab() / oklab() / oklch() / color() (modern) into RGB.
// We use a temporary DOM node + getComputedStyle to let the browser resolve
// the value for us — cheaper and more accurate than reimplementing the
// full CSS color spec. Two round-trips: Chrome sometimes returns lab/oklab
// back from getComputedStyle instead of normalizing to rgb; the second
// round-trip (set the computed color, read again) forces normalization.
  function resolveAnyColor(value) {
    if (!value) return '';
    if (/^#[0-9a-f]{3,8}$/i.test(value) || /^rgba?\(/.test(value) || /^hsla?\(/.test(value) || value === 'transparent') return value;
    function trip(v) {
      var probe = document.createElement('span');
      probe.style.color = v;
      if (!probe.style.color) return '';
      document.body.appendChild(probe);
      var computed = window.getComputedStyle(probe).color;
      document.body.removeChild(probe);
      return computed || '';
    }
    var first = trip(value);
    if (!first) return value;
    // If the browser returned the same modern syntax we passed in, do a
    // second round-trip to coax it into rgb().
    if (/^(lab|oklab|oklch|color)\(/i.test(first)) {
      var second = trip(first);
      if (second) first = second;
    }
    // Final fallback: if still in modern syntax, approximate via the
    // CSS Color 4 luminance. We can't accurately convert without a
    // library, so we leave it for the caller to decide.
    return first || value;
  }
  function parseFontFamily(value) {
    if (!value) return [];
    return value.split(',').map(function (s) {
      s = s.trim();
      if (s.charAt(0) === '"' || s.charAt(0) === "'") s = s.slice(1, -1);
      return s.trim();
    }).filter(Boolean);
  }
  function clampNum(n) { n = parseFloat(n); return isNaN(n) ? 0 : n; }
  function px(s) {
    if (!s) return '0px';
    if (/px$/.test(s)) return s;
    var n = parseFloat(s);
    if (isNaN(n)) return s;
    return n + 'px';
  }
  function normColor(c) {
    if (!c || c === 'transparent' || c === 'rgba(0, 0, 0, 0)') return '';
    var resolved = resolveAnyColor(c);
    var hex = rgbToHex(resolved);
    if (hex.charAt(0) !== '#') return c;
    if (hex.length === 4) hex = '#' + hex[1] + hex[1] + hex[2] + hex[2] + hex[3] + hex[3];
    return hex.toLowerCase();
  }

  // ---------- collect preload + @font-face ----------
  // Phase 1: walk every @font-face rule to build:
  //   urlToFamily     { full URL  -> family }
  //   basenameToFamily{ basename  -> family }   (for matching across
  //                                            /_next/static/media/X.woff2
  //                                            vs /media/X.woff2)
  //   registeredURLs  Set of full URLs we've already added to fontLinks
  // so the preload loop can resolve hashed Next.js filenames back to their
  // real family ("Inter", "hostGrotesk", …) without duplicating entries.
  var urlToFamily = {};
  var basenameToFamily = {};
  var registeredURLs = {};
  var fontLinks = {};
  function basenameOf(url) {
    var q = url.split('?')[0].split('#')[0];
    var parts = q.split('/');
    return parts[parts.length - 1] || '';
  }
  function registerFontLink(displayName, weight, url) {
    if (!displayName || !url) return;
    // Skip if this exact URL was already registered (avoids the same file
    // appearing under both its real family AND its hashed basename).
    if (registeredURLs[url]) return;
    registeredURLs[url] = displayName;
    var key = displayName;
    if (!fontLinks[key]) {
      var isGoogle = /fonts\.googleapis\.com|fonts\.gstatic\.com/.test(url);
      var isAdobe = /use\.typekit\.net/.test(url);
      fontLinks[key] = {
        type: isGoogle ? 'google' : (isAdobe ? 'adobe' : 'custom'),
        files: {},
        displayName: displayName
      };
    }
    // Variable fonts ship multiple weights inside a single file (e.g.
    // "100 900"). We collapse them under a "variable" key so the same
    // file isn't listed once per weight.
    var w = String(weight || 400);
    var wKey = (w.indexOf(' ') !== -1) ? 'variable' : w;
    fontLinks[key].files[wKey] = url;
  }

  function registerFontFace(rule) {
    var style = rule.style;
    var family = style.getPropertyValue('font-family');
    if (!family) return;
    family = family.replace(/^["']|["']$/g, '').trim();
    var weight = style.getPropertyValue('font-weight') || '400';
    var src = style.getPropertyValue('src');
    if (!src) return;
    var urls = [];
    var re = /url\(["']?([^"')]+)["']?\)/g;
    var mm;
    while ((mm = re.exec(src)) !== null) urls.push(mm[1]);
    if (!urls.length) return;
    for (var u = 0; u < urls.length; u++) {
      var url = urls[u];
      try { url = new URL(url, location.href).href; } catch (e) {}
      urlToFamily[url] = family;
      basenameToFamily[basenameOf(url)] = family;
      registerFontLink(family, weight, url);
    }
  }

  function walkStyleSheet(sheet) {
    try {
      var rules = sheet.cssRules || sheet.rules;
      if (!rules) return;
      var lim = Math.min(rules.length, MAX_RULES_PER_SHEET);
      for (var r = 0; r < lim; r++) {
        var rule = rules[r];
        if (!rule) continue;
        if (rule.type === CSSRule.FONT_FACE_RULE) {
          registerFontFace(rule);
        } else if (rule.type === CSSRule.IMPORT_RULE) {
          var s2 = rule.styleSheet;
          if (s2) walkStyleSheet(s2);
        }
      }
    } catch (e) { /* cross-origin, ignore */ }
  }

  var styleSheets = document.styleSheets || [];
  var sheetLim = Math.min(styleSheets.length, MAX_SHEETS);
  for (var s = 0; s < sheetLim; s++) walkStyleSheet(styleSheets[s]);

  // <link rel="preload" as="font" href="..."> with optional type/font-weight.
  // Three-tier lookup:
  //   1. Exact full URL match in urlToFamily  (path-equivalent @font-face)
  //   2. Basename match                       (path differs: /media vs /_next)
  //   3. Filename fallback                    (CORS-blocked stylesheets)
  // The first two prevent the same file from appearing under both its
  // real family and its hashed basename.
  var preloadLinks = document.querySelectorAll('link[rel="preload"][as="font"], link[rel="preload"][as="font"][type]');
  for (var pl = 0; pl < preloadLinks.length; pl++) {
    var link = preloadLinks[pl];
    var href = link.getAttribute('href');
    if (!href) continue;
    try { href = new URL(href, location.href).href; } catch (e) {}
    var family = urlToFamily[href] || basenameToFamily[basenameOf(href)];
    if (!family) {
      family = basenameOf(href).split('.')[0];
    }
    if (family && !registeredURLs[href]) {
      var w2 = link.getAttribute('font-weight') || link.getAttribute('data-weight') || 400;
      registerFontLink(family, w2, href);
    }
  }

  // ---------- font usage ----------
  var fontStats = {};
  function visit(el) {
    if (!el || el.nodeType !== 1) return;
    var tag = el.tagName ? el.tagName.toLowerCase() : '';
    if (tag === 'script' || tag === 'style' || tag === 'noscript' || tag === 'meta') return;
    var cs = window.getComputedStyle(el);
    var families = parseFontFamily(cs.fontFamily);
    if (!families.length) return;
    var primary = families[0];
    var stat = fontStats[primary] || (fontStats[primary] = { uses: {}, numElements: 0, numWords: 0, fallbacks: families.slice() });
    stat.uses[tag] = true;
    stat.numElements += 1;
    var text = (el.innerText || '').trim();
    if (text) {
      var wc = text.split(/\s+/).filter(Boolean).length;
      if (wc > 0) stat.numWords += wc;
    }
  }

  var all = document.querySelectorAll('*');
  var lim = Math.min(all.length, MAX_ELEMENTS);
  for (var e = 0; e < lim; e++) visit(all[e]);

  var totalElements = 0;
  var totalWords = 0;
  var fontArr = [];
  Object.keys(fontStats).forEach(function (name) {
    var s = fontStats[name];
    totalElements += s.numElements;
    totalWords += s.numWords;
  });
  Object.keys(fontStats).forEach(function (name) {
    var s = fontStats[name];
    var uses = Object.keys(s.uses).sort();
    fontArr.push({
      font: name,
      uses: uses,
      fallbacks: s.fallbacks.slice(1),
      num_elements: s.numElements,
      num_words: s.numWords,
      percent_elements: totalElements ? Math.round((s.numElements / totalElements) * 100) : 0,
      percent_words: totalWords ? Math.round((s.numWords / totalWords) * 100) : 0
    });
  });
  fontArr.sort(function (a, b) { return b.percent_elements - a.percent_elements; });

  // ---------- styleguide ----------
  function getStyle(sel) {
    var el = document.querySelector(sel);
    if (!el) return null;
    return window.getComputedStyle(el);
  }
  function pickButton() {
    var btns = document.querySelectorAll('button, a[role="button"], input[type="button"], input[type="submit"]');
    if (!btns.length) return null;
    // Score each button: larger + visible text + solid bg = better primary
    // candidate. We sort each bucket by score so [0] is the most representative.
    var candidates = [];
    for (var i = 0; i < btns.length; i++) {
      var b = btns[i];
      var rect = b.getBoundingClientRect();
      if (rect.width === 0 || rect.height === 0) continue;
      var cs = window.getComputedStyle(b);
      var bg = cs.backgroundColor;
      var text = (b.innerText || '').trim();
      if (!text) continue;
      var score = rect.width * rect.height + text.length * 50;
      candidates.push({ el: b, cs: cs, rect: rect, text: text, bg: bg, score: score });
    }
    candidates.sort(function (a, b) { return b.score - a.score; });
    var buckets = { primary: [], secondary: [], link: [] };
    for (var j = 0; j < candidates.length; j++) {
      var c = candidates[j];
      var isLink = c.cs.textDecorationLine.indexOf('underline') !== -1 ||
                   (c.bg === 'rgba(0, 0, 0, 0)' && c.cs.borderTopWidth === '0px');
      if (isLink) buckets.link.push(c);
      else if (c.bg && c.bg !== 'rgba(0, 0, 0, 0)') buckets.primary.push(c);
      else buckets.secondary.push(c);
    }
    return buckets;
  }
  function cardStyle() {
    // Try semantic and class-based first, then fall back to a generic
    // container with border + radius + padding.
    var sels = [
      '[class*="card" i]', '[class*="Card"]',
      'article', 'aside',
      '[role="region"]', '[class*="panel" i]', '[class*="surface" i]'
    ];
    for (var s = 0; s < sels.length; s++) {
      var hit = document.querySelector(sels[s]);
      if (hit) {
        var cs0 = window.getComputedStyle(hit);
        if (cs0.backgroundColor !== 'rgba(0, 0, 0, 0)') {
          return { el: hit, cs: cs0 };
        }
      }
    }
    // Fallback: a div with visible background, border, and radius.
    var all2 = document.querySelectorAll('div, article, section');
    for (var i = 0; i < Math.min(all2.length, 1500); i++) {
      var cs = window.getComputedStyle(all2[i]);
      if (cs.backgroundColor !== 'rgba(0, 0, 0, 0)' &&
          parseFloat(cs.borderTopLeftRadius) >= 4 &&
          parseFloat(cs.paddingLeft) >= 8 &&
          parseFloat(cs.paddingTop) >= 8) {
        return { el: all2[i], cs: cs };
      }
    }
    return null;
  }
  function styleToText(cs) {
    if (!cs) return null;
    var ff = parseFontFamily(cs.fontFamily);
    return {
      fontFamily: ff[0] || '',
      fontFallbacks: ff,
      fontSize: px(cs.fontSize),
      fontWeight: clampNum(cs.fontWeight) || 400,
      lineHeight: px(cs.lineHeight),
      letterSpacing: px(cs.letterSpacing)
    };
  }
  function colorBox(cs, el) {
    // Read min-width / min-height from the inline + computed values. The
    // computed value resolves "auto" / numeric defaults; for CSS literals
    // (e.g. "172px") we also peek at getPropertyValue on the resolved style.
    var minW = cs.minWidth;
    var minH = cs.minHeight;
    if (el && el.style) {
      var iw = el.style.minWidth;
      var ih = el.style.minHeight;
      if (iw) minW = iw;
      if (ih) minH = ih;
    }
    return {
      backgroundColor: normColor(cs.backgroundColor) || 'transparent',
      color: normColor(cs.color) || '#000000',
      borderColor: normColor(cs.borderColor) || 'transparent',
      borderRadius: px(cs.borderTopLeftRadius),
      borderWidth: px(cs.borderTopWidth),
      borderStyle: cs.borderTopStyle || 'none',
      padding: px(cs.paddingTop) + ' ' + px(cs.paddingRight) + ' ' + px(cs.paddingBottom) + ' ' + px(cs.paddingLeft),
      fontSize: px(cs.fontSize),
      fontWeight: clampNum(cs.fontWeight) || 400,
      textDecoration: cs.textDecorationLine || 'none',
      boxShadow: cs.boxShadow === 'none' ? 'none' : cs.boxShadow,
      fontFamily: parseFontFamily(cs.fontFamily)[0] || '',
      fontFallbacks: parseFontFamily(cs.fontFamily),
      minWidth: minW,
      minHeight: minH
    };
  }
  function buildCSS(parts) {
    var lines = [];
    Object.keys(parts).forEach(function (k) {
      if (k === 'fontFallbacks') return;
      var v = parts[k];
      if (v === undefined || v === null || v === '') return;
      var key = k.replace(/[A-Z]/g, function (m) { return '-' + m.toLowerCase(); });
      lines.push(key + ': ' + v + ';');
    });
    return lines.join(' ');
  }

  var bodyCS = window.getComputedStyle(document.body);
  var bodyBgRaw = bodyCS.backgroundColor;
  var bodyBg = normColor(bodyBgRaw) || '#ffffff';
  var bodyColor = normColor(bodyCS.color) || '#000000';
  // Mode detection: derive from the resolved background luminance. If the
  // background is genuinely light (luminance > 200), the page is light
  // regardless of accent color. This avoids the previous probe-based
  // heuristic that could flip a white-background page to "dark" when the
  // body inherited a dark wrapper color.
  var isDark = false;
  var bgRgb = resolveAnyColor(bodyBgRaw);
  var bgMatch = /rgba?\((\d+),\s*(\d+),\s*(\d+)/.exec(bgRgb);
  if (bgMatch) {
    var bgLum = (parseInt(bgMatch[1], 10) * 299 +
                 parseInt(bgMatch[2], 10) * 587 +
                 parseInt(bgMatch[3], 10) * 114) / 1000;
    isDark = bgLum < 128;
  }

  // Accent selection is deferred until after we know the primary button.
  var accent = bodyColor;

  var typography = { headings: {}, p: {} };
  // Body: prefer the first real <p> over <body>. <body> often inherits a
  // wrapper font (e.g. the whole site uses hostGrotesk on <body>) that
  // doesn't represent the design system's body face.
  var firstP = document.querySelector('p');
  if (firstP) typography.p = styleToText(window.getComputedStyle(firstP)) || {};
  else typography.p = styleToText(bodyCS) || {};

  // Headings: tag-based first, then a class-based fallback for h3/h4
  // (design systems commonly use class="text-h3" instead of <h4>).
  function headingByClass(needle) {
    var matches = document.querySelectorAll('[class*="' + needle + '" i], [class*="' + needle + '"]');
    for (var i = 0; i < matches.length; i++) {
      var cs = window.getComputedStyle(matches[i]);
      if (parseFloat(cs.fontSize) >= 14) return matches[i];
    }
    return null;
  }
  ['h1', 'h2', 'h3', 'h4'].forEach(function (tag) {
    var el = document.querySelector(tag);
    if (!el && tag !== 'h1') {
      var map = { h2: 'h2', h3: 'h3', h4: 'h4' };
      el = headingByClass(map[tag] || tag);
    }
    if (el) typography.headings[tag] = styleToText(window.getComputedStyle(el));
  });

  var buttons = pickButton();
  var primaryBtn = buttons && buttons.primary[0];
  var secondaryBtn = buttons && buttons.secondary[0];
  var linkBtn = buttons && buttons.link[0];

  // Accent: prefer the primary button's background when it's neither black
  // nor white (i.e. it carries the brand accent). Otherwise fall back to
  // the first link color.
  if (primaryBtn && primaryBtn.cs) {
    var pbg = normColor(primaryBtn.cs.backgroundColor);
    if (pbg && pbg !== '#000000' && pbg !== '#ffffff' && pbg !== 'transparent') accent = pbg;
  }
  if (!accent || accent === bodyColor) {
    var accentEl = document.querySelector('[class*="primary" i], [class*="brand" i], a');
    if (accentEl) accent = normColor(window.getComputedStyle(accentEl).color) || accent;
  }
  function buttonToBox(o) {
    if (!o) return null;
    var b = colorBox(o.cs, o.el);
    var css = buildCSS(b);
    return Object.assign({}, b, { css: css });
  }
  var card = cardStyle();
  var cardBox = card ? {
    backgroundColor: normColor(card.cs.backgroundColor) || '#ffffff',
    borderColor: normColor(card.cs.borderColor) || 'transparent',
    borderRadius: px(card.cs.borderTopLeftRadius),
    borderWidth: px(card.cs.borderTopWidth),
    borderStyle: card.cs.borderTopStyle || 'none',
    padding: px(card.cs.paddingTop) + ' ' + px(card.cs.paddingRight) + ' ' + px(card.cs.paddingBottom) + ' ' + px(card.cs.paddingLeft),
    boxShadow: card.cs.boxShadow === 'none' ? 'none' : card.cs.boxShadow,
    textColor: normColor(card.cs.color) || bodyColor,
    css: buildCSS({
      backgroundColor: normColor(card.cs.backgroundColor) || '#ffffff',
      color: normColor(card.cs.color) || bodyColor,
      border: px(card.cs.borderTopWidth) + ' ' + (card.cs.borderTopStyle || 'none') + ' ' + (normColor(card.cs.borderColor) || 'transparent'),
      'border-radius': px(card.cs.borderTopLeftRadius),
      padding: px(card.cs.paddingTop) + ' ' + px(card.cs.paddingRight) + ' ' + px(card.cs.paddingBottom) + ' ' + px(card.cs.paddingLeft),
      'box-shadow': card.cs.boxShadow === 'none' ? 'none' : card.cs.boxShadow
    })
  } : null;

  function gatherSpacing() {
    var buckets = { xs: [], sm: [], md: [], lg: [], xl: [] };
    var all3 = document.querySelectorAll('section, article, main, div');
    var cap = Math.min(all3.length, 800);
    for (var i = 0; i < cap; i++) {
      var cs = window.getComputedStyle(all3[i]);
      var p = parseFloat(cs.paddingTop);
      if (p > 0) {
        if (p <= 8) buckets.xs.push(p);
        else if (p <= 16) buckets.sm.push(p);
        else if (p <= 32) buckets.md.push(p);
        else if (p <= 64) buckets.lg.push(p);
        else buckets.xl.push(p);
      }
    }
    function median(arr) {
      if (!arr.length) return 0;
      var s = arr.slice().sort(function (a, b) { return a - b; });
      return s[Math.floor(s.length / 2)];
    }
    return {
      xs: px(median(buckets.xs) || 4),
      sm: px(median(buckets.sm) || 12),
      md: px(median(buckets.md) || 24),
      lg: px(median(buckets.lg) || 48),
      xl: px(median(buckets.xl) || 96)
    };
  }
  function gatherShadows() {
    // Bucket every observed box-shadow by its primary blur radius so we
    // can map them to semantic sizes (sm / md / lg / xl / inner) the way
    // Tailwind / Material do:
    //   sm: blur ≤ 8px     md: ≤ 16px     lg: ≤ 32px     xl: > 32px
    var buckets = { sm: [], md: [], lg: [], xl: [], inner: [] };
    var seen = {}; // dedupe per bucket
    var all4 = document.querySelectorAll('*');
    var cap = Math.min(all4.length, 2000);
    for (var i = 0; i < cap; i++) {
      var cs = window.getComputedStyle(all4[i]);
      var sh = cs.boxShadow;
      if (!sh || sh === 'none') continue;
      // Extract the largest blur radius from the shadow string.
      var maxBlur = 0;
      var re = /(-?\d+(?:\.\d+)?)px/g;
      var m;
      var nums = [];
      while ((m = re.exec(sh)) !== null) nums.push(parseFloat(m[1]));
      // CSS box-shadow syntax: <offset-x> <offset-y> <blur> <spread> <color>
      // Multiple shadows separated by commas. We scan each comma-group for
      // its largest blur (3rd numeric in the group, after x/y).
      var groups = sh.split(/,(?![^()]*\))/);
      for (var g = 0; g < groups.length; g++) {
        var grpNums = [];
        var re2 = /(-?\d+(?:\.\d+)?)px/g;
        var mm;
        while ((mm = re2.exec(groups[g])) !== null) grpNums.push(parseFloat(mm[1]));
        if (grpNums.length >= 3) {
          var blur = grpNums[2];
          if (blur > maxBlur) maxBlur = blur;
        }
      }
      var isInset = sh.indexOf('inset') !== -1;
      var key = isInset ? 'inner' : (maxBlur <= 8 ? 'sm' : maxBlur <= 16 ? 'md' : maxBlur <= 32 ? 'lg' : 'xl');
      if (!seen[key + sh]) {
        seen[key + sh] = true;
        buckets[key].push(sh);
      }
    }
    function pick(arr) { return arr.length ? arr[0] : 'none'; }
    return {
      sm: pick(buckets.sm),
      md: pick(buckets.md),
      lg: pick(buckets.lg),
      xl: pick(buckets.xl),
      inner: pick(buckets.inner)
    };
  }

  var styleguide = {
    mode: isDark ? 'dark' : 'light',
    colors: { accent: accent, background: bodyBg, text: bodyColor },
    typography: typography,
    elementSpacing: gatherSpacing(),
    shadows: gatherShadows(),
    components: {
      button: {
        primary: buttonToBox(primaryBtn) || {},
        secondary: buttonToBox(secondaryBtn) || {},
        link: buttonToBox(linkBtn) || {}
      },
      card: cardBox || {}
    },
    fontLinks: fontLinks
  };

  return JSON.stringify({ fonts: { fonts: fontArr, fontLinks: fontLinks }, styleguide: styleguide });
})();
`

// fetchBrandDesignTokens opens the URL in a real browser, waits for the
// page to be ready, then evaluates the brand-token JS and returns both
// the rendered HTML and the raw JSON token payload.
//
// If browser is not configured, returns an error so callers can fall back
// to HTTP-only HTML extraction.
func (renderer *Renderer) fetchBrandDesignTokens(ctx context.Context, rawURL string) (*BrandDesignTokens, error) {
	if renderer.allocCtx == nil {
		return nil, ErrBrowserNotAvailable.New("no browser WS URL configured")
	}

	host := extractHost(rawURL)
	release := renderer.pool.Acquire(host)
	defer release()

	allocCtx := renderer.allocCtx

	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()

	runCtx, cancel := context.WithTimeout(browserCtx, renderer.cfg.PageTimeout)
	defer cancel()

	var fullHTML, tokensJSON string

	// Capture document.documentElement.outerHTML — the complete rendered DOM.
	// We do this in a single Evaluate so we only do one round-trip for HTML.
	const captureHTMLJS = `document.documentElement.outerHTML`

	// Use a sub-action to compose: navigate, wait for SPA, then capture both.
	err := chromedp.Run(runCtx,
		chromedp.Navigate(rawURL),
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := WaitForSPAReady(ctx, SPAReadinessOptions{
				URL:          rawURL,
				Timeout:      15 * time.Second,
				PollInterval: 200 * time.Millisecond,
			})
			return err
		}),
		chromedp.Evaluate(captureHTMLJS, &fullHTML),
		chromedp.Evaluate(brandTokenJS, &tokensJSON),
	)

	if err != nil && fullHTML == "" {
		return nil, err
	}

	return &BrandDesignTokens{
		HTML:   strings.TrimSpace(fullHTML),
		Tokens: []byte(strings.TrimSpace(tokensJSON)),
	}, nil
}