package renderer

// stealthInjectionJS masks obvious automation fingerprints before the page runs scripts.
const stealthInjectionJS = `(function() {
  Object.defineProperty(navigator, 'webdriver', { get: () => false });
  if (!window.chrome) {
    window.chrome = { runtime: {}, loadTimes: function(){}, csi: function(){} };
  }
  Object.defineProperty(navigator, 'plugins', {
    get: () => {
      const arr = [
        { name: 'Chrome PDF Plugin', filename: 'internal-pdf-viewer' },
        { name: 'Chrome PDF Viewer', filename: 'mhjfbmdgcfjbbpaeojofohoefgiehjai' },
        { name: 'Native Client', filename: 'internal-nacl-plugin' },
      ];
      arr.item = (i) => arr[i];
      arr.namedItem = (n) => arr.find(p => p.name == n);
      arr.refresh = () => {};
      return arr;
    }
  });
  Object.defineProperty(navigator, 'languages', { get: () => ['en-US', 'en'] });
  const _origQuery = window.navigator.permissions.query.bind(window.navigator.permissions);
  window.navigator.permissions.query = (params) =>
    params.name === 'notifications'
      ? Promise.resolve({ state: Notification.permission })
      : _origQuery(params);
  const _origHTMLElement = HTMLIFrameElement.prototype.__lookupGetter__('contentWindow');
  if (_origHTMLElement) {
    Object.defineProperty(HTMLIFrameElement.prototype, 'contentWindow', {
      get: function() {
        const w = _origHTMLElement.call(this);
        if (w && !w.chrome) w.chrome = window.chrome;
        return w;
      }
    });
  }
  const _nativeToString = Function.prototype.toString;
  const _overrides = new Map();
  const _proxy = new Proxy(_nativeToString, {
    apply(target, thisArg, args) {
      const override = _overrides.get(thisArg);
      return override || _nativeToString.call(thisArg);
    }
  });
  Function.prototype.toString = _proxy;
  _overrides.set(Function.prototype.toString, 'function toString() { [native code] }');
  try {
    const _getParameter = WebGLRenderingContext.prototype.getParameter;
    WebGLRenderingContext.prototype.getParameter = function(parameter) {
      if (parameter === 37445) return 'Intel Inc.';
      if (parameter === 37446) return 'Intel Iris OpenGL Engine';
      return _getParameter.call(this, parameter);
    };
    if (typeof WebGL2RenderingContext !== 'undefined') {
      const _getParameter2 = WebGL2RenderingContext.prototype.getParameter;
      WebGL2RenderingContext.prototype.getParameter = function(parameter) {
        if (parameter === 37445) return 'Intel Inc.';
        if (parameter === 37446) return 'Intel Iris OpenGL Engine';
        return _getParameter2.call(this, parameter);
      };
    }
  } catch (_) {}
})()`

// cookieBannerDismissJS clicks common consent banners when they are present.
const cookieBannerDismissJS = `
(() => {
  let clicks = 0;
  const isVisible = (el) => {
    if (!el || !el.getBoundingClientRect) return false;
    const r = el.getBoundingClientRect();
    if (r.width === 0 || r.height === 0) return false;
    const s = window.getComputedStyle(el);
    return s.display !== 'none' && s.visibility !== 'hidden' && s.opacity !== '0';
  };
  const click = (el) => {
    try {
      if (!isVisible(el)) return false;
      el.click();
      clicks++;
      return true;
    } catch (_) { return false; }
  };
  const SELECTORS = [
    '#onetrust-accept-btn-handler', '.ot-accept-all',
    '#CybotCookiebotDialogBodyButtonAccept', '#CybotCookiebotDialogBodyLevelButtonAccept',
    '[data-testid="uc-accept-all-button"]', '[data-cy="uc-accept-all-button"]',
    '.sp_choice_type_11', 'button.message-component[title*="Accept" i]',
    '.qc-cmp2-summary-buttons button[mode="primary"]', '#qc-cmp2-ui button[mode="primary"]',
    '#truste-consent-button', '.cc-btn.cc-allow', '.cc-btn.cc-dismiss',
    'button[data-cmp-action="accept"]', 'button[data-accept-action="all"]',
    'button[aria-label*="Accept all" i]', 'button[aria-label*="Allow all" i]',
    '[id*="accept-cookies" i]', '[class*="accept-cookies" i]:not(input):not(textarea)',
  ];
  const tryRoot = (root) => {
    for (const sel of SELECTORS) {
      try {
        const el = root.querySelector(sel);
        if (el && click(el)) return;
      } catch (_) {}
    }
    try {
      const buttons = root.querySelectorAll('button, [role="button"], input[type="button"], input[type="submit"]');
      const PATTERNS = /^(accept all|allow all|accept cookies|i accept|agree|got it|ok|tümünü kabul et|tout accepter|alle akzeptieren|aceptar todo)$/i;
      for (const b of buttons) {
        const t = (b.innerText || b.value || b.textContent || '').trim();
        if (PATTERNS.test(t) && click(b)) return;
      }
    } catch (_) {}
  };
  tryRoot(document);
  try {
    const all = document.querySelectorAll('*');
    for (const host of all) {
      if (host.shadowRoot) tryRoot(host.shadowRoot);
    }
  } catch (_) {}
  try {
    for (const f of document.querySelectorAll('iframe')) {
      try {
        const doc = f.contentDocument || (f.contentWindow && f.contentWindow.document);
        if (doc) tryRoot(doc);
      } catch (_) {}
    }
  } catch (_) {}
  try {
    if (typeof window.__tcfapi === 'function') {
      window.__tcfapi('ping', 2, (data, ok) => {
        if (ok && data && data.cmpStatus !== 'error') {
          try { window.__tcfapi('addEventListener', 2, () => {}); } catch (_) {}
        }
      });
    }
  } catch (_) {}
  return clicks;
})()
`

// shadowDOMFlattenJS serializes shadow DOM content into plain HTML for extraction.
const shadowDOMFlattenJS = `
(() => {
  const VOID = new Set(['area','base','br','col','embed','hr','img','input','link','meta','param','source','track','wbr']);
  let hasShadow = false;
  try {
    const all = document.querySelectorAll('*');
    for (let i = 0; i < all.length; i++) {
      if (all[i].shadowRoot) { hasShadow = true; break; }
    }
  } catch (_) {}
  if (!hasShadow) return document.documentElement.outerHTML;
  const escAttr = (v) => String(v).replace(/&/g, '&amp;').replace(/"/g, '&quot;');
  const serializeAttrs = (node) => {
    let s = '';
    for (const a of node.attributes || []) s += ' ' + a.name + '="' + escAttr(a.value) + '"';
    return s;
  };
  const serialize = (node) => {
    if (node.nodeType === Node.TEXT_NODE) return node.textContent;
    if (node.nodeType === Node.COMMENT_NODE) return '';
    if (node.nodeType !== Node.ELEMENT_NODE) return '';
    const tag = node.tagName.toLowerCase();
    const attrs = serializeAttrs(node);
    let inner = '';
    if (node.shadowRoot) {
      inner = serializeShadowRoot(node);
    } else {
      for (const child of node.childNodes) inner += serialize(child);
    }
    if (VOID.has(tag)) return '<' + tag + attrs + '>';
    return '<' + tag + attrs + '>' + inner + '</' + tag + '>';
  };
  const serializeShadowRoot = (host) => {
    let result = '';
    for (const child of host.shadowRoot.childNodes) {
      result += serializeShadowChild(child, host);
    }
    return result;
  };
  const serializeShadowChild = (node, host) => {
    if (node.nodeType === Node.TEXT_NODE) return node.textContent;
    if (node.nodeType === Node.COMMENT_NODE) return '';
    if (node.nodeType !== Node.ELEMENT_NODE) return '';
    const tag = node.tagName.toLowerCase();
    if (tag === 'style') return '';
    if (tag === 'slot') {
      const assigned = node.assignedNodes({ flatten: true });
      if (assigned.length > 0) {
        let out = '';
        for (const a of assigned) out += serialize(a);
        return out;
      }
      let fallback = '';
      for (const child of node.childNodes) fallback += serializeShadowChild(child, host);
      return fallback;
    }
    const attrs = serializeAttrs(node);
    let inner = '';
    if (node.shadowRoot) {
      inner = serializeShadowRoot(node);
    } else {
      for (const child of node.childNodes) inner += serializeShadowChild(child, host);
    }
    if (VOID.has(tag)) return '<' + tag + attrs + '>';
    return '<' + tag + attrs + '>' + inner + '</' + tag + '>';
  };
  return serialize(document.documentElement);
})()
`

// autoScrollJS scrolls through lazy-loaded content to trigger rendering.
const autoScrollJS = `
(() => {
  const maxSteps = 12;
  let steps = 0;
  while (steps < maxSteps) {
    window.scrollBy(0, window.innerHeight);
    steps++;
    if (document.body.scrollHeight - window.scrollY - window.innerHeight < 100) break;
  }
  return steps;
})()
`

// autoClickRevealJS clicks "load more" style controls to reveal deferred content.
const autoClickRevealJS = `
(() => {
  const maxClicks = 5;
  const buttons = document.querySelectorAll('button, [role="button"], a');
  let clicks = 0;
  for (const btn of buttons) {
    if (clicks >= maxClicks) break;
    const text = (btn.innerText || btn.textContent || '').trim().toLowerCase();
    if (text.match(/load more|show more|continue|read more|see all|view more/i)) {
      btn.click();
      clicks++;
    }
  }
  return clicks;
})()
`

// stealthScript returns the JavaScript snippet used to hide automation signals.
func stealthScript() string {
	return stealthInjectionJS
}

// cookieBannerDismissScript returns the JavaScript snippet that dismisses consent banners.
func cookieBannerDismissScript() string {
	return cookieBannerDismissJS
}

// shadowDOMFlattenScript returns the JavaScript snippet that serializes shadow DOM.
func shadowDOMFlattenScript() string {
	return shadowDOMFlattenJS
}

// autoScrollScript returns the JavaScript snippet that scrolls through lazy content.
func autoScrollScript() string {
	return autoScrollJS
}

// autoClickRevealScript returns the JavaScript snippet that clicks "load more" controls.
func autoClickRevealScript() string {
	return autoClickRevealJS
}
