# Go-Hijri

[![Go Report Card][report-badge]][report-url]
[![Go Reference][doc-badge]][doc-url]

Go-Hijri is a Go package for converting Gregorian date to Hijrian date and vice-versa. There are two supported Hijrian calendar :

- the arithmetic calendar which calculated based on arithmetic rules rather than by observation or astronomical calculation,
- the Umm al-Qura calendar which calculated using astronomical rules that used and created by Saudi Arabia and most Islamic country.

## Usage Examples

```go
package main

import (
	"fmt"
	"time"

	"github.com/hablullah/go-hijri"
)

func main() {
	// 1 January 2020 to arithmetic Hijri calendar
	newYear := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	hijriDate, _ := hijri.CreateHijriDate(newYear, hijri.Default)
	fmt.Printf("%s AD = %04d-%02d-%02d H (arithmetic)\n",
		newYear.Format("2006-01-02"),
		hijriDate.Year,
		hijriDate.Month,
		hijriDate.Day)

	// 1 January 2019 to Umm al-Qura calendar
	ummAlQuraDate, _ := hijri.CreateUmmAlQuraDate(newYear)
	fmt.Printf("%s AD = %s, %04d-%02d-%02d H (Umm al-Qura)\n",
		newYear.Format("2006-01-02"),
		ummAlQuraDate.Weekday.String(),
		ummAlQuraDate.Year,
		ummAlQuraDate.Month,
		ummAlQuraDate.Day)

	// 1 Ramadhan 1410 arithmetic Hijri to Gregorian
	stdRamadhan := hijri.HijriDate{Year: 1410, Month: 9, Day: 1}
	fmt.Printf("1410-09-01 H (arithmetic) = %s AD\n",
		stdRamadhan.ToGregorian().Format("2006-01-02"))

	// 1 Ramadhan 1442 Umm al-Qura to Gregorian
	ummAlQuraRamadhan := hijri.UmmAlQuraDate{Year: 1410, Month: 9, Day: 1}
	fmt.Printf("1410-09-01 H (Umm al-Qura) = %s AD\n",
		ummAlQuraRamadhan.ToGregorian().Format("2006-01-02"))
}
```

Codes above will give us following results :

```
2020-01-01 AD = 1441-05-05 H (arithmetic)
2020-01-01 AD = Thursday, 1441-05-06 H (Umm al-Qura)
1410-09-01 H (arithmetic) = 1990-03-28 AD
1410-09-01 H (Umm al-Qura) = 1990-03-27 AD
```

## Resource

1. Anugraha, R. 2012. _Mekanika Benda Langit_. ([PDF][pdf-rinto-anugraha])
2. Van Gent, R. H. 2019. _Islamic-Western Calendar Converter_. ([Website][web-van-gent-1])
3. Van Gent, R. H. 2019. _The Umm al-Qura Calendar of Saudi Arabia_. ([Website][web-van-gent-2])
4. Strous, Dr. 2019. _Astronomy Answers: Julian Days Number_. ([Website][web-astronomy-answers])

## License

Go-Hijri is distributed using [MIT](http://choosealicense.com/licenses/mit/) license.

[report-badge]: https://goreportcard.com/badge/github.com/hablullah/go-hijri
[report-url]: https://goreportcard.com/report/github.com/hablullah/go-hijri
[doc-badge]: https://pkg.go.dev/badge/github.com/hablullah/go-hijri.svg
[doc-url]: https://pkg.go.dev/github.com/hablullah/go-hijri

[pdf-rinto-anugraha]: https://simpan.ugm.ac.id/s/GcxKuyZWn8Rshnn
[web-van-gent-1]: https://webspace.science.uu.nl/~gent0113/islam/islam_tabcal.htm
[web-van-gent-2]: https://webspace.science.uu.nl/~gent0113/islam/ummalqura.htm
[web-astronomy-answers]: https://www.aa.quae.nl/en/reken/juliaansedag.html