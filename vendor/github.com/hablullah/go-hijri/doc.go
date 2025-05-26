// Hijri is package for converting Gregorian date into Hijri date and vice versa. Hijri or Islamic
// calendar system itself is a lunar calendar used in many Muslim countries, with a year has 12 months
// and 354 days or 355 days in a leap year.
//
// Hijri calendar only recognizes one era: A.H. (Latin "Anno Hegirae", which means "the year of the
// migration," in reference to the migration of Muhammad (PBUH) from Mecca). With that said, Hijri
// calendar is not proleptic, so there are no negative Hijri year.
//
// This package supports two kind of Hijri calendar, the arithmetic calendar and Umm al-Qura calendar.
//
// The arithmetic or tabular Hijri calendar (or simply Hijri) is a rule-based variation of the Islamic
// calendar, in which months are worked out by arithmetic rules rather than by observation or astronomical
// calculation. It is introduced by Muslim astronomers in the 8th century CE to predict the approximate
// beginning of the months in the Islamic lunar calendar.
//
// It has a 30-year cycle with 11 leap years of 355 days and 19 years of 354 days. In the long term, it
// is accurate to one day in about 2,500 solar years or 2,570 lunar years. It also deviates up to about
// one or two days in the short term. However, there are several patterns of leap years to decide which
// years within the 30 are leap.
//
// The Umm al-Qura calendar is astronomical-based calendar that used and created by Saudi Arabia. It is
// also used by several neighbouring states on the Arabian Peninsula such as Bahrain and Qatar. For this
// calendar, each month has either 29 or 30 days, but usually in no discernible order.
//
// The implementation of Umm al-Qura calendar in this package is based on Javascript code by R.H. van Gent
// from Utrecht University. The date must be between 14 March 1937 (1 Muharram 1356 H) and 16 November 2077
// (29 Dhu al-Hijjah 1500 H).
package hijri
