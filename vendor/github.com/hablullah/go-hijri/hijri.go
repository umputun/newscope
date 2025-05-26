package hijri

import (
	"errors"
	"time"

	"github.com/hablullah/go-juliandays"
)

// LeapYearsPattern is patterns of leap years in the 30 year cycle.
type LeapYearsPattern uint8

const (
	// Default is the most commonly used leap years pattern. In this pattern, leap year happened
	// on years 2, 5, 7, 10, 13, 16, 18, 21, 24, 26 & 29.
	Default LeapYearsPattern = iota

	// Base15 is leap years pattern that used by Microsoft, and they named it as "Kuwaiti algorithm".
	// In this pattern, leap year happened on years 2, 5, 7, 10, 13, 15, 18, 21, 24, 26 & 29.
	Base15

	// Fattimid is leap years pattern that used in Fattimid empire. In this pattern, leap year
	// happened on years 2, 5, 8, 10, 13, 16, 19, 21, 24, 27 & 29.
	Fattimid

	// HabashAlHasib is leap years pattern that created using research from Habash al-Hasib,
	// an astronomer from Abbasid empire (766-869 in Iraq). In this pattern, leap year happened on
	// years 2, 5, 8, 11, 13, 16, 19, 21, 24, 27 & 30.
	HabashAlHasib
)

// HijriDate is date that uses arithmetic Islamic calendar system.
type HijriDate struct {
	Day     int64
	Month   int64
	Year    int64
	Pattern LeapYearsPattern
}

// CreateHijriDate converts normal Gregorian date to Hijri date. Since Hijri calendar is not proleptic
// any date before 16 July 622 CE (1 Muharram 1 H) will make this method throws error.
func CreateHijriDate(date time.Time, leapPattern LeapYearsPattern) (HijriDate, error) {
	// Convert date to UTC and strip times from the date
	date = date.UTC()
	date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)

	// Calculate Julian Days
	julianDays, err := juliandays.FromTime(date)
	if err != nil {
		return HijriDate{}, err
	}

	// Get days since 1 Muharram 1
	islamicDays := int64(julianDays - 1948438.5)
	if islamicDays < 0 {
		return HijriDate{}, errors.New("date is before hijri calendar started")
	}

	// Check how many 30 years cycles to reach this day
	nCycles := islamicDays / 10631

	// Calculate leftover years outside 30 years cycle
	leftoverDays := islamicDays % 10631
	leftoverYears := leftoverDays / 354

	// Calculate the leftover days after years subtracted
	leftoverDays = leftoverDays % 354

	// Adjust leftover days based on leap years that happened within leftover years
	for year := int64(1); year <= leftoverYears; year++ {
		if isLeapYear(year, leapPattern) {
			leftoverDays--
		}
	}

	// Calculate final hijri year
	hijriYear := nCycles*30 + leftoverYears
	if leftoverDays > 0 {
		hijriYear++
	} else {
		leftoverDays += 354
		if isLeapYear(hijriYear, leapPattern) {
			leftoverDays++
		}
	}

	// Calculate final hijri month and day
	var hijriDay, hijriMonth int64
	inLeapYear := isLeapYear(hijriYear, leapPattern)

	for month := int64(1); month <= 12; month++ {
		hijriMonth = month
		daysInMonth := int64(29 + month%2)
		if inLeapYear && month == 12 {
			daysInMonth = 30
		}

		leftoverDays -= daysInMonth
		if leftoverDays <= 0 {
			hijriDay = leftoverDays + daysInMonth
			break
		}
	}

	return HijriDate{
		Day:     hijriDay,
		Month:   hijriMonth,
		Year:    hijriYear,
		Pattern: leapPattern,
	}, nil
}

// ToGregorian convert Hijri date to Gregorian date using Golang standard time.
func (h HijriDate) ToGregorian() time.Time {
	// Calculate the passed days from the passed hijri years
	passedYear := h.Year - 1
	nCycles := passedYear / 30
	leftoverYears := passedYear % 30
	passedDays := nCycles*10631 + leftoverYears*354

	// Consider leap years to the count of passed days
	for year := int64(1); year <= leftoverYears; year++ {
		if isLeapYear(year, h.Pattern) {
			passedDays++
		}
	}

	// Increase the passed days from the passed hijri months
	passedMonths := h.Month - 1
	for month := int64(1); month <= passedMonths; month++ {
		passedDays += 29 + month%2
	}

	// Increase passed days using current hijri day
	passedDays += h.Day

	// Calculate Julian Days since Hijri epoch
	jd := 1948438.5 + float64(passedDays)
	return juliandays.ToTime(jd)
}

func isLeapYear(year int64, pattern LeapYearsPattern) bool {
	year = year % 30

	switch pattern {
	case Default:
		switch year {
		case 2, 5, 7, 10, 13, 16, 18, 21, 24, 26, 29:
			return true
		}

	case Base15:
		switch year {
		case 2, 5, 7, 10, 13, 15, 18, 21, 24, 26, 29:
			return true
		}

	case Fattimid:
		switch year {
		case 2, 5, 8, 10, 13, 16, 19, 21, 24, 27, 29:
			return true
		}

	case HabashAlHasib:
		switch year {
		case 2, 5, 8, 11, 13, 16, 19, 21, 24, 27, 30:
			return true
		}
	}

	return false
}
