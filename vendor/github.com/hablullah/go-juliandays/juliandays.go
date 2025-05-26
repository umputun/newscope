package juliandays

import (
	"errors"
	"math"
	"time"
)

// FromTime converts Golang std time into Julian Days.
func FromTime(dt time.Time) (float64, error) {
	// Convert to UTC
	dt = dt.UTC()

	// If year is before 4713 B.C, stop
	if dt.Year() < -4712 {
		return 0, errors.New("year is before Julian calendar")
	}

	// If date is in blank days, stop
	endOfJulian := time.Date(1582, 10, 4, 23, 59, 59, 0, time.UTC)
	startOfGregorian := time.Date(1582, 10, 15, 0, 0, 0, 0, time.UTC)
	if dt.After(endOfJulian) && dt.Before(startOfGregorian) {
		return 0, errors.New("date is within blank days")
	}

	// Prepare variables for calculating
	Y := float64(dt.Year())
	M := float64(dt.Month())
	D := float64(dt.Day())
	H := float64(dt.Hour())
	m := float64(dt.Minute())
	s := float64(dt.Second())

	// If month <= 2, change year and month
	if M <= 2 {
		M += 12
		Y--
	}

	// Check whether date is gregorian or julian
	var constant float64
	if dt.After(endOfJulian) {
		temp := math.Floor(float64(Y) / 100)
		constant = 2 + math.Floor(temp/4) - temp
	}

	// Calculate julian day
	yearToDays := math.Floor(Y * 365.25)
	monthToDays := math.Floor((M + 1) * 30.6001)
	timeToDays := (H*3600 + m*60 + s) / 86400
	julianDay := 1720994.5 + yearToDays + monthToDays + constant + D + timeToDays

	return julianDay, nil
}

// ToTime converts Julian Days into Golang std time.
func ToTime(jd float64) time.Time {
	// Prepare variables for calculating
	jd1 := jd + 0.5
	z := math.Floor(jd1)
	f := jd1 - z

	a := z
	if z >= 2299161 {
		aa := math.Floor((z - 1867216.25) / 36524.25)
		a = z + 1 + aa - math.Floor(aa/4)
	}

	b := a + 1524
	c := math.Floor((b - 122.1) / 365.25)
	d := math.Floor(c * 365.25)
	e := math.Floor((b - d) / 30.6001)

	// Calculate day with its time
	dayTime := b - d - math.Floor(e*30.6001) + f
	day := math.Floor(dayTime)

	// Calculate time
	seconds := (dayTime - day) * 24 * 60 * 60

	hours := math.Floor(seconds / 3600)
	seconds -= hours * 3600

	minutes := math.Floor(seconds / 60)
	seconds -= minutes * 60

	// Calculate month
	var month float64
	if e < 14 {
		month = e - 1
	} else {
		month = e - 13
	}

	// Calculate year
	var year float64
	if month > 2 {
		year = c - 4716
	} else {
		year = c - 4715
	}

	// Create date
	return time.Date(
		int(year), time.Month(int(month)), int(day),
		int(hours), int(minutes), int(seconds), 0, time.UTC)
}
