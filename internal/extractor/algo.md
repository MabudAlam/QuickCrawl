# HTML Processing Algorithm Documentation

## Overview

This module implements a Mozilla Readability-inspired algorithm for extracting main article content from HTML pages. The processing pipeline has three stages:

```
Raw HTML → Preprocess → Readability.Parse → Postprocess → Clean HTML
```

## Pipeline Architecture

```mermaid
flowchart TD
    A[Raw HTML] --> B[preprocessHTML]
    B --> C[stripDocumentHead]
    B --> D[cleanNoise]
    B --> E[filterBySelectors]
    B --> F[removeElementsBySelectors]
    B --> G[applyNoisePatterns]
    D --> C
    E --> C
    F --> C
    G --> C
    C --> H[extractWithReadability]
    H --> I[Readability.Parse]
    I --> J[grabArticle]
    I --> K[getArticleMetadata]
    J --> L[prepArticle]
    L --> M[postProcessContent]
    M --> N[postprocessHTML]
    N --> O[Clean HTML Output]
```

---

## Stage 1: Preprocessing (`preprocessHTML`)

### Purpose
Cleans raw HTML before readability parsing by removing noise, applying selectors, and stripping the `<head>` element.

### Flow

```mermaid
flowchart LR
    A[Raw HTML] --> B[stripDocumentHead]
    B --> C[cleanNoise]
    C --> D{IncludeTags?}
    D -->|Yes| E[filterBySelectors]
    E --> F{ExcludeTags?}
    D -->|No| F
    F -->|Yes| G[removeElementsBySelectors]
    F -->|No| H{CSSSelector?}
    G --> H
    H -->|Yes| I[applyCSSSelector]
    H -->|No| J{SkipNoisePatterns?}
    I --> J
    J -->|No| K[applyNoisePatterns]
    J -->|Yes| L[Cleaned HTML]
    K --> L
```

### Methods

#### `stripDocumentHead(html string) string`
- Removes the entire `<head>` element from HTML
- Uses goquery to parse and remove `<head>` nodes
- Returns the remaining body HTML

#### `cleanNoise(html string) string`
Removes noise elements using regex patterns:

| Pattern | Element Removed |
|---------|-----------------|
| `scriptRe` | `<script>...</script>` |
| `styleRe` | `<style>...</style>` |
| `noScriptRe` | `<noscript>...</noscript>` |
| `iframeRe` | `<iframe>...</iframe>` |
| `svgRe` | `<svg>...</svg>` |
| `dataImgRe` | `<img src="data:...">` |
| `urlTextRe` | URLs in text |
| `buttonRe` | `<button>...</button>` |

#### `filterBySelectors(html string, selectors []string) string`
- Finds elements matching each CSS selector
- Extracts text content from matched elements
- Returns concatenated text or original HTML if no matches

#### `removeElementsBySelectors(html string, selectors []string) string`
- Removes all elements matching any of the provided CSS selectors
- Used to exclude unwanted content (e.g., ads, navigation)

#### `applyCSSSelector(html, selector string) string`
- Applies a single CSS selector to extract specific content
- Returns HTML content of first match or empty string

#### `applyNoisePatterns(html string) string`
Identifies and removes noise wrapper elements by checking:
- Class/ID against `noisePatterns` list (sidebar, nav, footer, etc.)
- Exact token matches (`toc`, `share`, `related`, etc.)
- Prefix matches (`ad-`, `ads-`)
- ARIA roles (`navigation`, `banner`, `contentinfo`, `complementary`)

#### `isNoiseWrapper(attributes, role string) bool`
```mermaid
flowchart TD
    A[Start] --> B[Check noisePatterns]
    B --> C{Match found?}
    C -->|Yes| D[Return true]
    C -->|No| E[Tokenize attributes]
    E --> F{Exact match?}
    F -->|Yes| D
    F -->|No| G{Prefix match?}
    G -->|Yes| D
    G -->|No| H{Role in blacklist?}
    H -->|Yes| D
    H -->|No| I[Return false]
```

---

## Stage 2: Readability Parsing

### Entry Point: `Parse(input io.Reader, pageURL string) (Article, error)`

```mermaid
sequenceDiagram
    participant Input
    participant Parse
    participant prepDocument
    participant getArticleMetadata
    participant grabArticle
    participant postProcessContent

    Input->>Parse: io.Reader, pageURL
    Parse->>Parse: url.ParseRequestURI
    Parse->>Parse: html.Parse
    Parse->>prepDocument: removeScripts, prepDocument
    Parse->>getArticleMetadata: extract metadata
    Parse->>grabArticle: find main content
    grabArticle->>grabArticle: loop with fallback flags
    Parse->>postProcessContent: fixURIs, cleanClasses, clearAttr
    Parse->>Output: Article
```

### Main Parse Flow

```mermaid
flowchart TD
    A[Parse] --> B[Initialize flags]
    B --> C[Parse URL]
    B --> D[Parse HTML]
    D --> E{MaxElemsToParse?}
    E -->|Yes| F{Check element count}
    E -->|No| G[Continue]
    F -->|Too many| H[Error]
    F -->|OK| G
    G --> I[removeScripts]
    I --> J[prepDocument]
    J --> K[getArticleMetadata]
    K --> L[grabArticle]
    L --> M{articleContent nil?}
    M -->|Yes| N[Return empty]
    M -->|No| O[postProcessContent]
    O --> P[Extract excerpt]
    P --> Q[Return Article]
```

### `grabArticle()` - Core Content Detection

This is the heart of the algorithm. It identifies the main article content through scoring.

```mermaid
flowchart TD
    A[Start] --> B[Clone document]
    B --> C[Get body]
    C --> D{body exists?}
    D -->|No| E[Return nil]
    D -->|Yes| F[Initialize candidates]
    F --> G[Traverse all nodes]
    G --> H{Visible node?}
    H -->|No| I[Remove and get next]
    H -->|Yes| J{Byline match?}
    J -->|Yes| I
    J -->|No| K{Unlikely candidate?}
    K -->|Yes| L{Not okMaybeCandidate?}
    L -->|Yes| I
    K -->|No| M{Empty element?}
    M -->|Yes| I
    M -->|No| N{Tag in TagsToScore?}
    N -->|Yes| O[Add to elementsToScore]
    N -->|No| P{Div tag?}
    P -->|Yes| Q[Process div children]
    Q --> R{Convert to paragraphs?}
    R -->|Yes| S[Add to elementsToScore]
    P -->|No| T[Continue]
    O --> T
    S --> T
    T --> G
    G --> U{More nodes?}
    U -->|Yes| G
    U -->|No| V[Calculate scores]
    V --> W[Sort candidates]
    W --> X[Select top candidates]
    X --> Y{Top candidate valid?}
    Y -->|No| Z[Create from body]
    Y -->|Yes| AA[Find common ancestor]
    AA --> AB[Build article content]
    AB --> AC{Parse successful?}
    AC -->|No| AD[Adjust flags, retry]
    AC -->|Yes| AE[Return articleContent]
    AD --> B
    Z --> AB
```

#### Step 1: Node Traversal & Candidate Collection

For each node in document tree:
1. **Visibility Check** (`isProbablyVisible`): Skip hidden elements
2. **Byline Check** (`checkByline`): Extract author info
3. **Unlikely Candidates Filter**: Remove ad-related, sidebar elements
4. **Empty Element Check**: Remove elements with only `<br>` or `<hr>`
5. **Scoring Tags**: Add `<section>`, `<h2-h6>`, `<p>`, `<td>`, `<pre>` to scoring list
6. **Div Processing**: Convert `<div>` children to `<p>` if phrasing content

#### Step 2: Content Scoring

```mermaid
flowchart TD
    A[For each elementToScore] --> B{Parent exists?}
    B -->|No| C[Skip]
    B -->|Yes| D{Inner text >= 25?}
    D -->|No| C
    D -->|Yes| E[Get 3 ancestors]
    E --> F[Calculate base score]
    F --> G[+1 per comma]
    G --> H[+ floor(len/100), max 3]
    H --> I[For each ancestor]
    I --> J{Ancestor has score?}
    J -->|No| K[Initialize with classWeight]
    K --> L[Add to candidates]
    J -->|Yes| M[Divide score by level]
    M --> L
    L --> N{Continue ancestors?}
    N -->|Yes| I
    N -->|No| O[Next element]
    O --> A
```

**Scoring Formula:**
```
baseScore = 1 + commaCount + floor(textLength/100) [capped at 3]
ancestorScore = baseScore / levelDivider
where levelDivider: 0→1, 1→2, 2→6, 3→9, etc.
```

**Initial Node Scores** (`initializeNode`):
- `<div>`: +5
- `<pre>`, `<td>`, `<blockquote>`: +3
- `<address>`, `<ol>`, `<ul>`, `<dl>`, `<dd>`, `<dt>`, `<li>`, `<form>`: -3
- `<h1-h6>`, `<th>`: -5
- Class-based weights (from `getClassWeight`)

#### Step 3: Link Density Adjustment

```mermaid
flowchart LR
    A[Candidate] --> B[getLinkDensity]
    B --> C[linkTextLength / totalTextLength]
    C --> D[newScore = score * (1 - linkDensity)]
```

#### Step 4: Top Candidate Selection

1. Sort candidates by score (descending)
2. Take top N (default 5)
3. If top is `<body>` or nil, create candidate from body children
4. Check for common ancestor among alternatives (if >= 3 candidates have 75% of top score)
5. Walk up to find parent with sufficient score

#### Step 5: Article Content Building

```mermaid
flowchart TD
    A[Start with topCandidate] --> B[Calculate threshold]
    B --> C[siblingScoreThreshold = max10, topScore * 0.2]
    C --> D[Get siblings of parent]
    D --> E[For each sibling]
    E --> F{Is topCandidate?}
    F -->|Yes| G[appendNode = true]
    F -->|No| H{Has matching class?}
    H -->|Yes| I[contentBonus = score * 0.2]
    I --> J{score + bonus >= threshold?}
    J -->|Yes| G
    J -->|No| K{Tag is p?}
    K -->|Yes| L{Check link density}
    L --> M{len > 80 && density < 0.25?}
    M -->|Yes| G
    M -->|No| N{len < 80 && density = 0?}
    N -->|Yes| O{Has sentence period?}
    O -->|Yes| G
    N -->|No| P[Don't append]
    K -->|No| P
    H -->|No| P
    G --> Q[Convert to div if needed]
    Q --> R[Append to articleContent]
    P --> R
    R --> S{More siblings?}
    S -->|Yes| E
    S -->|No| T[Return content]
```

### `prepArticle(articleContent *html.Node)`

Cleans the extracted content:

```mermaid
flowchart TD
    A[Input: articleContent] --> B[cleanStyles]
    B --> C[markDataTables]
    C --> D[cleanConditionally form]
    D --> E[cleanConditionally fieldset]
    E --> F[clean object, embed, footer, link, aside]
    F --> G[cleanMatchedNodes share patterns]
    G --> H{H2 with similar title?}
    H -->|Yes| I[Remove H2]
    H -->|No| J[Clean iframe, input, textarea, select, button]
    J --> K[cleanHeaders]
    K --> L[cleanConditionally table, ul, div]
    L --> M[Remove empty paragraphs]
    M --> N[Remove br followed by p]
    N --> O[Convert simple tables to div/p]
    O --> P[Return cleaned content]
```

### Metadata Extraction (`getArticleMetadata`)

```mermaid
flowchart TD
    A[Scan all meta tags] --> B{Property attribute?}
    B -->|Yes| C[Match propertyPattern]
    B -->|No| D{Name attribute matches?}
    D -->|Yes| E[Match namePattern]
    D -->|No| F[Skip]
    C --> G[Store in values map]
    E --> G
    G --> H[Extract title from priority list]
    H --> I{dc:title, dcterm:title, og:title...}
    I --> J[Fallback to getArticleTitle]
    J --> K[Extract byline]
    K --> L{dc:creator, dcterm:creator, author}
    L --> M[Extract excerpt]
    M --> N{description, og:description, twitter:description}
    N --> O[Extract siteName]
    O --> P[og:site_name]
    P --> Q[Extract image]
    Q --> R{og:image, image, twitter:image}
    R --> S[Get favicon]
    S --> T[Return Article metadata]
```

### Article Title Extraction (`getArticleTitle`)

```mermaid
flowchart TD
    A[Get title tag content] --> B{Hierarchical separators?}
    B -->|Yes| C{Remove final part}
    C --> D{Word count < 3?}
    D -->|Yes| E[Remove first part]
    D -->|No| F[Keep current]
    B -->|No| G{Has colon?}
    G -->|Yes| H{Heading matches?}
    G -->|No| I{Length check}
    I --> J{Length > 150 or < 15?}
    J -->|Yes| K[Use h1]
    J -->|No| L[Keep title]
    H -->|No| M[Extract after last colon]
    M --> N{Word count < 3?}
    N -->|Yes| O[Extract after first colon]
    N -->|No| P{First part > 5 words?}
    P -->|Yes| L
    P -->|No| F
    E --> F
    H -->|Yes| F
    K --> F
    O --> F
```

### Fallback Mechanism

If initial parse fails (textLength < 500), the algorithm retries with relaxed flags:

```mermaid
flowchart TD
    A[First attempt: stripUnlikelys=true] --> B{Success?}
    B -->|Yes| C[Return content]
    B -->|No| D[Retry: stripUnlikelys=false]
    D --> E{Success?}
    E -->|Yes| C
    E -->|No| F[Retry: useWeightClasses=false]
    F --> G{Success?}
    G -->|Yes| C
    G -->|No| H[Retry: cleanConditionally=false]
    H --> I{Success?}
    I -->|Yes| C
    I -->|No| J[Use best attempt]
    J --> K{Select by textLength}
    K --> C
```

---

## Stage 3: Postprocessing

### `postProcessContent(articleContent *html.Node)`

```mermaid
flowchart TD
    A[Input: articleContent] --> B[fixRelativeURIs]
    B --> C[Convert relative to absolute]
    C --> D[cleanClasses]
    D --> E[Remove all classes except 'page']
    E --> F[clearReadabilityAttr]
    F --> G[Remove data-readability-* attributes]
    G --> H[Return cleaned content]
```

### `postprocessHTML(html string) string`

Final regex-based cleanup:

```mermaid
flowchart LR
    A[HTML Input] --> B[Remove img tags]
    B --> C[Remove anchor tags]
    C --> D[Remove self-closing anchors]
    D --> E{Iterate cleanup}
    E --> F[Remove empty divs]
    F --> G[Remove empty spans]
    G --> H{Something removed?}
    H -->|Yes| E
    H -->|No| I[Normalize whitespace]
    I --> J[Normalize newlines]
    J --> K[Trim and return]
```

---

## Readability Methods Reference

### DOM Helper Functions

| Method | Purpose |
|--------|---------|
| `firstElementChild(node)` | Get first child element |
| `nextElementSibling(node)` | Get next sibling element |
| `appendChild(node, child)` | Add child (move if exists) |
| `childNodes(node)` | Get all direct children |
| `includeNode(nodeList, node)` | Check if node in list |
| `cloneNode(node)` | Deep clone node |
| `createElement(tagName)` | Create new element |
| `createTextNode(data)` | Create text node |
| `getElementsByTagName(node, tag)` | Find all by tag |
| `getAttribute(node, attr)` | Get attribute value |
| `setAttribute(node, attr, val)` | Set attribute |
| `removeAttribute(node, attr)` | Remove attribute |
| `hasAttribute(node, attr)` | Check attribute exists |
| `outerHTML(node)` | Serialize node and children |
| `innerHTML(node)` | Serialize children only |
| `documentElement(doc)` | Get `<html>` root |
| `className(node)` | Get class attribute |
| `id(node)` | Get id attribute |
| `children(node)` | Get element children only |
| `tagName(node)` | Get tag name |
| `textContent(node)` | Get all text |
| `toAbsoluteURI(uri, base)` | Convert to absolute URL |

### Readability Core Methods

| Method | Purpose |
|--------|---------|
| `Parse(input, pageURL)` | Main entry point |
| `grabArticle()` | Find main content |
| `prepArticle(content)` | Clean extracted content |
| `getArticleMetadata()` | Extract meta tags |
| `getArticleTitle()` | Extract/normalize title |
| `getArticleFavicon()` | Find favicon |
| `prepDocument()` | Initial document prep |

### Scoring & Analysis Methods

| Method | Purpose |
|--------|---------|
| `initializeNode(node)` | Set initial score |
| `getContentScore(node)` | Get score from attribute |
| `setContentScore(node, score)` | Store score in attribute |
| `getClassWeight(node)` | Calculate class-based weight |
| `getLinkDensity(element)` | Calculate link text ratio |
| `getInnerText(node, normalize)` | Get text with optional normalize |
| `getCharCount(node, s)` | Count character occurrences |
| `getNodeAncestors(node, depth)` | Get parent chain |

### Traversal Methods

| Method | Purpose |
|--------|---------|
| `removeAndGetNext(node)` | Remove node, return next |
| `getNextNode(node, ignoreKids)` | Tree traversal |
| `getAllNodesWithTag(node, tags...)` | Find multiple tag types |

### Content Detection Methods

| Method | Purpose |
|--------|---------|
| `isProbablyVisible(node)` | Check visibility |
| `isPhrasingContent(node)` | Check if text-like |
| `isWhitespace(node)` | Check if whitespace |
| `isElementWithoutContent(node)` | Check empty element |
| `hasChildBlockElement(element)` | Check for block children |
| `hasSingleTagInsideElement(elem, tag)` | Check single child |
| `checkByline(node, matchStr)` | Detect byline |
| `isValidByline(byline)` | Validate byline format |

### Cleaning Methods

| Method | Purpose |
|--------|---------|
| `clean(node, tag)` | Remove elements by tag |
| `cleanStyles(node)` | Remove style attributes |
| `cleanConditionally(elem, tag)` | Conditional removal |
| `cleanMatchedNodes(e, filter)` | Pattern-based removal |
| `cleanHeaders(e)` | Remove negative headers |
| `cleanClasses(node)` | Remove unwanted classes |
| `cleanTables(root)` | Mark data tables |
| `removeNodes(list, filter)` | Bulk removal |
| `replaceNodeTags(list, tag)` | Bulk tag change |

### URI & Utility Methods

| Method | Purpose |
|--------|---------|
| `fixRelativeURIs(content)` | Convert relative URLs |
| `removeScripts(doc)` | Remove script elements |
| `removeComments(doc)` | Remove HTML comments |
| `hasAncestorTag(node, tag, depth, filter)` | Check ancestry |
| `getRowAndColumnCount(table)` | Table analysis |
| `isReadabilityDataTable(node)` | Check data table flag |
| `setReadabilityDataTable(node, bool)` | Set data table flag |

### Array/Collection Helpers

| Method | Purpose |
|--------|---------|
| `forEachNode(list, fn)` | Iterate with index |
| `someNode(list, fn)` | Any match? |
| `everyNode(list, fn)` | All match? |
| `concatNodeLists(lists...)` | Concatenate lists |
| `indexOf(array, key)` | Find index |
| `wordCount(str)` | Count words |

---

## IsReadable Quick Check

```mermaid
flowchart TD
    A[Quick scan for p, pre, br] --> B[Build node list]
    B --> C[For each node]
    C --> D{Probably visible?}
    D -->|No| E[Skip]
    D -->|Yes| F{Unlikely candidate?}
    F -->|Yes| E
    F -->|No| G{Parent is li?}
    G -->|Yes| E
    G -->|No| H{Text length >= 140?}
    H -->|Yes| I[score += sqrt(len - 140)]
    I --> J{score > 20?}
    J -->|Yes| K[Return true]
    J -->|No| C
    H -->|No| C
    E --> C
    C --> L{More nodes?}
    L -->|Yes| C
    L -->|No| M[Return false]
```

---

## Constants Reference

### `divToPElems`
Elements that trigger div-to-p conversion: `a`, `blockquote`, `div`, `dl`, `img`, `ol`, `p`, `pre`, `select`, `table`, `ul`

### `alterToDivExceptions`
Tags NOT converted to div when building article: `article`, `div`, `p`, `section`

### `presentationalAttributes`
Removed during style cleaning: `align`, `background`, `bgcolor`, `border`, `cellpadding`, `cellspacing`, `frame`, `hspace`, `rules`, `style`, `valign`, `vspace`

### `deprecatedSizeAttributeElems`
Width/height removed from: `table`, `th`, `td`, `hr`, `pre`

### `phrasingElems`
Inline/text elements for phrasing content check: `abbr`, `audio`, `b`, `bdo`, `br`, `button`, `cite`, `code`, `data`, `datalist`, `dfn`, `em`, `embed`, `i`, `img`, `input`, `kbd`, `label`, `mark`, `math`, `meter`, `noscript`, `object`, `output`, `progress`, `q`, `ruby`, `samp`, `script`, `select`, `small`, `span`, `strong`, `sub`, `sup`, `textarea`, `time`, `var`, `wbr`

---

## Regular Expressions Reference

### Candidate Detection
- `rxUnlikelyCandidates`: `-ad-`, `banner`, `sidebar`, `share`, `social`, `comment`, etc.
- `rxOkMaybeItsACandidate`: `and`, `article`, `body`, `column`, `main`, `shadow`
- `rxPositive`: `article`, `body`, `content`, `entry`, `main`, `page`, `post`, `text`, `blog`
- `rxNegative`: `hidden`, `banner`, `contact`, `footer`, `meta`, `sidebar`, `widget`

### Metadata
- `rxByline`: `byline`, `author`, `dateline`, `writtenby`, `p-author`
- `rxPropertyPattern`: Matches `og:`, `dc:`, `dcterm:`, `twitter:` property names
- `rxNamePattern`: Matches meta name attributes

### Title Processing
- `rxTitleSeparator`: `|`, `-`, `\`, `/`, `»` with spaces
- `rxTitleHierarchySep`: Hierarchy separators
- `rxTitleRemoveFinalPart`: Remove after separator
- `rxTitleRemove1stPart`: Remove before separator
- `rxTitleAnySeparator`: Any separator pattern

### Video Detection
- `rxVideos`: YouTube, Vimeo, DailyMotion, QQ, Twitch, Wikipedia archives

---

## Default Configuration

```go
MaxElemsToParse:   0      // No limit
NTopCandidates:    5      // Keep top 5 candidates
CharThresholds:    500    // Min text length for success
ClassesToPreserve: ["page"]  // Keep page class
TagsToScore:       ["section", "h2", "h3", "h4", "h5", "h6", "p", "td", "pre"]
KeepClasses:       false  // Strip classes
```