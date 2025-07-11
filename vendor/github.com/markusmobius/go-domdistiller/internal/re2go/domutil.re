/*!include:re2c "base.re" */

import "strings"

// Original pattern: \s+([.?!,;])\s*(\S*)
func TidyUpPunctuation(input string) string {
	var cursor, marker int
	input += string(rune(0)) // add terminating null
	limit := len(input) - 1  // limit points at the terminating null
	_ = marker

	// Variable for capturing parentheses (twice the number of groups).
	/*!maxnmatch:re2c*/
	yypmatch := make([]int, YYMAXNMATCH*2)
	var yynmatch int
	_ = yynmatch

	// Autogenerated tag variables used by the lexer to track tag values.
	/*!stags:re2c format = 'var @@ int; _ = @@\n'; */

	var start int
	var sb strings.Builder
	for { /*!use:re2c:base_template
		re2c:posix-captures = 1;

		space    = [\t\n\f\r ];
		nonSpace = [^\t\n\f\r ];

		quant1      = {space}+;
		punctuation = {space}+([.?!,;]){space}*({nonSpace}*);

		{quant1}      { continue }
		{punctuation} {
			before := input[start:yypmatch[0]]
			submatch1 := input[yypmatch[2]:yypmatch[3]]
			submatch2 := input[yypmatch[4]:yypmatch[5]]

			sb.WriteString(before)
			sb.WriteString(submatch1)
			sb.WriteString(" ")
			sb.WriteString(submatch2)

			start = yypmatch[1]
			continue
		}

		$ {
			sb.WriteString(input[start:limit])
			return sb.String()
		}

		* { continue }
		*/
	}
}

// Original pattern: \s*\|\\/\|\s*
func FixTempNewline(input string) string {
	var cursor, marker int
	input += string(rune(0)) // add terminating null
	limit := len(input) - 1  // limit points at the terminating null
	_ = marker

	// Variable for capturing parentheses (twice the number of groups).
	/*!maxnmatch:re2c*/
	yypmatch := make([]int, YYMAXNMATCH*2)
	var yynmatch int
	_ = yynmatch

	// Autogenerated tag variables used by the lexer to track tag values.
	/*!stags:re2c format = 'var @@ int; _ = @@\n'; */

	var start int
	var sb strings.Builder
	for { /*!use:re2c:base_template
		re2c:posix-captures = 1;

		space      = [\t\n\f\r ];
		quant1     = {space}*[^|];
		tmpNewline = {space}*[|][\\][/][|]{space}*;

		{quant1} { continue }

		{tmpNewline} {
			sb.WriteString(input[start:yypmatch[0]])
			sb.WriteString("\n")
			start = yypmatch[1]
			continue
		}

		$ {
			sb.WriteString(input[start:limit])
			return sb.String()
		}

		* { continue }
		*/
	}
}