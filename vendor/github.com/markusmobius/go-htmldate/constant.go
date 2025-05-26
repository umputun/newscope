// Copyright (C) 2022 Markus Mobius
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Code in this file is ported from <https://github.com/adbar/htmldate>
// which available under Apache 2.0 license.

package htmldate

import (
	"fmt"
	"regexp"
	"time"

	dps "github.com/markusmobius/go-dateparser"
	"github.com/markusmobius/go-htmldate/internal/re2go"
)

type fnRe2GoFinder func(string) [][]int

var (
	timeZero       = time.Time{}
	defaultMinDate = time.Date(1995, 1, 1, 0, 0, 0, 0, time.UTC)
	defaultMaxDate = time.Now().AddDate(1, 0, 0)

	externalParser = &dps.Parser{
		ParserTypes: []dps.ParserType{
			dps.CustomFormat,
			dps.AbsoluteTime,
		},
	}

	externalDpsConfig = &dps.Configuration{
		// DateOrder:           dps.DMY,
		// PreferredDayOfMonth: dps.First,
		PreferredDateSource: dps.Past,
		StrictParsing:       true,
	}
)

const (
	minSegmentLen         = 6
	maxSegmentLen         = 52
	maxPossibleCandidates = 1_000
	defaultDateFormat     = "2006-1-2"
)

var (
	rxLastNonDigits = regexp.MustCompile(`\D+$`)

	rxDay   = `[0-3]?[0-9]`
	rxMonth = `[0-1]?[0-9]`
	rxYear  = `199[0-9]|20[0-3][0-9]`

	rxYmdNoSepPattern = regexp.MustCompile(`(?:\D|^)(\d{8})(?:\D|$)`)
	rxYmdPattern      = compileRegexF(`(?i)`+
		`(?:\D|^)(?:`+
		`(?P<year>%[1]s)[\-/.](?P<month>%[2]s)[\-/.](?P<day>%[3]s)`+
		`|`+
		`(?P<day>%[3]s)[\-/.](?P<month>%[2]s)[\-/.](?P<year>\d{2,4})`+
		`)(?:\D|$)`, rxYear, rxMonth, rxDay)
	rxYmPattern = compileRegexF(`(?i)`+
		`(?:\D|^)(?:`+
		`(?P<year>%[1]s)[\-/.](?P<month>%[2]s)`+
		`|`+
		`(?P<month>%[2]s)[\-/.](?P<year>%[1]s)`+
		`)(?:\D|$)`, rxYear, rxMonth)

	rxCompleteUrl = compileRegexF(`(?i)\D(%[1]s)[/_-](%[2]s)[/_-](%[3]s)(?:\D|$)`,
		rxYear, rxMonth, rxDay)

	rxTextDatePattern = regexp.MustCompile(`(?i)[.:,_/ -]|^\d+$`)

	rxDiscardPattern = regexp.MustCompile(`` +
		`^\d{2}:\d{2}(?: |:|$)|` +
		`^\D*\d{4}\D*$|` +
		`[$€¥Ұ£¢₽₱฿#₹]|` + // currency symbols and special characters
		`[A-Z]{3}[^A-Z]|` + // currency codes
		`(?:^|\D)(?:\+\d{2}|\d{3}|\d{5})\D|` + // tel./IPs/postal codes
		`ftps?|https?|sftp|` + // protocols
		`\.(?:com|net|org|info|gov|edu|de|fr|io)(?:\z|[^\pL\pM\d_])|` + // TLDs
		`IBAN|[A-Z]{2}[0-9]{2}|` + // bank accounts
		`®` + // ©
		``)

	// Extensive search patterns
	rxYearPattern      = compileRegexF(`^\D?(%s)`, rxYear)
	rxThreeCatch       = regexp.MustCompile(`([0-9]{4})/([0-9]{2})/([0-9]{2})`)
	rxThreeLooseCatch  = regexp.MustCompile(`([0-9]{4})[/.-]([0-9]{2})[/.-]([0-9]{2})`)
	rxSelectYmdYear    = compileRegexF(`(%s)\D?$`, rxYear)
	rxYmdYear          = compileRegexF(`^(%s)`, rxYear)
	rxDateStringsCatch = compileRegexF(`(%s)([01][0-9])([0-3][0-9])`, rxYear)
	rxSlashesYear      = regexp.MustCompile(`([0-9]{2})$`)
	rxYyyyMmCatch      = compileRegexF(`(%s)[/.-](1[0-2]|0[1-9])`, rxYear)
	rxMmYyyyYear       = compileRegexF(`(%s)\D?$`, rxYear)
	rxSimpleW3Cleaner  = compileRegexF(`w3.org\D(%s)\D`, rxYear)

	rxThreeComponents = []struct {
		Name    string
		Pattern fnRe2GoFinder
		Catcher *regexp.Regexp
	}{
		{"ThreePattern", re2go.ThreePattern, rxThreeCatch},
		{"ThreeLoosePattern", re2go.ThreeLoosePattern, rxThreeLooseCatch},
	}

	// Time patterns
	rxCommonTime = regexp.MustCompile(`(?i)(?:\D|^)(\d{1,2})(?::|\s*h\s*)(\d{1,2})(?::(\d{1,2})(?:\.\d+)?)?(?:\s*((?:a|p)\.?m\.?))?`)
	rxTzCode     = regexp.MustCompile(`(?i)(?:\s|^)([-+])(\d{2})(?::?(\d{2}))?`)
	rxIsoTime    = regexp.MustCompile(`(?i)(\d{2}):(\d{2})(?::(\d{2})(?:\.\d+)?)?(Z|[+-]\d{2}(?::?\d{2})?)`)

	rxLastJsonBracket = regexp.MustCompile(`(?i)\s*\}$`)
)

// English, French, German, Indonesian and Turkish dates cache
var monthNumber = func() map[string]int {
	var monthNames = [][]string{
		{"jan", "januar", "jänner", "january", "januari", "janvier", "ocak", "oca"},
		{"feb", "februar", "feber", "february", "februari", "février", "şubat", "şub"},
		{"mar", "mär", "märz", "march", "maret", "mart", "mars"},
		{"apr", "april", "avril", "nisan", "nis"},
		{"may", "mai", "mei", "mayıs"},
		{"jun", "juni", "june", "juin", "haziran", "haz"},
		{"jul", "juli", "july", "juillet", "temmuz", "tem"},
		{"aug", "august", "agustus", "ağustos", "ağu", "aout"},
		{"sep", "september", "septembre", "eylül", "eyl"},
		{"oct", "oktober", "october", "octobre", "okt", "ekim", "eki"},
		{"nov", "november", "kasım", "kas", "novembre"},
		{"dec", "dez", "dezember", "december", "desember", "décembre", "aralık", "ara"},
	}

	mapNameNumber := make(map[string]int)
	for i, names := range monthNames {
		for _, name := range names {
			mapNameNumber[name] = i + 1
		}
	}

	return mapNameNumber
}()

var dateAttributes = sliceToMap(
	"analyticsattributes.articledate",
	"article.created",
	"article_date_original",
	"article:post_date",
	"article.published",
	"article:published",
	"article:published_date",
	"article:published_time",
	"article:publicationdate",
	"bt:pubdate",
	"citation_date",
	"citation_publication_date",
	"content_create_date",
	"created",
	"cxenseparse:recs:publishtime",
	"date",
	"date_created",
	"date_published",
	"datecreated",
	"dateposted",
	"datepublished",
	// Dublin Core: https://wiki.whatwg.org/wiki/MetaExtensions
	"dc.date",
	"dc.created",
	"dc.date.created",
	"dc.date.issued",
	"dc.date.publication",
	"dcsext.articlefirstpublished",
	"dcterms.created",
	"dcterms.date",
	"dcterms.issued",
	"dc:created",
	"dc:date",
	"displaydate",
	"doc_date",
	"field-name-post-date",
	"gentime",
	"mediator_published_time",
	"meta", // too loose?
	// Open Graph: https://opengraphprotocol.org/
	"og:article:published",
	"og:article:published_time",
	"og:datepublished",
	"og:pubdate",
	"og:publish_date",
	"og:published_time",
	"og:question:published_time",
	"og:regdate",
	"originalpublicationdate",
	"parsely-pub-date",
	"pdate",
	"ptime",
	"pubdate",
	"publishdate",
	"publish_date",
	"publish_time",
	"publish-date",
	"published-date",
	"published_date",
	"published_time",
	"publisheddate",
	"publication_date",
	"rbpubdate",
	"release_date",
	"rnews:datepublished",
	"sailthru.date",
	"shareaholic:article_published_time",
	"timestamp",
	"twt-published-at",
	"video:release_date",
	"vr:published_time")

var propertyModified = sliceToMap(
	"article:modified",
	"article:modified_date",
	"article:modified_time",
	"article:post_modified",
	"bt:moddate",
	"datemodified",
	"dc.modified",
	"dcterms.modified",
	"lastmodified",
	"modified_time",
	"modificationdate",
	"og:article:modified_time",
	"og:modified_time",
	"og:updated_time",
	"release_date",
	"revision_date",
	"updated_time")

var (
	attrModifiedNames = sliceToMap(
		"lastdate",
		"lastmod",
		"lastmodified",
		"last-modified",
		"modified",
		"utime")
	attrPublishClasses = sliceToMap("published", "date-published", "time-published")

	listItemPropAttrs = []string{"datecreated", "datepublished", "pubyear", "datemodified", "dateupdate"}
	itemPropAttrKeys  = sliceToMap(listItemPropAttrs...)
	itemPropOriginal  = sliceToMap(listItemPropAttrs[:3]...)
	itemPropModified  = sliceToMap(listItemPropAttrs[3:]...)
)

func sliceToMap(strings ...string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, s := range strings {
		result[s] = struct{}{}
	}
	return result
}

func compileRegexF(pattern string, args ...any) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(pattern, args...))
}
