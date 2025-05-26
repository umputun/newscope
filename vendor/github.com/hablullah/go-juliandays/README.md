# Go-JulianDays

[![Go Report Card][report-badge]][report-url]
[![Go Reference][doc-badge]][doc-url]

Go-JulianDays is a simple package for converting date time into Julian Days and vice versa.

## Usage Examples

```go
package main

import (
	"fmt"
	"time"

	"github.com/hablullah/go-juliandays"
)

func main() {
	// Convert date time to Julian Days
	dt := time.Date(2021, 1, 1, 12, 0, 0, 0, time.UTC)
	jd, _ := juliandays.FromTime(dt)
	fmt.Printf("%s = %.0f JD\n", dt.Format("2006-01-02 15:04:05"), jd)

	// Convert Julian Days to date time
	jd = 100
	dt = juliandays.ToTime(jd)
	fmt.Printf("%.0f JD = %s\n", jd, dt.Format("2006-01-02 15:04:05"))
}
```

Codes above will give us following results :

```
2021-01-01 12:00:00 = 2459216 JD
100 JD = -4712-04-10 12:00:00
```

## Resources

1. Anugraha, R. 2012. _Mekanika Benda Langit_. ([PDF][pdf-rinto-anugraha])

## License

Go-JulianDays is distributed using [MIT] license.

[report-badge]: https://goreportcard.com/badge/github.com/hablullah/go-juliandays
[report-url]: https://goreportcard.com/report/github.com/hablullah/go-juliandays
[doc-badge]: https://pkg.go.dev/badge/github.com/hablullah/go-juliandays.svg
[doc-url]: https://pkg.go.dev/github.com/hablullah/go-juliandays
[pdf-rinto-anugraha]: https://simpan.ugm.ac.id/s/GcxKuyZWn8Rshnn
[MIT]: http://choosealicense.com/licenses/mit/