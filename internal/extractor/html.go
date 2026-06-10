// Package extractor provides content extraction from HTML into various formats.
package extractor

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

// ─── HTML Content Extraction ────────────────────────────────────────────────
//
// This module provides HTML content extraction using a port of Mozilla's Readability
// algorithm combined with preprocessing and postprocessing stages.
//
// Pipeline:
//  1. preprocessHTML - strips head, applies include/exclude selectors, CSS selector
//  2. Readability.Parse - extracts main article content using readability scoring
//  3. postprocessHTML - sanitizes output, removes images/URLs, unwraps wrappers
//
// Key design decisions:
//   - Images and URLs are stripped from HTML output (they are extracted separately)
//   - Readability handles content identification and noise removal internally
//   - Preprocessing selectors allow targeted extraction before readability runs
//   - Postprocessing ensures clean, minimal HTML output

// ─── DOM Helper Functions ─────────────────────────────────────────────────

// firstElementChild returns the object's first child Element, or nil if there
// are no child elements.
func firstElementChild(node *html.Node) *html.Node {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode {
			return child
		}
	}
	return nil
}

// nextElementSibling returns the Element immediately following the specified
// one in its parent's children list, or nil if the specified Element is the
// last one in the list.
func nextElementSibling(node *html.Node) *html.Node {
	for sibling := node.NextSibling; sibling != nil; sibling = sibling.NextSibling {
		if sibling.Type == html.ElementNode {
			return sibling
		}
	}
	return nil
}

// appendChild adds a node to the end of the list of children of a specified
// parent node. If the given child is a reference to an existing node in the
// document, appendChild moves it from its current position to the new position.
func appendChild(node *html.Node, child *html.Node) {
	if child.Parent != nil {
		temp := cloneNode(child)
		node.AppendChild(temp)
		child.Parent.RemoveChild(child)
		return
	}
	node.AppendChild(child)
}

// childNodes returns list of a node's direct children.
func childNodes(node *html.Node) []*html.Node {
	var list []*html.Node
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		list = append(list, c)
	}
	return list
}

// includeNode determines if node is included inside nodeList.
func includeNode(nodeList []*html.Node, node *html.Node) bool {
	for i := 0; i < len(nodeList); i++ {
		if nodeList[i] == node {
			return true
		}
	}
	return false
}

// cloneNode returns a duplicate of the node on which this method was called.
func cloneNode(node *html.Node) *html.Node {
	clone := &html.Node{
		Type:     node.Type,
		DataAtom: node.DataAtom,
		Data:     node.Data,
		Attr:     make([]html.Attribute, len(node.Attr)),
	}
	copy(clone.Attr, node.Attr)
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		clone.AppendChild(cloneNode(c))
	}
	return clone
}

// createElement creates the HTML element specified by tagName.
func createElement(tagName string) *html.Node {
	return &html.Node{Type: html.ElementNode, Data: tagName}
}

// createTextNode creates a new Text node.
func createTextNode(data string) *html.Node {
	return &html.Node{Type: html.TextNode, Data: data}
}

// getElementsByTagName returns a collection of HTML elements with the given tag name.
func getElementsByTagName(node *html.Node, tag string) []*html.Node {
	var lst []*html.Node
	var fun func(*html.Node)
	fun = func(n *html.Node) {
		if n.Type == html.ElementNode && (tag == "*" || n.Data == tag) {
			lst = append(lst, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			fun(c)
		}
	}
	fun(node)
	return lst
}

// getAttribute returns the value of a specified attribute on the element.
func getAttribute(node *html.Node, attrName string) string {
	for i := 0; i < len(node.Attr); i++ {
		if node.Attr[i].Key == attrName {
			return node.Attr[i].Val
		}
	}
	return ""
}

// setAttribute sets attribute for node. If attribute already exists, it will be replaced.
func setAttribute(node *html.Node, attrName string, attrValue string) {
	attrIdx := -1
	for i := 0; i < len(node.Attr); i++ {
		if node.Attr[i].Key == attrName {
			attrIdx = i
			break
		}
	}
	if attrIdx >= 0 {
		node.Attr[attrIdx].Val = attrValue
		return
	}
	node.Attr = append(node.Attr, html.Attribute{Key: attrName, Val: attrValue})
}

// removeAttribute removes attribute with given name.
func removeAttribute(node *html.Node, attrName string) {
	attrIdx := -1
	for i := 0; i < len(node.Attr); i++ {
		if node.Attr[i].Key == attrName {
			attrIdx = i
			break
		}
	}
	if attrIdx >= 0 {
		a := node.Attr
		a = append(a[:attrIdx], a[attrIdx+1:]...)
		node.Attr = a
	}
}

// hasAttribute returns a Boolean value indicating whether the specified node has the specified attribute.
func hasAttribute(node *html.Node, attrName string) bool {
	for i := 0; i < len(node.Attr); i++ {
		if node.Attr[i].Key == attrName {
			return true
		}
	}
	return false
}

// outerHTML returns an HTML serialization of the element and its descendants.
func outerHTML(node *html.Node) string {
	var buffer bytes.Buffer
	if err := html.Render(&buffer, node); err != nil {
		return ""
	}
	return buffer.String()
}

// innerHTML returns the HTML content (inner HTML) of an element.
func innerHTML(node *html.Node) string {
	var err error
	var buffer bytes.Buffer
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if err = html.Render(&buffer, child); err != nil {
			return ""
		}
	}
	return strings.TrimSpace(buffer.String())
}

// documentElement returns the root element of the document.
func documentElement(doc *html.Node) *html.Node {
	nodes := getElementsByTagName(doc, "html")
	if len(nodes) > 0 {
		return nodes[0]
	}
	return nil
}

// className returns the value of the class attribute of the element.
func className(node *html.Node) string {
	className := getAttribute(node, "class")
	className = strings.TrimSpace(className)
	className = rxNormalize.ReplaceAllString(className, "\x20")
	return className
}

// id returns the value of the id attribute of the specified element.
func id(node *html.Node) string {
	id := getAttribute(node, "id")
	id = strings.TrimSpace(id)
	return id
}

// children returns an HTMLCollection of the child elements of Node.
func children(node *html.Node) []*html.Node {
	var children []*html.Node
	if node == nil {
		return nil
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode {
			children = append(children, child)
		}
	}
	return children
}

// wordCount returns number of word in str.
func wordCount(str string) int {
	return len(strings.Fields(str))
}

// indexOf returns the first index at which a given element can be found in the array.
func indexOf(array []string, key string) int {
	for idx, val := range array {
		if val == key {
			return idx
		}
	}
	return -1
}

// replaceNode replaces a child node within the given (parent) node.
func replaceNode(oldNode *html.Node, newNode *html.Node) {
	if oldNode.Parent == nil {
		return
	}
	newNode.Parent = nil
	newNode.PrevSibling = nil
	newNode.NextSibling = nil
	oldNode.Parent.InsertBefore(newNode, oldNode)
	oldNode.Parent.RemoveChild(oldNode)
}

// tagName returns the tag name of the element on which it's called.
func tagName(node *html.Node) string {
	if node.Type != html.ElementNode {
		return ""
	}
	return node.Data
}

// textContent returns text content of a Node and its descendants.
func textContent(node *html.Node) string {
	var buffer bytes.Buffer
	var finder func(*html.Node)
	finder = func(n *html.Node) {
		if n.Type == html.TextNode {
			buffer.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			finder(c)
		}
	}
	finder(node)
	return buffer.String()
}

// toAbsoluteURI convert uri to absolute path based on base.
func toAbsoluteURI(uri string, base *url.URL) string {
	if uri == "" || base == nil {
		return ""
	}
	if uri[:1] == "#" {
		return uri
	}
	tmp, err := url.ParseRequestURI(uri)
	if err == nil && tmp.Scheme != "" && tmp.Hostname() != "" {
		return uri
	}
	tmp, err = url.Parse(uri)
	if err != nil {
		return uri
	}
	return base.ResolveReference(tmp).String()
}

// ─── Regular Expressions ───────────────────────────────────────────────────

var rxUnlikelyCandidates = regexp.MustCompile(`(?i)-ad-|ai2html|banner|breadcrumbs|combx|comment|community|cover-wrap|disqus|extra|foot|gdpr|header|legends|menu|related|remark|replies|rss|shoutbox|sidebar|skyscraper|social|sponsor|supplemental|ad-break|agegate|pagination|pager|popup|yom-remote`)
var rxOkMaybeItsACandidate = regexp.MustCompile(`(?i)and|article|body|column|main|shadow`)
var rxPositive = regexp.MustCompile(`(?i)article|body|content|entry|hentry|h-entry|main|page|pagination|post|text|blog|story`)
var rxNegative = regexp.MustCompile(`(?i)hidden|^hid$| hid$| hid |^hid |banner|combx|comment|com-|contact|foot|footer|footnote|gdpr|masthead|media|meta|outbrain|promo|related|scroll|share|shoutbox|sidebar|skyscraper|sponsor|shopping|tags|tool|widget`)
var rxByline = regexp.MustCompile(`(?i)byline|author|dateline|writtenby|p-author`)
var rxNormalize = regexp.MustCompile(`(?i)\s{2,}`)
var rxVideos = regexp.MustCompile(`(?i)//(www\.)?((dailymotion|youtube|youtube-nocookie|player\.vimeo|v\.qq)\.com|(archive|upload\.wikimedia)\.org|player\.twitch\.tv)`)
var rxWhitespace = regexp.MustCompile(`(?i)^\s*$`)
var rxHasContent = regexp.MustCompile(`(?i)\S$`)
var rxPropertyPattern = regexp.MustCompile(`(?i)\s*(dc|dcterm|og|twitter)\s*:\s*(author|creator|description|title|site_name|image\S*)\s*`)
var rxNamePattern = regexp.MustCompile(`(?i)^\s*(?:(dc|dcterm|og|twitter|weibo:(article|webpage))\s*[\.:]\s*)?(author|creator|description|title|site_name|image)\s*$`)
var rxTitleSeparator = regexp.MustCompile(`(?i) [\|\-\\/>»] `)
var rxTitleHierarchySep = regexp.MustCompile(`(?i) [\\/>»] `)
var rxTitleRemoveFinalPart = regexp.MustCompile(`(?i)(.*)[\|\-\\/>»] .*`)
var rxTitleRemove1stPart = regexp.MustCompile(`(?i)[^\|\-\\/>»]*[\|\-\\/>»](.*)`)
var rxTitleAnySeparator = regexp.MustCompile(`(?i)[\|\-\\/>»]+`)
var rxDisplayNone = regexp.MustCompile(`(?i)display\s*:\s*none`)
var rxSentencePeriod = regexp.MustCompile(`(?i)\.( |$)`)
var rxShare = regexp.MustCompile(`(?i)share`)
var rxFaviconSize = regexp.MustCompile(`(?i)(\d+)x(\d+)`)

// ─── Constants ────────────────────────────────────────────────────────────

var divToPElems = []string{
	"a", "blockquote", "div", "dl", "img",
	"ol", "p", "pre", "select", "table", "ul",
}

var alterToDivExceptions = []string{
	"article", "div", "p", "section",
}

var presentationalAttributes = []string{
	"align", "background", "bgcolor", "border", "cellpadding",
	"cellspacing", "frame", "hspace", "rules", "style", "valign", "vspace",
}

var deprecatedSizeAttributeElems = []string{
	"table", "th", "td", "hr", "pre",
}

var phrasingElems = []string{
	"abbr", "audio", "b", "bdo", "br", "button", "cite", "code", "data",
	"datalist", "dfn", "em", "embed", "i", "img", "input", "kbd", "label",
	"mark", "math", "meter", "noscript", "object", "output", "progress", "q",
	"ruby", "samp", "script", "select", "small", "span", "strong", "sub",
	"sup", "textarea", "time", "var", "wbr",
}

// ─── Readability Types ────────────────────────────────────────────────────

type flags struct {
	stripUnlikelys     bool
	useWeightClasses   bool
	cleanConditionally bool
}

type parseAttempt struct {
	articleContent *html.Node
	textLength     int
}

// Article represents the metadata and content of the article.
type Article struct {
	Title       string
	Byline      string
	Dir         string
	Content     string
	TextContent string
	Excerpt     string
	SiteName    string
	Favicon     string
	Image       string
	Length      int
	Node        *html.Node
}

// Readability is an HTML parser that reads and extract relevant content.
type Readability struct {
	doc               *html.Node
	documentURI       *url.URL
	articleTitle      string
	articleByline     string
	attempts          []parseAttempt
	flags             flags
	MaxElemsToParse   int
	NTopCandidates    int
	CharThresholds    int
	ClassesToPreserve []string
	TagsToScore       []string
	KeepClasses       bool
}

// NewReadability returns new Readability with sane defaults.
func NewReadability() *Readability {
	return &Readability{
		MaxElemsToParse:   0,
		NTopCandidates:    5,
		CharThresholds:    500,
		ClassesToPreserve: []string{"page"},
		TagsToScore:       []string{"section", "h2", "h3", "h4", "h5", "h6", "p", "td", "pre"},
		KeepClasses:       false,
	}
}

// ─── Readability Methods ─────────────────────────────────────────────────

func (r *Readability) removeNodes(list []*html.Node, filter func(*html.Node) bool) {
	var node, parentNode *html.Node
	for i := len(list) - 1; i >= 0; i-- {
		node = list[i]
		parentNode = node.Parent
		if parentNode != nil && (filter == nil || filter(node)) {
			parentNode.RemoveChild(node)
		}
	}
}

func (r *Readability) replaceNodeTags(list []*html.Node, newTagName string) {
	for i := len(list) - 1; i >= 0; i-- {
		r.setNodeTag(list[i], newTagName)
	}
}

func (r *Readability) forEachNode(list []*html.Node, fn func(*html.Node, int)) {
	for idx, node := range list {
		fn(node, idx)
	}
}

func (r *Readability) someNode(nodeList []*html.Node, fn func(*html.Node) bool) bool {
	for i := 0; i < len(nodeList); i++ {
		if fn(nodeList[i]) {
			return true
		}
	}
	return false
}

func (r *Readability) everyNode(list []*html.Node, fn func(*html.Node) bool) bool {
	for _, node := range list {
		if !fn(node) {
			return false
		}
	}
	return true
}

func (r *Readability) concatNodeLists(nodeLists ...[]*html.Node) []*html.Node {
	var result []*html.Node
	for i := 0; i < len(nodeLists); i++ {
		result = append(result, nodeLists[i]...)
	}
	return result
}

func (r *Readability) getAllNodesWithTag(node *html.Node, tagNames ...string) []*html.Node {
	var list []*html.Node
	for _, tag := range tagNames {
		list = append(list, getElementsByTagName(node, tag)...)
	}
	return list
}

func (r *Readability) getArticleTitle() string {
	doc := r.doc
	curTitle := ""
	origTitle := ""
	titleHadHierarchicalSeparators := false

	if nodes := getElementsByTagName(doc, "title"); len(nodes) > 0 {
		origTitle = r.getInnerText(nodes[0], true)
		curTitle = origTitle
	}

	if rxTitleSeparator.MatchString(curTitle) {
		titleHadHierarchicalSeparators = rxTitleHierarchySep.MatchString(curTitle)
		curTitle = rxTitleRemoveFinalPart.ReplaceAllString(origTitle, "$1")
		if wordCount(curTitle) < 3 {
			curTitle = rxTitleRemove1stPart.ReplaceAllString(origTitle, "$1")
		}
	} else if strings.Index(curTitle, ": ") != -1 {
		headings := r.concatNodeLists(
			getElementsByTagName(doc, "h1"),
			getElementsByTagName(doc, "h2"),
		)
		trimmedTitle := strings.TrimSpace(curTitle)
		match := r.someNode(headings, func(heading *html.Node) bool {
			return strings.TrimSpace(textContent(heading)) == trimmedTitle
		})
		if !match {
			curTitle = origTitle[strings.LastIndex(origTitle, ":")+1:]
			if wordCount(curTitle) < 3 {
				curTitle = origTitle[strings.Index(origTitle, ":")+1:]
			} else if wordCount(origTitle[:strings.Index(origTitle, ":")]) > 5 {
				curTitle = origTitle
			}
		}
	} else if len(curTitle) > 150 || len(curTitle) < 15 {
		if hOnes := getElementsByTagName(doc, "h1"); len(hOnes) == 1 {
			curTitle = r.getInnerText(hOnes[0], true)
		}
	}

	curTitle = strings.TrimSpace(curTitle)
	curTitle = rxNormalize.ReplaceAllString(curTitle, "\x20")
	curTitleWordCount := wordCount(curTitle)
	tmpOrigTitle := rxTitleAnySeparator.ReplaceAllString(origTitle, "")

	if curTitleWordCount <= 4 &&
		(!titleHadHierarchicalSeparators ||
			curTitleWordCount != wordCount(tmpOrigTitle)-1) {
		curTitle = origTitle
	}

	return curTitle
}

func (r *Readability) getArticleFavicon() string {
	favicon := ""
	faviconSize := -1
	linkElements := getElementsByTagName(r.doc, "link")

	r.forEachNode(linkElements, func(link *html.Node, _ int) {
		linkRel := strings.TrimSpace(getAttribute(link, "rel"))
		linkType := strings.TrimSpace(getAttribute(link, "type"))
		linkHref := strings.TrimSpace(getAttribute(link, "href"))
		linkSizes := strings.TrimSpace(getAttribute(link, "sizes"))

		if linkHref == "" || !strings.Contains(linkRel, "icon") {
			return
		}
		if linkType != "image/png" && !strings.Contains(linkHref, ".png") {
			return
		}
		size := 0
		for _, sizesLocation := range []string{linkSizes, linkHref} {
			sizeParts := rxFaviconSize.FindStringSubmatch(sizesLocation)
			if len(sizeParts) != 3 || sizeParts[1] != sizeParts[2] {
				continue
			}
			size, _ = strconv.Atoi(sizeParts[1])
			break
		}
		if size > faviconSize {
			faviconSize = size
			favicon = linkHref
		}
	})

	return toAbsoluteURI(favicon, r.documentURI)
}

func (r *Readability) prepDocument() {
	doc := r.doc
	r.removeNodes(getElementsByTagName(doc, "style"), nil)
	if n := getElementsByTagName(doc, "body"); len(n) > 0 && n[0] != nil {
		r.replaceBrs(n[0])
	}
	r.replaceNodeTags(getElementsByTagName(doc, "font"), "SPAN")
}

func (r *Readability) nextElement(node *html.Node) *html.Node {
	next := node
	for next != nil &&
		next.Type != html.ElementNode &&
		rxWhitespace.MatchString(textContent(next)) {
		next = next.NextSibling
	}
	return next
}

func (r *Readability) replaceBrs(elem *html.Node) {
	r.forEachNode(r.getAllNodesWithTag(elem, "br"), func(br *html.Node, _ int) {
		next := br.NextSibling
		replaced := false
		for {
			next = r.nextElement(next)
			if next == nil || tagName(next) == "BR" {
				break
			}
			replaced = true
			brSibling := next.NextSibling
			next.Parent.RemoveChild(next)
			next = brSibling
		}
		if replaced {
			p := createElement("p")
			replaceNode(br, p)
			next = p.NextSibling
			for next != nil {
				if tagName(next) == "br" {
					nextElem := r.nextElement(next.NextSibling)
					if nextElem != nil && tagName(nextElem) == "br" {
						break
					}
				}
				if !r.isPhrasingContent(next) {
					break
				}
				sibling := next.NextSibling
				appendChild(p, next)
				next = sibling
			}
			for p.LastChild != nil && r.isWhitespace(p.LastChild) {
				p.RemoveChild(p.LastChild)
			}
			if tagName(p.Parent) == "P" {
				r.setNodeTag(p.Parent, "div")
			}
		}
	})
}

func (r *Readability) setNodeTag(node *html.Node, newTagName string) {
	if node.Type == html.ElementNode {
		node.Data = newTagName
	}
}

func (r *Readability) getArticleMetadata() Article {
	values := make(map[string]string)
	metaElements := getElementsByTagName(r.doc, "meta")

	r.forEachNode(metaElements, func(element *html.Node, _ int) {
		elementName := getAttribute(element, "name")
		elementProperty := getAttribute(element, "property")
		content := getAttribute(element, "content")
		if content == "" {
			return
		}
		matches := []string{}
		name := ""

		if elementProperty != "" {
			matches = rxPropertyPattern.FindAllString(elementProperty, -1)
			for i := len(matches) - 1; i >= 0; i-- {
				name = strings.ToLower(matches[i])
				name = strings.Join(strings.Fields(name), "")
				values[name] = strings.TrimSpace(content)
			}
		}
		if len(matches) == 0 && elementName != "" && rxNamePattern.MatchString(elementName) {
			name = strings.ToLower(elementName)
			name = strings.Join(strings.Fields(name), "")
			name = strings.Replace(name, ".", ":", -1)
			values[name] = strings.TrimSpace(content)
		}
	})

	metadataTitle := ""
	for _, name := range []string{
		"dc:title", "dcterm:title", "og:title", "weibo:article:title",
		"weibo:webpage:title", "title", "twitter:title",
	} {
		if value, ok := values[name]; ok {
			metadataTitle = value
			break
		}
	}
	if metadataTitle == "" {
		metadataTitle = r.getArticleTitle()
	}

	metadataByline := ""
	for _, name := range []string{"dc:creator", "dcterm:creator", "author"} {
		if value, ok := values[name]; ok {
			metadataByline = value
			break
		}
	}

	metadataExcerpt := ""
	for _, name := range []string{
		"dc:description", "dcterm:description", "og:description",
		"weibo:article:description", "weibo:webpage:description",
		"description", "twitter:description",
	} {
		if value, ok := values[name]; ok {
			metadataExcerpt = value
			break
		}
	}

	metadataSiteName := values["og:site_name"]
	metadataImage := ""
	for _, name := range []string{"og:image", "image", "twitter:image"} {
		if value, ok := values[name]; ok {
			metadataImage = toAbsoluteURI(value, r.documentURI)
			break
		}
	}

	metadataFavicon := r.getArticleFavicon()

	return Article{
		Title:    metadataTitle,
		Byline:   metadataByline,
		Excerpt:  metadataExcerpt,
		SiteName: metadataSiteName,
		Image:    metadataImage,
		Favicon:  metadataFavicon,
	}
}

func (r *Readability) prepArticle(articleContent *html.Node) {
	r.cleanStyles(articleContent)
	r.markDataTables(articleContent)
	r.cleanConditionally(articleContent, "form")
	r.cleanConditionally(articleContent, "fieldset")
	r.clean(articleContent, "object")
	r.clean(articleContent, "embed")
	r.clean(articleContent, "footer")
	r.clean(articleContent, "link")
	r.clean(articleContent, "aside")

	r.forEachNode(children(articleContent), func(topCandidate *html.Node, _ int) {
		r.cleanMatchedNodes(topCandidate, func(node *html.Node, nodeClassID string) bool {
			return rxShare.MatchString(nodeClassID) && len(textContent(node)) < r.CharThresholds
		})
	})

	if h2s := getElementsByTagName(articleContent, "h2"); len(h2s) == 1 {
		h2 := h2s[0]
		h2Text := textContent(h2)
		lengthSimilarRate := float64(len(h2Text)-len(r.articleTitle)) / float64(len(r.articleTitle))
		if math.Abs(lengthSimilarRate) < 0.5 {
			titlesMatch := false
			if lengthSimilarRate > 0 {
				titlesMatch = strings.Contains(h2Text, r.articleTitle)
			} else {
				titlesMatch = strings.Contains(r.articleTitle, h2Text)
			}
			if titlesMatch {
				r.clean(articleContent, "h2")
			}
		}
	}

	r.clean(articleContent, "iframe")
	r.clean(articleContent, "input")
	r.clean(articleContent, "textarea")
	r.clean(articleContent, "select")
	r.clean(articleContent, "button")
	r.cleanHeaders(articleContent)
	r.cleanConditionally(articleContent, "table")
	r.cleanConditionally(articleContent, "ul")
	r.cleanConditionally(articleContent, "div")

	r.removeNodes(getElementsByTagName(articleContent, "p"), func(p *html.Node) bool {
		imgCount := len(getElementsByTagName(p, "img"))
		embedCount := len(getElementsByTagName(p, "embed"))
		objectCount := len(getElementsByTagName(p, "object"))
		iframeCount := len(getElementsByTagName(p, "iframe"))
		totalCount := imgCount + embedCount + objectCount + iframeCount
		return totalCount == 0 && r.getInnerText(p, false) == ""
	})

	r.forEachNode(getElementsByTagName(articleContent, "br"), func(br *html.Node, _ int) {
		next := r.nextElement(br.NextSibling)
		if next != nil && tagName(next) == "p" {
			br.Parent.RemoveChild(br)
		}
	})

	r.forEachNode(getElementsByTagName(articleContent, "table"), func(table *html.Node, _ int) {
		tbody := table
		if r.hasSingleTagInsideElement(table, "tbody") {
			tbody = firstElementChild(table)
		}
		if r.hasSingleTagInsideElement(tbody, "tr") {
			row := firstElementChild(tbody)
			if r.hasSingleTagInsideElement(row, "td") {
				cell := firstElementChild(row)
				newTag := "div"
				if r.everyNode(childNodes(cell), r.isPhrasingContent) {
					newTag = "p"
				}
				r.setNodeTag(cell, newTag)
				replaceNode(table, cell)
			}
		}
	})
}

func (r *Readability) grabArticle() *html.Node {
	for {
		doc := cloneNode(r.doc)
		var page *html.Node
		if nodes := getElementsByTagName(doc, "body"); len(nodes) > 0 {
			page = nodes[0]
		}
		if page == nil {
			return nil
		}

		var elementsToScore []*html.Node
		var node = documentElement(doc)

		for node != nil {
			matchString := className(node) + "\x20" + id(node)

			if !r.isProbablyVisible(node) {
				node = r.removeAndGetNext(node)
				continue
			}

			if r.checkByline(node, matchString) {
				node = r.removeAndGetNext(node)
				continue
			}

			nodeTagName := tagName(node)
			if r.flags.stripUnlikelys {
				if rxUnlikelyCandidates.MatchString(matchString) &&
					!rxOkMaybeItsACandidate.MatchString(matchString) &&
					!r.hasAncestorTag(node, "table", 3, nil) &&
					nodeTagName != "body" &&
					nodeTagName != "a" {
					node = r.removeAndGetNext(node)
					continue
				}
			}

			switch nodeTagName {
			case "div", "section", "header", "h1", "h2", "h3", "h4", "h5", "h6":
				if r.isElementWithoutContent(node) {
					node = r.removeAndGetNext(node)
					continue
				}
			}

			if indexOf(r.TagsToScore, nodeTagName) != -1 {
				elementsToScore = append(elementsToScore, node)
			}

			if nodeTagName == "div" {
				var p *html.Node
				childNode := node.FirstChild

				for childNode != nil {
					nextSibling := childNode.NextSibling
					if r.isPhrasingContent(childNode) {
						if p != nil {
							appendChild(p, childNode)
						} else if !r.isWhitespace(childNode) {
							p = createElement("p")
							appendChild(p, cloneNode(childNode))
							replaceNode(childNode, p)
						}
					} else if p != nil {
						for p.LastChild != nil && r.isWhitespace(p.LastChild) {
							p.RemoveChild(p.LastChild)
						}
						p = nil
					}
					childNode = nextSibling
				}

				if r.hasSingleTagInsideElement(node, "p") && r.getLinkDensity(node) < 0.25 {
					newNode := children(node)[0]
					replaceNode(node, newNode)
					node = newNode
					elementsToScore = append(elementsToScore, node)
				} else if !r.hasChildBlockElement(node) {
					r.setNodeTag(node, "p")
					elementsToScore = append(elementsToScore, node)
				}
			}

			node = r.getNextNode(node, false)
		}

		var candidates []*html.Node
		r.forEachNode(elementsToScore, func(elementToScore *html.Node, _ int) {
			if elementToScore.Parent == nil || tagName(elementToScore.Parent) == "" {
				return
			}
			innerText := r.getInnerText(elementToScore, true)
			if len(innerText) < 25 {
				return
			}
			ancestors := r.getNodeAncestors(elementToScore, 3)
			if len(ancestors) == 0 {
				return
			}
			contentScore := 1
			contentScore += strings.Count(innerText, ",")
			contentScore += int(math.Min(math.Floor(float64(len(innerText))/100.0), 3.0))

			r.forEachNode(ancestors, func(ancestor *html.Node, level int) {
				if tagName(ancestor) == "" || ancestor.Parent == nil || ancestor.Parent.Type != html.ElementNode {
					return
				}
				if !r.hasContentScore(ancestor) {
					r.initializeNode(ancestor)
					candidates = append(candidates, ancestor)
				}
				scoreDivider := 1
				switch level {
				case 0:
					scoreDivider = 1
				case 1:
					scoreDivider = 2
				default:
					scoreDivider = level * 3
				}
				ancestorScore := r.getContentScore(ancestor)
				ancestorScore += float64(contentScore) / float64(scoreDivider)
				r.setContentScore(ancestor, ancestorScore)
			})
		})

		for i := 0; i < len(candidates); i++ {
			candidate := candidates[i]
			candidateScore := r.getContentScore(candidate) * (1 - r.getLinkDensity(candidate))
			r.setContentScore(candidate, candidateScore)
		}

		sort.Slice(candidates, func(i int, j int) bool {
			return r.getContentScore(candidates[i]) > r.getContentScore(candidates[j])
		})

		var topCandidates []*html.Node
		if len(candidates) > r.NTopCandidates {
			topCandidates = candidates[:r.NTopCandidates]
		} else {
			topCandidates = candidates
		}

		var topCandidate, parentOfTopCandidate *html.Node
		neededToCreateTopCandidate := false
		if len(topCandidates) > 0 {
			topCandidate = topCandidates[0]
		}

		if topCandidate == nil || tagName(topCandidate) == "body" {
			topCandidate = createElement("div")
			neededToCreateTopCandidate = true
			kids := childNodes(page)
			for i := 0; i < len(kids); i++ {
				appendChild(topCandidate, kids[i])
			}
			appendChild(page, topCandidate)
			r.initializeNode(topCandidate)
		} else if topCandidate != nil {
			topCandidateScore := r.getContentScore(topCandidate)
			var alternativeCandidateAncestors [][]*html.Node
			for i := 1; i < len(topCandidates); i++ {
				if r.getContentScore(topCandidates[i])/topCandidateScore >= 0.75 {
					topCandidateAncestors := r.getNodeAncestors(topCandidates[i], 0)
					alternativeCandidateAncestors = append(alternativeCandidateAncestors, topCandidateAncestors)
				}
			}

			minimumTopCandidates := 3
			if len(alternativeCandidateAncestors) >= minimumTopCandidates {
				parentOfTopCandidate = topCandidate.Parent
				for parentOfTopCandidate != nil && tagName(parentOfTopCandidate) != "body" {
					listContainingThisAncestor := 0
					for ancestorIndex := 0; ancestorIndex < len(alternativeCandidateAncestors) && listContainingThisAncestor < minimumTopCandidates; ancestorIndex++ {
						if includeNode(alternativeCandidateAncestors[ancestorIndex], parentOfTopCandidate) {
							listContainingThisAncestor++
						}
					}
					if listContainingThisAncestor >= minimumTopCandidates {
						topCandidate = parentOfTopCandidate
						break
					}
					parentOfTopCandidate = parentOfTopCandidate.Parent
				}
			}

			if !r.hasContentScore(topCandidate) {
				r.initializeNode(topCandidate)
			}

			parentOfTopCandidate = topCandidate.Parent
			lastScore := r.getContentScore(topCandidate)
			scoreThreshold := lastScore / 3.0
			for parentOfTopCandidate != nil && tagName(parentOfTopCandidate) != "body" {
				if !r.hasContentScore(parentOfTopCandidate) {
					parentOfTopCandidate = parentOfTopCandidate.Parent
					continue
				}
				parentScore := r.getContentScore(parentOfTopCandidate)
				if parentScore < scoreThreshold {
					break
				}
				if parentScore > lastScore {
					topCandidate = parentOfTopCandidate
					break
				}
				lastScore = parentScore
				parentOfTopCandidate = parentOfTopCandidate.Parent
			}

			parentOfTopCandidate = topCandidate.Parent
			for parentOfTopCandidate != nil && tagName(parentOfTopCandidate) != "body" && len(children(parentOfTopCandidate)) == 1 {
				topCandidate = parentOfTopCandidate
				parentOfTopCandidate = topCandidate.Parent
			}

			if !r.hasContentScore(topCandidate) {
				r.initializeNode(topCandidate)
			}
		}

		articleContent := createElement("div")
		siblingScoreThreshold := math.Max(10, r.getContentScore(topCandidate)*0.2)
		topCandidateScore := r.getContentScore(topCandidate)
		topCandidateClassName := className(topCandidate)

		parentOfTopCandidate = topCandidate.Parent
		siblings := children(parentOfTopCandidate)
		for s := 0; s < len(siblings); s++ {
			sibling := siblings[s]
			appendNode := false

			if sibling == topCandidate {
				appendNode = true
			} else {
				contentBonus := float64(0)
				if className(sibling) == topCandidateClassName && topCandidateClassName != "" {
					contentBonus += topCandidateScore * 0.2
				}
				if r.hasContentScore(sibling) && r.getContentScore(sibling)+contentBonus >= siblingScoreThreshold {
					appendNode = true
				} else if tagName(sibling) == "p" {
					linkDensity := r.getLinkDensity(sibling)
					nodeContent := r.getInnerText(sibling, true)
					nodeLength := len(nodeContent)
					if nodeLength > 80 && linkDensity < 0.25 {
						appendNode = true
					} else if nodeLength < 80 && nodeLength > 0 && linkDensity == 0 &&
						rxSentencePeriod.MatchString(nodeContent) {
						appendNode = true
					}
				}
			}

			if appendNode {
				if indexOf(alterToDivExceptions, tagName(sibling)) == -1 {
					r.setNodeTag(sibling, "div")
				}
				appendChild(articleContent, sibling)
			}
		}

		r.prepArticle(articleContent)

		if neededToCreateTopCandidate {
			firstChild := firstElementChild(articleContent)
			if firstChild != nil && tagName(firstChild) == "div" {
				setAttribute(firstChild, "id", "readability-page-1")
				setAttribute(firstChild, "class", "page")
			}
		} else {
			div := createElement("div")
			setAttribute(div, "id", "readability-page-1")
			setAttribute(div, "class", "page")
			childs := childNodes(articleContent)
			for i := 0; i < len(childs); i++ {
				appendChild(div, childs[i])
			}
			appendChild(articleContent, div)
		}

		parseSuccessful := true
		textLength := len(r.getInnerText(articleContent, true))
		if textLength < r.CharThresholds {
			parseSuccessful = false
			if r.flags.stripUnlikelys {
				r.flags.stripUnlikelys = false
				r.attempts = append(r.attempts, parseAttempt{articleContent: articleContent, textLength: textLength})
			} else if r.flags.useWeightClasses {
				r.flags.useWeightClasses = false
				r.attempts = append(r.attempts, parseAttempt{articleContent: articleContent, textLength: textLength})
			} else if r.flags.cleanConditionally {
				r.flags.cleanConditionally = false
				r.attempts = append(r.attempts, parseAttempt{articleContent: articleContent, textLength: textLength})
			} else {
				r.attempts = append(r.attempts, parseAttempt{articleContent: articleContent, textLength: textLength})
				sort.Slice(r.attempts, func(i, j int) bool {
					return r.attempts[i].textLength > r.attempts[j].textLength
				})
				if r.attempts[0].textLength == 0 {
					return nil
				}
				articleContent = r.attempts[0].articleContent
				parseSuccessful = true
			}
		}

		if parseSuccessful {
			return articleContent
		}
	}
}

func (r *Readability) initializeNode(node *html.Node) {
	contentScore := float64(r.getClassWeight(node))
	switch tagName(node) {
	case "div":
		contentScore += 5
	case "pre", "td", "blockquote":
		contentScore += 3
	case "address", "ol", "ul", "dl", "dd", "dt", "li", "form":
		contentScore -= 3
	case "h1", "h2", "h3", "h4", "h5", "h6", "th":
		contentScore -= 5
	}
	r.setContentScore(node, contentScore)
}

func (r *Readability) removeAndGetNext(node *html.Node) *html.Node {
	nextNode := r.getNextNode(node, true)
	if node.Parent != nil {
		node.Parent.RemoveChild(node)
	}
	return nextNode
}

func (r *Readability) getNextNode(node *html.Node, ignoreSelfAndKids bool) *html.Node {
	if firstChild := firstElementChild(node); !ignoreSelfAndKids && firstChild != nil {
		return firstChild
	}
	if sibling := nextElementSibling(node); sibling != nil {
		return sibling
	}
	for {
		node = node.Parent
		if node == nil || nextElementSibling(node) != nil {
			break
		}
	}
	if node != nil {
		return nextElementSibling(node)
	}
	return nil
}

func (r *Readability) isValidByline(byline string) bool {
	byline = strings.TrimSpace(byline)
	return len(byline) > 0 && len(byline) < 100
}

func (r *Readability) checkByline(node *html.Node, matchString string) bool {
	if r.articleByline != "" {
		return false
	}
	rel := getAttribute(node, "rel")
	itemprop := getAttribute(node, "itemprop")
	nodeText := textContent(node)
	if (rel == "author" || strings.Contains(itemprop, "author") || rxByline.MatchString(matchString)) && r.isValidByline(nodeText) {
		nodeText = strings.TrimSpace(nodeText)
		nodeText = strings.Join(strings.Fields(nodeText), "\x20")
		r.articleByline = nodeText
		return true
	}
	return false
}

func (r *Readability) getNodeAncestors(node *html.Node, maxDepth int) []*html.Node {
	level := 0
	ancestors := []*html.Node{}
	for node.Parent != nil {
		level++
		ancestors = append(ancestors, node.Parent)
		if maxDepth > 0 && level == maxDepth {
			break
		}
		node = node.Parent
	}
	return ancestors
}

func (r *Readability) setContentScore(node *html.Node, score float64) {
	setAttribute(node, "data-readability-score", fmt.Sprintf("%.4f", score))
}

func (r *Readability) hasContentScore(node *html.Node) bool {
	return hasAttribute(node, "data-readability-score")
}

func (r *Readability) getContentScore(node *html.Node) float64 {
	strScore := getAttribute(node, "data-readability-score")
	strScore = strings.TrimSpace(strScore)
	if strScore == "" {
		return 0
	}
	score, err := strconv.ParseFloat(strScore, 64)
	if err != nil {
		return 0
	}
	return score
}

func (r *Readability) removeScripts(doc *html.Node) {
	r.removeNodes(getElementsByTagName(doc, "script"), nil)
	r.removeNodes(getElementsByTagName(doc, "noscript"), nil)
}

func (r *Readability) hasSingleTagInsideElement(element *html.Node, tag string) bool {
	if childs := children(element); len(childs) != 1 || tagName(childs[0]) != tag {
		return false
	}
	return !r.someNode(childNodes(element), func(node *html.Node) bool {
		return node.Type == html.TextNode && rxHasContent.MatchString(textContent(node))
	})
}

func (r *Readability) isElementWithoutContent(node *html.Node) bool {
	brs := getElementsByTagName(node, "br")
	hrs := getElementsByTagName(node, "hr")
	childs := children(node)
	return node.Type == html.ElementNode &&
		strings.TrimSpace(textContent(node)) == "" &&
		(len(childs) == 0 || len(childs) == len(brs)+len(hrs))
}

func (r *Readability) hasChildBlockElement(element *html.Node) bool {
	return r.someNode(childNodes(element), func(node *html.Node) bool {
		return indexOf(divToPElems, tagName(node)) != -1 ||
			r.hasChildBlockElement(node)
	})
}

func (r *Readability) isPhrasingContent(node *html.Node) bool {
	if node.Type == html.TextNode {
		return true
	}
	tag := tagName(node)
	if indexOf(phrasingElems, tag) != -1 {
		return true
	}
	return ((tag == "a" || tag == "del" || tag == "ins") &&
		r.everyNode(childNodes(node), r.isPhrasingContent))
}

func (r *Readability) isWhitespace(node *html.Node) bool {
	return (node.Type == html.TextNode && strings.TrimSpace(textContent(node)) == "") ||
		(node.Type == html.ElementNode && tagName(node) == "br")
}

func (r *Readability) getInnerText(node *html.Node, normalizeSpaces bool) string {
	textContent := strings.TrimSpace(textContent(node))
	if normalizeSpaces {
		textContent = rxNormalize.ReplaceAllString(textContent, "\x20")
	}
	return textContent
}

func (r *Readability) getCharCount(node *html.Node, s string) int {
	innerText := r.getInnerText(node, true)
	return strings.Count(innerText, s)
}

func (r *Readability) cleanStyles(node *html.Node) {
	nodeTagName := tagName(node)
	if node == nil || nodeTagName == "svg" {
		return
	}
	for i := 0; i < len(presentationalAttributes); i++ {
		removeAttribute(node, presentationalAttributes[i])
	}
	if indexOf(deprecatedSizeAttributeElems, nodeTagName) != -1 {
		removeAttribute(node, "width")
		removeAttribute(node, "height")
	}
	for child := firstElementChild(node); child != nil; child = nextElementSibling(child) {
		r.cleanStyles(child)
	}
}

func (r *Readability) getLinkDensity(element *html.Node) float64 {
	textLength := len(r.getInnerText(element, true))
	if textLength == 0 {
		return 0
	}
	linkLength := 0
	r.forEachNode(getElementsByTagName(element, "a"), func(linkNode *html.Node, _ int) {
		linkLength += len(r.getInnerText(linkNode, true))
	})
	return float64(linkLength) / float64(textLength)
}

func (r *Readability) getClassWeight(node *html.Node) int {
	if !r.flags.useWeightClasses {
		return 0
	}
	weight := 0
	if nodeClassName := className(node); nodeClassName != "" {
		if rxNegative.MatchString(nodeClassName) {
			weight -= 25
		}
		if rxPositive.MatchString(nodeClassName) {
			weight += 25
		}
	}
	if nodeID := id(node); nodeID != "" {
		if rxNegative.MatchString(nodeID) {
			weight -= 25
		}
		if rxPositive.MatchString(nodeID) {
			weight += 25
		}
	}
	return weight
}

func (r *Readability) clean(node *html.Node, tag string) {
	isEmbed := indexOf([]string{"object", "embed", "iframe"}, tag) != -1
	r.removeNodes(getElementsByTagName(node, tag), func(element *html.Node) bool {
		if isEmbed {
			for _, attr := range element.Attr {
				if rxVideos.MatchString(attr.Val) {
					return false
				}
			}
			if tagName(element) == "object" && rxVideos.MatchString(innerHTML(element)) {
				return false
			}
		}
		return true
	})
}

func (r *Readability) hasAncestorTag(node *html.Node, tag string, maxDepth int, filterFn func(*html.Node) bool) bool {
	depth := 0
	for node.Parent != nil {
		if maxDepth > 0 && depth > maxDepth {
			return false
		}
		if tagName(node.Parent) == tag && (filterFn == nil || filterFn(node.Parent)) {
			return true
		}
		node = node.Parent
		depth++
	}
	return false
}

func (r *Readability) getRowAndColumnCount(table *html.Node) (int, int) {
	rows := 0
	columns := 0
	trs := getElementsByTagName(table, "tr")
	for i := 0; i < len(trs); i++ {
		strRowSpan := getAttribute(trs[i], "rowspan")
		rowSpan, _ := strconv.Atoi(strRowSpan)
		if rowSpan == 0 {
			rowSpan = 1
		}
		rows += rowSpan
		columnsInThisRow := 0
		cells := getElementsByTagName(trs[i], "td")
		for j := 0; j < len(cells); j++ {
			strColSpan := getAttribute(cells[j], "colspan")
			colSpan, _ := strconv.Atoi(strColSpan)
			if colSpan == 0 {
				colSpan = 1
			}
			columnsInThisRow += colSpan
		}
		if columnsInThisRow > columns {
			columns = columnsInThisRow
		}
	}
	return rows, columns
}

func (r *Readability) isReadabilityDataTable(node *html.Node) bool {
	return hasAttribute(node, "data-readability-table")
}

func (r *Readability) setReadabilityDataTable(node *html.Node, isDataTable bool) {
	if isDataTable {
		setAttribute(node, "data-readability-table", "true")
		return
	}
	removeAttribute(node, "data-readability-table")
}

func (r *Readability) markDataTables(root *html.Node) {
	tables := getElementsByTagName(root, "table")
	for i := 0; i < len(tables); i++ {
		table := tables[i]
		role := getAttribute(table, "role")
		if role == "presentation" {
			r.setReadabilityDataTable(table, false)
			continue
		}
		datatable := getAttribute(table, "datatable")
		if datatable == "0" {
			r.setReadabilityDataTable(table, false)
			continue
		}
		if hasAttribute(table, "summary") {
			r.setReadabilityDataTable(table, true)
			continue
		}
		if captions := getElementsByTagName(table, "caption"); len(captions) > 0 {
			if caption := captions[0]; caption != nil && len(childNodes(caption)) > 0 {
				r.setReadabilityDataTable(table, true)
				continue
			}
		}
		hasDataTableDescendantTags := false
		for _, descendantTag := range []string{"col", "colgroup", "tfoot", "thead", "th"} {
			descendants := getElementsByTagName(table, descendantTag)
			if len(descendants) > 0 && descendants[0] != nil {
				hasDataTableDescendantTags = true
				break
			}
		}
		if hasDataTableDescendantTags {
			r.setReadabilityDataTable(table, true)
			continue
		}
		if len(getElementsByTagName(table, "table")) > 0 {
			r.setReadabilityDataTable(table, false)
			continue
		}
		rows, columns := r.getRowAndColumnCount(table)
		if rows >= 10 || columns > 4 {
			r.setReadabilityDataTable(table, true)
			continue
		}
		if rows*columns > 10 {
			r.setReadabilityDataTable(table, true)
		}
	}
}

func (r *Readability) cleanConditionally(element *html.Node, tag string) {
	if !r.flags.cleanConditionally {
		return
	}
	isList := tag == "ul" || tag == "ol"
	r.removeNodes(getElementsByTagName(element, tag), func(node *html.Node) bool {
		if tag == "table" && r.isReadabilityDataTable(node) {
			return false
		}
		if r.hasAncestorTag(node, "table", -1, r.isReadabilityDataTable) {
			return false
		}
		weight := r.getClassWeight(node)
		if weight < 0 {
			return true
		}
		if r.getCharCount(node, ",") < 10 {
			p := float64(len(getElementsByTagName(node, "p")))
			img := float64(len(getElementsByTagName(node, "img")))
			li := float64(len(getElementsByTagName(node, "li")) - 100)
			input := float64(len(getElementsByTagName(node, "input")))
			embedCount := 0
			embeds := r.concatNodeLists(
				getElementsByTagName(node, "object"),
				getElementsByTagName(node, "embed"),
				getElementsByTagName(node, "iframe"),
			)
			for _, embed := range embeds {
				for _, attr := range embed.Attr {
					if rxVideos.MatchString(attr.Val) {
						return false
					}
				}
				if tagName(embed) == "object" && rxVideos.MatchString(innerHTML(embed)) {
					return false
				}
				embedCount++
			}
			linkDensity := r.getLinkDensity(node)
			contentLength := len(r.getInnerText(node, true))
			return (img > 1 && p/img < 0.5 && !r.hasAncestorTag(node, "figure", 3, nil)) ||
				(!isList && li > p) ||
				(input > math.Floor(p/3)) ||
				(!isList && contentLength < 25 && (img == 0 || img > 2) && !r.hasAncestorTag(node, "figure", 3, nil)) ||
				(!isList && weight < 25 && linkDensity > 0.2) ||
				(weight >= 25 && linkDensity > 0.5) ||
				((embedCount == 1 && contentLength < 75) || embedCount > 1)
		}
		return false
	})
}

func (r *Readability) cleanMatchedNodes(e *html.Node, filter func(*html.Node, string) bool) {
	endOfSearchMarkerNode := r.getNextNode(e, true)
	next := r.getNextNode(e, false)
	for next != nil && next != endOfSearchMarkerNode {
		if filter != nil && filter(next, className(next)+"\x20"+id(next)) {
			next = r.removeAndGetNext(next)
		} else {
			next = r.getNextNode(next, false)
		}
	}
}

func (r *Readability) cleanHeaders(e *html.Node) {
	for headerIndex := 1; headerIndex < 3; headerIndex++ {
		headerTag := fmt.Sprintf("h%d", headerIndex)
		r.removeNodes(getElementsByTagName(e, headerTag), func(header *html.Node) bool {
			return r.getClassWeight(header) < 0
		})
	}
}

func (r *Readability) isProbablyVisible(node *html.Node) bool {
	nodeStyle := getAttribute(node, "style")
	nodeAriaHidden := getAttribute(node, "aria-hidden")
	className := getAttribute(node, "class")
	return (nodeStyle == "" || !rxDisplayNone.MatchString(nodeStyle)) &&
		!hasAttribute(node, "hidden") &&
		(nodeAriaHidden == "" ||
			nodeAriaHidden != "true" ||
			strings.Contains(className, "fallback-image"))
}

func (r *Readability) fixRelativeURIs(articleContent *html.Node) {
	links := r.getAllNodesWithTag(articleContent, "a")
	r.forEachNode(links, func(link *html.Node, _ int) {
		href := getAttribute(link, "href")
		if href == "" {
			return
		}
		if strings.HasPrefix(href, "javascript:") {
			text := createTextNode(textContent(link))
			replaceNode(link, text)
			return
		}
		newHref := toAbsoluteURI(href, r.documentURI)
		if newHref == "" {
			removeAttribute(link, "href")
			return
		}
		setAttribute(link, "href", newHref)
	})
	imgs := r.getAllNodesWithTag(articleContent, "img")
	r.forEachNode(imgs, func(img *html.Node, _ int) {
		src := getAttribute(img, "src")
		if src == "" {
			return
		}
		newSrc := toAbsoluteURI(src, r.documentURI)
		if newSrc == "" {
			removeAttribute(img, "src")
			return
		}
		setAttribute(img, "src", newSrc)
	})
}

func (r *Readability) cleanClasses(node *html.Node) {
	nodeClassName := className(node)
	preservedClassName := []string{}
	for _, class := range strings.Fields(nodeClassName) {
		if indexOf(r.ClassesToPreserve, class) != -1 {
			preservedClassName = append(preservedClassName, class)
		}
	}
	if len(preservedClassName) > 0 {
		setAttribute(node, "class", strings.Join(preservedClassName, "\x20"))
	} else {
		removeAttribute(node, "class")
	}
	for child := firstElementChild(node); child != nil; child = nextElementSibling(child) {
		r.cleanClasses(child)
	}
}

func (r *Readability) clearReadabilityAttr(node *html.Node) {
	removeAttribute(node, "data-readability-score")
	removeAttribute(node, "data-readability-table")
	for child := firstElementChild(node); child != nil; child = nextElementSibling(child) {
		r.clearReadabilityAttr(child)
	}
}

func (r *Readability) isSingleImage(node *html.Node) bool {
	if tagName(node) == "img" {
		return true
	}
	children := children(node)
	textContent := textContent(node)
	if len(children) != 1 || strings.TrimSpace(textContent) != "" {
		return false
	}
	return r.isSingleImage(children[0])
}

func (r *Readability) removeComments(doc *html.Node) {
	var comments []*html.Node
	var finder func(*html.Node)
	finder = func(node *html.Node) {
		if node.Type == html.CommentNode {
			comments = append(comments, node)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			finder(child)
		}
	}
	for child := doc.FirstChild; child != nil; child = child.NextSibling {
		finder(child)
	}
	r.removeNodes(comments, nil)
}

func (r *Readability) postProcessContent(articleContent *html.Node) {
	r.fixRelativeURIs(articleContent)
	r.cleanClasses(articleContent)
	r.clearReadabilityAttr(articleContent)
}

// Parse parses input and find the main readable content.
func (r *Readability) Parse(input io.Reader, pageURL string) (Article, error) {
	var err error
	r.articleTitle = ""
	r.articleByline = ""
	r.attempts = []parseAttempt{}
	r.flags.stripUnlikelys = true
	r.flags.useWeightClasses = true
	r.flags.cleanConditionally = true

	if r.documentURI, err = url.ParseRequestURI(pageURL); err != nil {
		return Article{}, fmt.Errorf("failed to parse URL: %v", err)
	}

	if r.doc, err = html.Parse(input); err != nil {
		return Article{}, fmt.Errorf("failed to parse input: %v", err)
	}

	if r.MaxElemsToParse > 0 {
		numTags := len(getElementsByTagName(r.doc, "*"))
		if numTags > r.MaxElemsToParse {
			return Article{}, fmt.Errorf("too many elements: %d", numTags)
		}
	}

	r.removeScripts(r.doc)
	r.prepDocument()

	metadata := r.getArticleMetadata()
	r.articleTitle = metadata.Title

	finalHTMLContent := ""
	finalTextContent := ""
	readableNode := &html.Node{}
	articleContent := r.grabArticle()

	if articleContent != nil {
		r.postProcessContent(articleContent)

		if metadata.Excerpt == "" {
			paragraphs := getElementsByTagName(articleContent, "p")
			if len(paragraphs) > 0 {
				metadata.Excerpt = strings.TrimSpace(textContent(paragraphs[0]))
			}
		}

		readableNode = firstElementChild(articleContent)
		finalHTMLContent = innerHTML(articleContent)
		finalTextContent = textContent(articleContent)
		finalTextContent = strings.TrimSpace(finalTextContent)
	}

	finalByline := metadata.Byline
	if finalByline == "" {
		finalByline = r.articleByline
	}

	return Article{
		Title:       r.articleTitle,
		Byline:      finalByline,
		Node:        readableNode,
		Content:     finalHTMLContent,
		TextContent: finalTextContent,
		Length:      len(finalTextContent),
		Excerpt:     metadata.Excerpt,
		SiteName:    metadata.SiteName,
		Image:       metadata.Image,
		Favicon:     metadata.Favicon,
	}, nil
}

// IsReadable decides whether the document is usable or not without parsing the whole thing.
func (r *Readability) IsReadable(input io.Reader) bool {
	doc, err := html.Parse(input)
	if err != nil {
		return false
	}
	nodeList := make([]*html.Node, 0)
	nodeDict := make(map[*html.Node]struct{})
	var finder func(*html.Node)
	finder = func(node *html.Node) {
		if node.Type == html.ElementNode {
			tag := tagName(node)
			if tag == "p" || tag == "pre" {
				if _, exist := nodeDict[node]; !exist {
					nodeList = append(nodeList, node)
					nodeDict[node] = struct{}{}
				}
			} else if tag == "br" && node.Parent != nil && tagName(node.Parent) == "div" {
				if _, exist := nodeDict[node.Parent]; !exist {
					nodeList = append(nodeList, node.Parent)
					nodeDict[node.Parent] = struct{}{}
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			finder(child)
		}
	}
	finder(doc)

	score := float64(0)
	return r.someNode(nodeList, func(node *html.Node) bool {
		if !r.isProbablyVisible(node) {
			return false
		}
		matchString := className(node) + "\x20" + id(node)
		if rxUnlikelyCandidates.MatchString(matchString) &&
			!rxOkMaybeItsACandidate.MatchString(matchString) {
			return false
		}
		if tagName(node) == "p" && r.hasAncestorTag(node, "li", -1, nil) {
			return false
		}
		nodeText := strings.TrimSpace(textContent(node))
		nodeTextLength := len(nodeText)
		if nodeTextLength < 140 {
			return false
		}
		score += math.Sqrt(float64(nodeTextLength - 140))
		if score > 20 {
			return true
		}
		return false
	})
}

// ─── Noise Pattern Data ─────────────────────────────────────────────────────

var noisePatterns = []string{
	"sidebar", "table-of-contents", "tableofcontents", "infobox", "navbox",
	"nav-box", "navigation", "breadcrumb", "cookie", "consent", "banner",
	"disqus", "advert", "popup", "modal", "subscribe",
	"printfooter", "catlinks", "mw-panel", "mw-navigation", "sitesub",
	"jump-to-nav", "mw-editsection", "reflist", "mw-references",
	"authority-control", "mw-indicators", "sistersitebox", "mbox", "ambox",
	"ombox", "hatnote", "shortdescription", "sphinxsidebar", "sphinxfooter",
	"copyright", "dropdown", "city-selector", "location-selector",
	"lang-selector", "language-selector", "skip-to", "skip-link", "skiplinks",
	"promo", "promotional", "widget", "widgets",
	"site-footer", "site-header", "page-footer", "page-header",
	"global-nav", "global-footer", "global-header", "main-nav",
	"primary-nav", "secondary-nav", "social-share", "social-links",
	"social-icons", "follow-us", "site-map", "sitemap",
	"playground", "railway", "up.railway", "external-link",
	"sharing", "share", "follow-btn", "follow-container", "btn-follow",
	"btn-fab", "like-btn", "like-btn", "article-tags",
	"article-tags-list", "article-tags-element", "tags-link",
	"option-btn", "btn-share", "sharing-btn", "w-sharing",
	"expand-extra-info", "extra-content-cta",
	"main-icon", "icon", "fab-label", "icon-xlarge", "icon-large",
	"i-follow", "i-share", "i-facebook", "i-x", "i-whatsapp",
	"i-threads", "i-bluesky", "i-linkedin", "i-reddit", "i-flipboard",
	"is-hidden", "article-options",
	"article-topics", "articleTopics", "articleTopicsRow", "linkButtonRow",
	"reader-comments", "article-reader-comments",
	"add-comment", "comment-form", "commentForm", "house-rules",
	"most-read", "most-watched", "dont-miss", "don't miss",
	"taboola", "ez-modal", "modal-",
	"column-content",
}

var noiseExactTokens = []string{
	"toc", "share", "social", "related", "recommended",
}

var noisePrefixes = []string{"ad-", "ads-"}

// ─── Regex Patterns ─────────────────────────────────────────────────────────

var (
	scriptRe            = regexp.MustCompile(`(?i)<script[^>]*>.*?</script>`)
	styleRe             = regexp.MustCompile(`(?i)<style[^>]*>.*?</style>`)
	noScriptRe          = regexp.MustCompile(`(?i)<noscript[^>]*>.*?</noscript>`)
	iframeRe            = regexp.MustCompile(`(?i)<iframe[^>]*>.*?</iframe>`)
	svgRe               = regexp.MustCompile(`(?i)<svg[^>]*>.*?</svg>`)
	dataImgRe           = regexp.MustCompile(`(?i)<img[^>]*src=["']data:[^"']*["'][^>]*>`)
	urlTextRe           = regexp.MustCompile(`(?i)(?:https?://|www\.)[^\s<>"']+\.[a-z]{2,}[^\s<>"']*`)
	buttonRe            = regexp.MustCompile(`(?si)<button[^>]*>.*?</button>`)
	whitespaceRe        = regexp.MustCompile(`[ \t]{2,}`)
	newlineRe           = regexp.MustCompile(`\n\s*\n\s*\n+`)
	emptyDivRe          = regexp.MustCompile(`(?si)<div[^>]*>\s*</div>`)
	emptySpanRe         = regexp.MustCompile(`(?si)<span[^>]*>\s*</span>`)
	imgRe               = regexp.MustCompile(`(?i)<img[^>]*>`)
	anchorRe            = regexp.MustCompile(`(?i)<a[^>]*>.*?</a>`)
	anchorSelfClosingRe = regexp.MustCompile(`(?i)<a[^>]*>`)
)

// ─── HTMLPreprocessOptions ─────────────────────────────────────────────────

type HTMLPreprocessOptions struct {
	IncludeTags       []string
	ExcludeTags       []string
	CSSSelector       *string
	BypassFilters     bool
	SkipNoisePatterns bool
}

// ─── Public API ─────────────────────────────────────────────────────────────

// ExtractHTML extracts the main article content along with title and excerpt.
func ExtractHTML(html string) ExtractedHTML {
	cleaned := cleanNoise(html)
	extracted := extractWithReadability(cleaned)
	post := postprocessHTML(extracted.Content)
	combined := formatHTMLWithMetadata(extracted.Title, extracted.Excerpt, post)

	return ExtractedHTML{
		Title:   extracted.Title,
		Excerpt: extracted.Excerpt,
		Content: combined,
	}
}

// ExtractMainContent extracts just the article content HTML.
func ExtractMainContent(html string) string {
	cleaned := cleanNoise(html)
	extracted := extractWithReadability(cleaned)
	post := postprocessHTML(extracted.Content)
	return post
}

func preprocessHTML(rawHTML string, opts HTMLPreprocessOptions) string {
	bodyHTML := stripDocumentHead(rawHTML)
	cleaned := cleanNoise(bodyHTML)

	if opts.BypassFilters {
		return cleaned
	}

	if len(opts.IncludeTags) > 0 {
		cleaned = filterBySelectors(cleaned, opts.IncludeTags)
	}
	if len(opts.ExcludeTags) > 0 {
		cleaned = removeElementsBySelectors(cleaned, opts.ExcludeTags)
	}

	if opts.CSSSelector != nil && *opts.CSSSelector != "" {
		cleaned = applyCSSSelector(cleaned, *opts.CSSSelector)
	}

	if !opts.SkipNoisePatterns {
		cleaned = applyNoisePatterns(cleaned)
	}

	return cleaned
}

func postprocessHTML(html string) string {
	if html == "" {
		return html
	}

	result := html
	result = imgRe.ReplaceAllString(result, "")
	result = anchorRe.ReplaceAllString(result, "")
	result = anchorSelfClosingRe.ReplaceAllString(result, "")

	for {
		before := result
		result = emptyDivRe.ReplaceAllString(result, "")
		result = emptySpanRe.ReplaceAllString(result, "")
		if result == before {
			break
		}
	}

	result = whitespaceRe.ReplaceAllString(result, " ")
	result = newlineRe.ReplaceAllString(result, "\n\n")

	return strings.TrimSpace(result)
}

func applyNoisePatterns(html string) string {
	if html == "" {
		return html
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return html
	}

	doc.Find("*").Each(func(_ int, s *goquery.Selection) {
		tagName := goquery.NodeName(s)
		if semanticTags[tagName] {
			return
		}

		classAttr, _ := s.Attr("class")
		idAttr, _ := s.Attr("id")
		roleAttr, _ := s.Attr("role")
		combined := strings.ToLower(classAttr + " " + idAttr)

		if isNoiseWrapper(combined, roleAttr) {
			s.Remove()
		}
	})

	result, _ := doc.Html()
	return result
}

func cleanNoise(html string) string {
	if html == "" {
		return html
	}

	result := html
	result = scriptRe.ReplaceAllString(result, "")
	result = styleRe.ReplaceAllString(result, "")
	result = noScriptRe.ReplaceAllString(result, "")
	result = iframeRe.ReplaceAllString(result, "")
	result = svgRe.ReplaceAllString(result, "")
	result = dataImgRe.ReplaceAllString(result, "")
	result = urlTextRe.ReplaceAllString(result, "")
	result = buttonRe.ReplaceAllString(result, "")

	return result
}

// ─── Internal Helpers ───────────────────────────────────────────────────────

var semanticTags = map[string]bool{
	"article": true, "section": true, "main": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"ul": true, "ol": true, "li": true, "dl": true, "dt": true, "dd": true,
	"p": true, "blockquote": true, "pre": true, "code": true,
	"table": true, "thead": true, "tbody": true, "tr": true, "td": true, "th": true,
	"strong": true, "em": true, "b": true, "i": true, "u": true,
	"figure": true, "figcaption": true, "picture": true,
	"label": true, "form": true, "input": true,
}

func stripDocumentHead(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return html
	}

	doc.Find("head").Remove()
	result, err := doc.Html()
	if err != nil {
		return html
	}

	return result
}

func isNoiseWrapper(attributes, role string) bool {
	for _, phrase := range noisePatterns {
		if strings.Contains(attributes, phrase) {
			return true
		}
	}

	tokens := strings.Fields(attributes)
	for _, token := range tokens {
		for _, exact := range noiseExactTokens {
			if token == exact {
				return true
			}
		}
		for _, prefix := range noisePrefixes {
			if strings.HasPrefix(token, prefix) {
				return true
			}
		}
	}

	if role != "" {
		switch strings.ToLower(role) {
		case "navigation", "banner", "contentinfo", "complementary":
			return true
		}
	}

	return false
}

func extractWithReadability(html string) ExtractedHTML {
	r := NewReadability()
	article, err := r.Parse(strings.NewReader(html), "https://example.com")
	if err != nil {
		return ExtractedHTML{Content: html}
	}
	return ExtractedHTML{
		Title:   article.Title,
		Excerpt: article.Excerpt,
		Content: article.Content,
	}
}

func filterBySelectors(html string, selectors []string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return html
	}

	var parts []string
	for _, sel := range selectors {
		doc.Find(sel).Each(func(_ int, s *goquery.Selection) {
			parts = append(parts, s.Text())
		})
	}

	if len(parts) == 0 {
		return html
	}
	return strings.Join(parts, "\n")
}

func removeElementsBySelectors(html string, selectors []string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return html
	}

	for _, sel := range selectors {
		doc.Find(sel).Each(func(_ int, s *goquery.Selection) {
			s.Remove()
		})
	}

	result, _ := doc.Html()
	return result
}

func applyCSSSelector(html, selector string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return ""
	}

	el := doc.Find(selector).First()
	if el.Length() > 0 {
		content, _ := el.Html()
		return content
	}
	return ""
}

func formatHTMLWithMetadata(title, excerpt, content string) string {
	var b strings.Builder

	b.WriteString("<html>\n  <body>\n")

	if title != "" {
		b.WriteString("    <h1>")
		b.WriteString(strings.TrimSpace(title))
		b.WriteString("</h1>\n")
	}

	if excerpt != "" {
		b.WriteString("    <p class=\"excerpt\">")
		b.WriteString(strings.TrimSpace(excerpt))
		b.WriteString("</p>\n")
	}

	if content != "" {
		b.WriteString(content)
		b.WriteString("\n")
	}

	b.WriteString("  </body>\n</html>")

	return b.String()
}
