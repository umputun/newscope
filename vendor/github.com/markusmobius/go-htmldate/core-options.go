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
	"time"

	dps "github.com/markusmobius/go-dateparser"
)

// Options is configuration for the extractor.
type Options struct {
	// ExtractTime specify if we want to extract publish time as well along the date. Still WIP.
	ExtractTime bool

	// UseOriginalDate specify whether to extract the original date (e.g. publication date) instead
	// of most recent one (e.g. last modified, updated time).
	UseOriginalDate bool

	// URL is the URL for the webpage.
	URL string

	// MinDate is the earliest acceptable date.
	MinDate time.Time

	// MaxDate is the latest acceptable date.
	MaxDate time.Time

	// EnableLog specify whether log should be enabled or not.
	EnableLog bool

	// SkipExtensiveSearch specify whether to skip pattern-based opportunistic text search or not
	// using the external `dateparser` library. Note: this extensive search might be quite slow,
	// so use as necessary.
	SkipExtensiveSearch bool

	// DeferUrlExtractor specify whether to use URL extractor only as backup to
	// prioritize full expressions.
	DeferUrlExtractor bool

	// DateParserConfig is configuration for the external `dateparser`. Only used extensive search
	// is enabled (`SkipExtensiveSearch=false`).
	DateParserConfig *dps.Configuration
}
