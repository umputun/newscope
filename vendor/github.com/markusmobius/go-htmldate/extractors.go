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
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-shiori/dom"
	dps "github.com/markusmobius/go-dateparser"
	"github.com/markusmobius/go-htmldate/internal/re2go"
	"github.com/markusmobius/go-htmldate/internal/selector"
	"golang.org/x/net/html"
)

// discardUnwanted removes unwanted sections of an HTML document and
// return the discarded elements as a list.
func discardUnwanted(doc *html.Node) []*html.Node {
	var discardedElements []*html.Node
	for _, elem := range selector.QueryAll(doc, selector.Discard) {
		if elem.Parent != nil {
			elem.Parent.RemoveChild(elem)
			discardedElements = append(discardedElements, elem)
		}
	}

	return discardedElements
}

// extractUrlDate extract the date out of an URL string complying
// with the Y-M-D format.
func extractUrlDate(url string, opts Options) time.Time {
	// Extract date component using regex
	parts := rxCompleteUrl.FindStringSubmatch(url)
	if len(parts) != 4 {
		return timeZero
	}

	// Create date from the extracted parts
	year, _ := strconv.Atoi(parts[1])
	month, _ := strconv.Atoi(parts[2])
	day, _ := strconv.Atoi(parts[3])

	date, valid := validateDateParts(year, month, day, opts)
	if !valid {
		return timeZero
	}

	log.Debug().Msgf("found date in url: %s", parts[0])
	return date
}

// tryDateExpr tries to extract date which contains year, month and day using
// a series of heuristics and rules.
func tryDateExpr(s string, opts Options) (string, time.Time) {
	// Trim
	s = normalizeSpaces(s)
	s = strLimit(s, maxSegmentLen)

	// If string less than 6 runes, stop
	if utf8.RuneCountInString(s) < 6 {
		return s, timeZero
	}

	// Formal constraint: 4 to 18 digits
	nDigit := getDigitCount(s)
	if nDigit < 4 || nDigit > 18 {
		return s, timeZero
	}

	// Check if string only contains time/single year or digits and not a date
	if rxDiscardPattern.MatchString(s) {
		return s, timeZero
	}

	// Try to parse date using the faster method
	parseResult := fastParse(s, opts)
	if !parseResult.IsZero() {
		return s, parseResult
	}

	// Use slow but extensive search, using dateparser
	if !opts.SkipExtensiveSearch {
		// Additional filters to prevent computational cost
		if !rxTextDatePattern.MatchString(s) {
			return s, timeZero
		}

		dt := externalDateParser(s, opts)
		if !dt.IsZero() {
			return s, dt
		}
	}

	return s, timeZero
}

// fastParse parse the string into time.Time.
// In the original Python library, this function is named `custom_parse`, but I
// renamed it to `fastParse` because I think it's more suitable to its purpose.
func fastParse(s string, opts Options) time.Time {
	// 1. Try YYYYMMDD without regex first
	// This also handle '201709011234' which not covered by dateparser
	if len(s) >= 8 && isDigit(s[4:8]) {
		year, _ := strconv.Atoi(s[:4])
		month, _ := strconv.Atoi(s[4:6])
		day, _ := strconv.Atoi(s[6:8])

		if dt, valid := validateDateParts(year, month, day, opts); valid {
			log.Debug().Msgf("fast parse found Y-M-D without separator: %s", s[:8])
			return dt
		}
	}

	// 2. Try YYYYMMDD with regex
	parts := rxYmdNoSepPattern.FindStringSubmatch(s)
	if len(parts) == 2 {
		text := parts[1]
		year, _ := strconv.Atoi(text[:4])
		month, _ := strconv.Atoi(text[4:6])
		day, _ := strconv.Atoi(text[6:8])

		if dt, valid := validateDateParts(year, month, day, opts); valid {
			log.Debug().Msgf("fast parse found Y-M-D without separator: %s", s[:8])
			return dt
		}
	}

	// 3. Try the very common YMD, Y-M-D, and D-M-Y patterns
	namedParts, lastMatchedName := rxFindNamedStringSubmatch(rxYmdPattern, s)
	if len(namedParts) != 0 {
		year, _ := strconv.Atoi(namedParts["year"])
		month, _ := strconv.Atoi(namedParts["month"])
		day, _ := strconv.Atoi(namedParts["day"])

		if lastMatchedName != "day" { // handle D-M-Y formats
			year = correctYear(year)
			day, month = trySwapValues(day, month)
		}

		// Make sure month is at most 12, because if not then it's not YMD
		dt, valid := validateDateParts(year, month, day, opts)
		if valid {
			log.Debug().Msgf("fast parse found Y-M-D date: %s", s)
			return dt
		}
	}

	// 4. Try the Y-M and M-Y patterns
	namedParts, _ = rxFindNamedStringSubmatch(rxYmPattern, s)
	if len(namedParts) != 0 {
		year, _ := strconv.Atoi(namedParts["year"])
		month, _ := strconv.Atoi(namedParts["month"])

		// Make sure month is at most 12, because if not then it's not D-M-Y
		dt, valid := validateDateParts(year, month, 1, opts)
		if valid {
			log.Debug().Msgf("fast parse found Y-M date: %s", s)
			return dt
		}
	}

	// 5. Try the other regex pattern
	dt := regexParse(s, opts)
	if validateDate(dt, opts) {
		log.Debug().Msgf("fast parse found regex date: %s", dt.Format("2006-01-02"))
		return dt
	}

	log.Error().Msgf("failed to parse \"%s\"", s)
	return timeZero
}

// externalDateParser uses go-dateparser package to extensively look for date.
func externalDateParser(s string, opts Options) time.Time {
	var cfg *dps.Configuration
	if opts.DateParserConfig != nil {
		cfg = opts.DateParserConfig
	} else {
		cfg = externalDpsConfig
	}

	dt, _ := externalParser.Parse(cfg, s)
	if validateDate(dt.Time, opts) {
		return dt.Time
	}

	return timeZero
}

// jsonSearch looks for JSON time patterns in JSON sections of the document.
func jsonSearch(doc *html.Node, opts Options) (string, time.Time) {
	// Prepare targetKeys to look for
	var targetKeys map[string]struct{}
	if opts.UseOriginalDate {
		targetKeys = sliceToMap("datePublished", "dateCreated")
	} else {
		targetKeys = sliceToMap("dateModified")
	}

	// Prepare function to capture date texts recursively
	var capturedTexts []jsonCapturedText
	var findDateTexts func(obj map[string]interface{})
	findDateTexts = func(obj map[string]interface{}) {
		for key, value := range obj {
			switch v := value.(type) {
			case string:
				if inMap(key, targetKeys) {
					capturedTexts = append(capturedTexts, jsonCapturedText{
						Key:  key,
						Text: normalizeSpaces(v),
					})
				}

			case map[string]interface{}:
				findDateTexts(v)

			case []interface{}:
				for _, item := range v {
					itemObject, isObject := item.(map[string]interface{})
					if isObject {
						findDateTexts(itemObject)
					}
				}
			}
		}
	}

	// Look throughout the HTML tree
	ldJsonScripts := dom.QuerySelectorAll(doc, `script[type="application/ld+json"]`)
	settingsJsonScripts := dom.QuerySelectorAll(doc, `script[type="application/settings+json"]`)
	scriptNodes := append(ldJsonScripts, settingsJsonScripts...)

	for _, elem := range scriptNodes {
		// Get the json text inside the script
		jsonText := dom.TextContent(elem)
		jsonText = strings.TrimSpace(jsonText)
		log.Debug().Msgf("found JSON: %s", strLimit(jsonText, 200))

		// First, decode JSON text assuming it as array of object
		var err error
		arrayData := []map[string]interface{}{}
		err = json.Unmarshal([]byte(jsonText), &arrayData)
		if err == nil {
			for _, data := range arrayData {
				findDateTexts(data)
			}
			continue
		}

		// If it's not array, decode JSON text assuming it as an object
		// There are some web pages whose JSON+LD contains additional trailing closing bracket
		// which make JSON decoder failed. So, here if the JSON decoder failed we'll remove
		// the last trailing bracket then try again.
		objData := map[string]interface{}{}
		for {
			err = json.Unmarshal([]byte(jsonText), &objData)
			if err == nil {
				break
			}

			tmp := rxLastJsonBracket.ReplaceAllString(jsonText, "")
			if tmp == jsonText {
				break
			}

			jsonText = tmp
		}

		if err == nil {
			findDateTexts(objData)
			continue
		}

		// At this point JSON decoder has failed
		log.Debug().Msgf("failed to decode JSON: %v", err)
	}

	// Parse date for each captured texts
	var dates []jsonCapturedDate
	for _, capturedText := range capturedTexts {
		dt := fastParse(capturedText.Text, opts)
		if validateDate(dt, opts) {
			dates = append(dates, jsonCapturedDate{
				Text: capturedText.Text,
				Date: dt,
			})
		}
	}

	if len(dates) == 0 {
		return "", timeZero
	}

	log.Debug().Msgf("captured dates: %v", dates)

	// Find the best date
	var best jsonCapturedDate
	for _, cd := range dates {
		if best.Date.IsZero() ||
			(opts.UseOriginalDate && cd.Date.Before(best.Date)) ||
			(!opts.UseOriginalDate && cd.Date.After(best.Date)) {
			best = cd
		}
	}

	return best.Text, best.Date
}

// idiosyncrasiesSearch looks for author-written dates throughout the web page.
func idiosyncrasiesSearch(htmlString string, opts Options) (string, time.Time) {
	// Extract date parts
	var candidate time.Time
	parts, startIdx := re2go.IdiosyncracyPatternSubmatch(htmlString)
	if len(parts) == 0 {
		return "", timeZero
	}

	// Process parts
	if len(parts[1]) == 4 { // YYYY/MM/DD
		year, _ := strconv.Atoi(parts[1])
		month, _ := strconv.Atoi(parts[2])
		day, _ := strconv.Atoi(parts[3])
		candidate, _ = validateDateParts(year, month, day, opts)
	} else if tmp := len(parts[3]); tmp == 2 || tmp == 4 { // DD/MM/YY or MM/DD/YY
		year, _ := strconv.Atoi(parts[3])
		month, _ := strconv.Atoi(parts[2])
		day, _ := strconv.Atoi(parts[1])

		year = correctYear(year)
		day, month = trySwapValues(day, month)
		candidate, _ = validateDateParts(year, month, day, opts)
	}

	if !validateDate(candidate, opts) {
		return "", timeZero
	}

	// Get raw string
	rawString := strLimit(htmlString[startIdx:], 100)

	// Return candidate
	log.Debug().Msgf("idiosyncratic pattern found: %s", parts[0])
	return rawString, candidate
}

// metaImgSearch looks for url in <meta> image elements.
func metaImgSearch(doc *html.Node, opts Options) (string, time.Time) {
	for _, elem := range dom.QuerySelectorAll(doc, `meta[property="og:image"]`) {
		content := strings.TrimSpace(dom.GetAttribute(elem, "content"))
		if content != "" {
			result := extractUrlDate(content, opts)
			if validateDate(result, opts) {
				return content, result
			}
		}
	}

	return "", timeZero
}

// regexPatternSearch looks for date expressions using a regular expression on a string of text.
func regexPatternSearch(
	text string,
	patternName string,
	dateSubmatchFinder func(string) ([]string, int),
	opts Options,
) (string, time.Time) {
	parts, _ := dateSubmatchFinder(text)
	if len(parts) < 2 {
		return "", timeZero
	}

	dt := fastParse(parts[1], opts)
	if validateDate(dt, opts) {
		log.Debug().Msgf("regex found: %q %q", patternName, parts[0])
		return parts[0], dt
	}

	return "", timeZero
}

// regexParse try full-text parse for date elements using a series of regular
// expressions with particular emphasis on English, French, German and Turkish.
func regexParse(s string, opts Options) time.Time {
	var exist bool
	var year, month, day int

	// Multilingual day-month-year pattern + American English patterns
	strYear, strMonth, strDay, ok := re2go.FindLongTextPattern(s)
	if ok {
		strMonth = strings.ToLower(strMonth)
		month, exist = monthNumber[strMonth]
		if exist {
			year, _ = strconv.Atoi(strYear)
			day, _ = strconv.Atoi(strDay)
		}
	}

	year = correctYear(year)
	day, month = trySwapValues(day, month)
	dt, valid := validateDateParts(year, month, day, opts)
	if valid {
		log.Debug().Msgf("multilingual text found: %s", s)
		return dt
	}

	return timeZero
}

func correctYear(year int) int {
	if year < 100 {
		if year >= 90 {
			year += 1900
		} else {
			year += 2000
		}
	}

	return year
}

// trySwapValues swap day and month values if it seems feaaible.
func trySwapValues(day, month int) (int, int) {
	if month > 12 && day <= 12 {
		return month, day
	}
	return day, month
}

type jsonCapturedText struct {
	Key  string
	Text string
}

type jsonCapturedDate struct {
	Text string
	Date time.Time
}
