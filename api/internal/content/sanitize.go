package content

import (
	"regexp"
	"sync"

	"github.com/microcosm-cc/bluemonday"
)

var (
	reHeadingID  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_\-:.]{0,99}$`)
	reUUID       = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	reLanguage   = regexp.MustCompile(`^language-[A-Za-z0-9_+#.\-]{1,40}$`)
	reBool       = regexp.MustCompile(`^(true|false)$`)
	reDimension  = regexp.MustCompile(`^[0-9]{1,5}(\.[0-9]{1,3})?(px|%)?$`)
	reSpan       = regexp.MustCompile(`^[0-9]{1,4}$`)
	reColWidth   = regexp.MustCompile(`^[0-9]{1,5}(,[0-9]{1,5}){0,63}$`)
	reRel        = regexp.MustCompile(`^(noopener|noreferrer|nofollow|ugc)( (noopener|noreferrer|nofollow|ugc)){0,3}$`)
	reTarget     = regexp.MustCompile(`^(_blank|_self)$`)
	reColorToken = regexp.MustCompile(`^(info|note|success|warning|error|grey|blue|green|yellow|red|purple)$`)
	reSpanType   = regexp.MustCompile(`^(mention|status|comment)$`)
	reDivType    = regexp.MustCompile(`^(panel|toc)$`)
	reTitleAlt   = regexp.MustCompile(`^.{0,500}$`)
	reAlign      = regexp.MustCompile(`^(left|center|right|justify)$`)
)

var (
	policyOnce sync.Once
	policy     *bluemonday.Policy
)

func Policy() *bluemonday.Policy {
	policyOnce.Do(func() { policy = buildPolicy() })
	return policy
}

func buildPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()

	p.AllowElements(
		"p", "h1", "h2", "h3", "h4", "h5", "h6",
		"ul", "ol", "li", "blockquote", "pre", "code",
		"strong", "em", "u", "s", "mark",
		"table", "thead", "tbody", "tr", "th", "td",
		"hr", "br", "summary",
	)
	p.AllowElements("a", "img", "span", "div", "details")

	p.AllowAttrs("data-align").Matching(reAlign).OnElements("p", "h1", "h2", "h3", "h4", "h5", "h6")
	p.AllowAttrs("id").Matching(reHeadingID).OnElements("h1", "h2", "h3", "h4", "h5", "h6")

	p.AllowAttrs("data-type").Matching(regexp.MustCompile(`^taskList$`)).OnElements("ul")
	p.AllowAttrs("data-type").Matching(regexp.MustCompile(`^taskItem$`)).OnElements("li")
	p.AllowAttrs("data-checked").Matching(reBool).OnElements("li")

	p.AllowAttrs("class").Matching(reLanguage).OnElements("code")

	p.AllowAttrs("data-type").Matching(regexp.MustCompile(`^page-link$`)).OnElements("a")
	p.AllowAttrs("data-page-id").Matching(reUUID).OnElements("a")
	p.AllowAttrs("href").OnElements("a")
	p.AllowAttrs("rel").Matching(reRel).OnElements("a")
	p.AllowAttrs("target").Matching(reTarget).OnElements("a")

	p.AllowAttrs("src").OnElements("img")
	p.AllowAttrs("alt", "title").Matching(reTitleAlt).OnElements("img")
	p.AllowAttrs("width", "height").Matching(reDimension).OnElements("img")
	p.AllowAttrs("data-attachment-id").Matching(reUUID).OnElements("img")

	p.AllowAttrs("colspan", "rowspan").Matching(reSpan).OnElements("th", "td")
	p.AllowAttrs("colwidth").Matching(reColWidth).OnElements("th", "td")

	p.AllowAttrs("data-type").Matching(reSpanType).OnElements("span")
	p.AllowAttrs("data-id").Matching(reUUID).OnElements("span")
	p.AllowAttrs("data-color").Matching(reColorToken).OnElements("span")

	p.AllowAttrs("data-type").Matching(reDivType).OnElements("div")
	p.AllowAttrs("data-color").Matching(reColorToken).OnElements("div")

	p.AllowAttrs("data-type").Matching(regexp.MustCompile(`^expand$`)).OnElements("details")

	p.AllowURLSchemes("http", "https", "mailto")
	p.AllowRelativeURLs(true)
	p.RequireParseableURLs(true)

	return p
}

func SanitizeHTML(html string) string {
	return Policy().Sanitize(html)
}
