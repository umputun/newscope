package whatlanggo

import (
	"unicode"
)

type trigram struct {
	trigram string
	count   int
}

// convert punctuations and digits to space.
func toTrigramChar(ch rune) rune {
	if isStopChar(ch) {
		return ' '
	}
	return ch
}

func count(input string) map[string]int {
	// Convert input runes to lower
	inputRunes := []rune(input)
	nInputRunes := len(inputRunes)

	runes := make([]rune, nInputRunes+1)
	for i, ir := range inputRunes {
		runes[i] = unicode.ToLower(toTrigramChar(ir))
	}
	runes[nInputRunes] = ' ' // put space as the last rune

	var r1, r2, r3 rune
	trigrams := map[string]int{}

	r1, r2 = ' ', runes[0]
	for i := 1; i < len(runes); i++ {
		r3 = runes[i]
		if !(r2 == ' ' && (r1 == ' ' || r3 == ' ')) {
			trigram := []rune{r1, r2, r3}
			strTrigram := string(trigram)

			if trigrams[strTrigram] == 0 {
				trigrams[strTrigram] = 1
			} else {
				trigrams[strTrigram]++
			}
		}
		r1 = r2
		r2 = r3
	}
	return trigrams
}
