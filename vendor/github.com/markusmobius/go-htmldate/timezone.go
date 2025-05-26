package htmldate

import (
	"strconv"
	"strings"
	"time"
)

// parseTimezoneCode returns the location for the specified timezone code.
func parseTimezoneCode(tzCode string) *time.Location {
	// If it's equal to Z, it's UTC
	tzCode = strings.ToUpper(tzCode)
	if tzCode == "Z" {
		return time.UTC
	}

	// Try ISO timezone format
	parts := rxTzCode.FindStringSubmatch(tzCode)
	if len(parts) > 0 {
		hour, _ := strconv.Atoi(parts[2])
		minute, _ := strconv.Atoi(parts[3])

		offset := hour*3_600 + minute*60
		if parts[1] == "-" {
			offset *= -1
		}

		return time.FixedZone(tzCode, offset)
	}

	// If nothing found, return nil
	return nil
}

// findNamedTimezone looks for known named timezone from the string.00
func findNamedTimezone(str string) *time.Location {
	for _, s := range strings.Fields(str) {
		if offset, exist := mapTimezoneNames[s]; exist {
			return time.FixedZone(s, offset)
		}
	}
	return nil
}

// mapTimezoneNames contains list of common timezone names. Unfortunately, there are
// some timezones with same name e.g. MST is used for Malaysia Standard Time and
// Mountain Standard Time in North America. In these cases, we have no choice but only
// use one.
var mapTimezoneNames = map[string]int{
	// https://en.wikipedia.org/wiki/List_of_tz_database_time_zones
	"Africa/Abidjan":                   0,      // +00:00
	"Africa/Accra":                     0,      // +00:00
	"Africa/Addis_Ababa":               10800,  // +03:00
	"Africa/Algiers":                   3600,   // +01:00
	"Africa/Asmara":                    10800,  // +03:00
	"Africa/Asmera":                    10800,  // +03:00
	"Africa/Bamako":                    0,      // +00:00
	"Africa/Bangui":                    3600,   // +01:00
	"Africa/Banjul":                    0,      // +00:00
	"Africa/Bissau":                    0,      // +00:00
	"Africa/Blantyre":                  7200,   // +02:00
	"Africa/Brazzaville":               3600,   // +01:00
	"Africa/Bujumbura":                 7200,   // +02:00
	"Africa/Cairo":                     7200,   // +02:00
	"Africa/Casablanca":                3600,   // +01:00
	"Africa/Ceuta":                     3600,   // +01:00
	"Africa/Conakry":                   0,      // +00:00
	"Africa/Dakar":                     0,      // +00:00
	"Africa/Dar_es_Salaam":             10800,  // +03:00
	"Africa/Djibouti":                  10800,  // +03:00
	"Africa/Douala":                    3600,   // +01:00
	"Africa/El_Aaiun":                  3600,   // +01:00
	"Africa/Freetown":                  0,      // +00:00
	"Africa/Gaborone":                  7200,   // +02:00
	"Africa/Harare":                    7200,   // +02:00
	"Africa/Johannesburg":              7200,   // +02:00
	"Africa/Juba":                      7200,   // +02:00
	"Africa/Kampala":                   10800,  // +03:00
	"Africa/Khartoum":                  7200,   // +02:00
	"Africa/Kigali":                    7200,   // +02:00
	"Africa/Kinshasa":                  3600,   // +01:00
	"Africa/Lagos":                     3600,   // +01:00
	"Africa/Libreville":                3600,   // +01:00
	"Africa/Lome":                      0,      // +00:00
	"Africa/Luanda":                    3600,   // +01:00
	"Africa/Lubumbashi":                7200,   // +02:00
	"Africa/Lusaka":                    7200,   // +02:00
	"Africa/Malabo":                    3600,   // +01:00
	"Africa/Maputo":                    7200,   // +02:00
	"Africa/Maseru":                    7200,   // +02:00
	"Africa/Mbabane":                   7200,   // +02:00
	"Africa/Mogadishu":                 10800,  // +03:00
	"Africa/Monrovia":                  0,      // +00:00
	"Africa/Nairobi":                   10800,  // +03:00
	"Africa/Ndjamena":                  3600,   // +01:00
	"Africa/Niamey":                    3600,   // +01:00
	"Africa/Nouakchott":                0,      // +00:00
	"Africa/Ouagadougou":               0,      // +00:00
	"Africa/Porto-Novo":                3600,   // +01:00
	"Africa/Sao_Tome":                  0,      // +00:00
	"Africa/Timbuktu":                  0,      // +00:00
	"Africa/Tripoli":                   7200,   // +02:00
	"Africa/Tunis":                     3600,   // +01:00
	"Africa/Windhoek":                  7200,   // +02:00
	"America/Adak":                     -36000, // -10:00
	"America/Anchorage":                -32400, // -09:00
	"America/Anguilla":                 -14400, // -04:00
	"America/Antigua":                  -14400, // -04:00
	"America/Araguaina":                -10800, // -03:00
	"America/Argentina/Buenos_Aires":   -10800, // -03:00
	"America/Argentina/Catamarca":      -10800, // -03:00
	"America/Argentina/ComodRivadavia": -10800, // -03:00
	"America/Argentina/Cordoba":        -10800, // -03:00
	"America/Argentina/Jujuy":          -10800, // -03:00
	"America/Argentina/La_Rioja":       -10800, // -03:00
	"America/Argentina/Mendoza":        -10800, // -03:00
	"America/Argentina/Rio_Gallegos":   -10800, // -03:00
	"America/Argentina/Salta":          -10800, // -03:00
	"America/Argentina/San_Juan":       -10800, // -03:00
	"America/Argentina/San_Luis":       -10800, // -03:00
	"America/Argentina/Tucuman":        -10800, // -03:00
	"America/Argentina/Ushuaia":        -10800, // -03:00
	"America/Aruba":                    -14400, // -04:00
	"America/Asuncion":                 -14400, // -04:00
	"America/Atikokan":                 -18000, // -05:00
	"America/Atka":                     -36000, // -10:00
	"America/Bahia":                    -10800, // -03:00
	"America/Bahia_Banderas":           -21600, // -06:00
	"America/Barbados":                 -14400, // -04:00
	"America/Belem":                    -10800, // -03:00
	"America/Belize":                   -21600, // -06:00
	"America/Blanc-Sablon":             -14400, // -04:00
	"America/Boa_Vista":                -14400, // -04:00
	"America/Bogota":                   -18000, // -05:00
	"America/Boise":                    -25200, // -07:00
	"America/Buenos_Aires":             -10800, // -03:00
	"America/Cambridge_Bay":            -25200, // -07:00
	"America/Campo_Grande":             -14400, // -04:00
	"America/Cancun":                   -18000, // -05:00
	"America/Caracas":                  -14400, // -04:00
	"America/Catamarca":                -10800, // -03:00
	"America/Cayenne":                  -10800, // -03:00
	"America/Cayman":                   -18000, // -05:00
	"America/Chicago":                  -21600, // -06:00
	"America/Chihuahua":                -25200, // -07:00
	"America/Coral_Harbour":            -18000, // -05:00
	"America/Cordoba":                  -10800, // -03:00
	"America/Costa_Rica":               -21600, // -06:00
	"America/Creston":                  -25200, // -07:00
	"America/Cuiaba":                   -14400, // -04:00
	"America/Curacao":                  -14400, // -04:00
	"America/Danmarkshavn":             0,      // +00:00
	"America/Dawson":                   -25200, // -07:00
	"America/Dawson_Creek":             -25200, // -07:00
	"America/Denver":                   -25200, // -07:00
	"America/Detroit":                  -18000, // -05:00
	"America/Dominica":                 -14400, // -04:00
	"America/Edmonton":                 -25200, // -07:00
	"America/Eirunepe":                 -18000, // -05:00
	"America/El_Salvador":              -21600, // -06:00
	"America/Ensenada":                 -28800, // -08:00
	"America/Fort_Nelson":              -25200, // -07:00
	"America/Fort_Wayne":               -18000, // -05:00
	"America/Fortaleza":                -10800, // -03:00
	"America/Glace_Bay":                -14400, // -04:00
	"America/Godthab":                  -10800, // -03:00
	"America/Goose_Bay":                -14400, // -04:00
	"America/Grand_Turk":               -18000, // -05:00
	"America/Grenada":                  -14400, // -04:00
	"America/Guadeloupe":               -14400, // -04:00
	"America/Guatemala":                -21600, // -06:00
	"America/Guayaquil":                -18000, // -05:00
	"America/Guyana":                   -14400, // -04:00
	"America/Halifax":                  -14400, // -04:00
	"America/Havana":                   -18000, // -05:00
	"America/Hermosillo":               -25200, // -07:00
	"America/Indiana/Indianapolis":     -18000, // -05:00
	"America/Indiana/Knox":             -21600, // -06:00
	"America/Indiana/Marengo":          -18000, // -05:00
	"America/Indiana/Petersburg":       -18000, // -05:00
	"America/Indiana/Tell_City":        -21600, // -06:00
	"America/Indiana/Vevay":            -18000, // -05:00
	"America/Indiana/Vincennes":        -18000, // -05:00
	"America/Indiana/Winamac":          -18000, // -05:00
	"America/Indianapolis":             -18000, // -05:00
	"America/Inuvik":                   -25200, // -07:00
	"America/Iqaluit":                  -18000, // -05:00
	"America/Jamaica":                  -18000, // -05:00
	"America/Jujuy":                    -10800, // -03:00
	"America/Juneau":                   -32400, // -09:00
	"America/Kentucky/Louisville":      -18000, // -05:00
	"America/Kentucky/Monticello":      -18000, // -05:00
	"America/Knox_IN":                  -21600, // -06:00
	"America/Kralendijk":               -14400, // -04:00
	"America/La_Paz":                   -14400, // -04:00
	"America/Lima":                     -18000, // -05:00
	"America/Los_Angeles":              -28800, // -08:00
	"America/Louisville":               -18000, // -05:00
	"America/Lower_Princes":            -14400, // -04:00
	"America/Maceio":                   -10800, // -03:00
	"America/Managua":                  -21600, // -06:00
	"America/Manaus":                   -14400, // -04:00
	"America/Marigot":                  -14400, // -04:00
	"America/Martinique":               -14400, // -04:00
	"America/Matamoros":                -21600, // -06:00
	"America/Mazatlan":                 -25200, // -07:00
	"America/Mendoza":                  -10800, // -03:00
	"America/Menominee":                -21600, // -06:00
	"America/Merida":                   -21600, // -06:00
	"America/Metlakatla":               -32400, // -09:00
	"America/Mexico_City":              -21600, // -06:00
	"America/Miquelon":                 -10800, // -03:00
	"America/Moncton":                  -14400, // -04:00
	"America/Monterrey":                -21600, // -06:00
	"America/Montevideo":               -10800, // -03:00
	"America/Montreal":                 -18000, // -05:00
	"America/Montserrat":               -14400, // -04:00
	"America/Nassau":                   -18000, // -05:00
	"America/New_York":                 -18000, // -05:00
	"America/Nipigon":                  -18000, // -05:00
	"America/Nome":                     -32400, // -09:00
	"America/Noronha":                  -7200,  // -02:00
	"America/North_Dakota/Beulah":      -21600, // -06:00
	"America/North_Dakota/Center":      -21600, // -06:00
	"America/North_Dakota/New_Salem":   -21600, // -06:00
	"America/Nuuk":                     -10800, // -03:00
	"America/Ojinaga":                  -25200, // -07:00
	"America/Panama":                   -18000, // -05:00
	"America/Pangnirtung":              -18000, // -05:00
	"America/Paramaribo":               -10800, // -03:00
	"America/Phoenix":                  -25200, // -07:00
	"America/Port-au-Prince":           -18000, // -05:00
	"America/Port_of_Spain":            -14400, // -04:00
	"America/Porto_Acre":               -18000, // -05:00
	"America/Porto_Velho":              -14400, // -04:00
	"America/Puerto_Rico":              -14400, // -04:00
	"America/Punta_Arenas":             -10800, // -03:00
	"America/Rainy_River":              -21600, // -06:00
	"America/Rankin_Inlet":             -21600, // -06:00
	"America/Recife":                   -10800, // -03:00
	"America/Regina":                   -21600, // -06:00
	"America/Resolute":                 -21600, // -06:00
	"America/Rio_Branco":               -18000, // -05:00
	"America/Rosario":                  -10800, // -03:00
	"America/Santa_Isabel":             -28800, // -08:00
	"America/Santarem":                 -10800, // -03:00
	"America/Santiago":                 -14400, // -04:00
	"America/Santo_Domingo":            -14400, // -04:00
	"America/Sao_Paulo":                -10800, // -03:00
	"America/Scoresbysund":             -3600,  // -01:00
	"America/Shiprock":                 -25200, // -07:00
	"America/Sitka":                    -32400, // -09:00
	"America/St_Barthelemy":            -14400, // -04:00
	"America/St_Johns":                 -12600, // -03:30
	"America/St_Kitts":                 -14400, // -04:00
	"America/St_Lucia":                 -14400, // -04:00
	"America/St_Thomas":                -14400, // -04:00
	"America/St_Vincent":               -14400, // -04:00
	"America/Swift_Current":            -21600, // -06:00
	"America/Tegucigalpa":              -21600, // -06:00
	"America/Thule":                    -14400, // -04:00
	"America/Thunder_Bay":              -18000, // -05:00
	"America/Tijuana":                  -28800, // -08:00
	"America/Toronto":                  -18000, // -05:00
	"America/Tortola":                  -14400, // -04:00
	"America/Vancouver":                -28800, // -08:00
	"America/Virgin":                   -14400, // -04:00
	"America/Whitehorse":               -25200, // -07:00
	"America/Winnipeg":                 -21600, // -06:00
	"America/Yakutat":                  -32400, // -09:00
	"America/Yellowknife":              -25200, // -07:00
	"Antarctica/Casey":                 39600,  // +11:00
	"Antarctica/Davis":                 25200,  // +07:00
	"Antarctica/DumontDUrville":        36000,  // +10:00
	"Antarctica/Macquarie":             36000,  // +10:00
	"Antarctica/Mawson":                18000,  // +05:00
	"Antarctica/McMurdo":               43200,  // +12:00
	"Antarctica/Palmer":                -10800, // -03:00
	"Antarctica/Rothera":               -10800, // -03:00
	"Antarctica/South_Pole":            43200,  // +12:00
	"Antarctica/Syowa":                 10800,  // +03:00
	"Antarctica/Troll":                 0,      // +00:00
	"Antarctica/Vostok":                21600,  // +06:00
	"Arctic/Longyearbyen":              3600,   // +01:00
	"Asia/Aden":                        10800,  // +03:00
	"Asia/Almaty":                      21600,  // +06:00
	"Asia/Amman":                       7200,   // +02:00
	"Asia/Anadyr":                      43200,  // +12:00
	"Asia/Aqtau":                       18000,  // +05:00
	"Asia/Aqtobe":                      18000,  // +05:00
	"Asia/Ashgabat":                    18000,  // +05:00
	"Asia/Ashkhabad":                   18000,  // +05:00
	"Asia/Atyrau":                      18000,  // +05:00
	"Asia/Baghdad":                     10800,  // +03:00
	"Asia/Bahrain":                     10800,  // +03:00
	"Asia/Baku":                        14400,  // +04:00
	"Asia/Bangkok":                     25200,  // +07:00
	"Asia/Barnaul":                     25200,  // +07:00
	"Asia/Beirut":                      7200,   // +02:00
	"Asia/Bishkek":                     21600,  // +06:00
	"Asia/Brunei":                      28800,  // +08:00
	"Asia/Calcutta":                    19800,  // +05:30
	"Asia/Chita":                       32400,  // +09:00
	"Asia/Choibalsan":                  28800,  // +08:00
	"Asia/Chongqing":                   28800,  // +08:00
	"Asia/Chungking":                   28800,  // +08:00
	"Asia/Colombo":                     19800,  // +05:30
	"Asia/Dacca":                       21600,  // +06:00
	"Asia/Damascus":                    7200,   // +02:00
	"Asia/Dhaka":                       21600,  // +06:00
	"Asia/Dili":                        32400,  // +09:00
	"Asia/Dubai":                       14400,  // +04:00
	"Asia/Dushanbe":                    18000,  // +05:00
	"Asia/Famagusta":                   7200,   // +02:00
	"Asia/Gaza":                        7200,   // +02:00
	"Asia/Harbin":                      28800,  // +08:00
	"Asia/Hebron":                      7200,   // +02:00
	"Asia/Ho_Chi_Minh":                 25200,  // +07:00
	"Asia/Hong_Kong":                   28800,  // +08:00
	"Asia/Hovd":                        25200,  // +07:00
	"Asia/Irkutsk":                     28800,  // +08:00
	"Asia/Istanbul":                    10800,  // +03:00
	"Asia/Jakarta":                     25200,  // +07:00
	"Asia/Jayapura":                    32400,  // +09:00
	"Asia/Jerusalem":                   7200,   // +02:00
	"Asia/Kabul":                       16200,  // +04:30
	"Asia/Kamchatka":                   43200,  // +12:00
	"Asia/Karachi":                     18000,  // +05:00
	"Asia/Kashgar":                     21600,  // +06:00
	"Asia/Kathmandu":                   20700,  // +05:45
	"Asia/Katmandu":                    20700,  // +05:45
	"Asia/Khandyga":                    32400,  // +09:00
	"Asia/Kolkata":                     19800,  // +05:30
	"Asia/Krasnoyarsk":                 25200,  // +07:00
	"Asia/Kuala_Lumpur":                28800,  // +08:00
	"Asia/Kuching":                     28800,  // +08:00
	"Asia/Kuwait":                      10800,  // +03:00
	"Asia/Macao":                       28800,  // +08:00
	"Asia/Macau":                       28800,  // +08:00
	"Asia/Magadan":                     39600,  // +11:00
	"Asia/Makassar":                    28800,  // +08:00
	"Asia/Manila":                      28800,  // +08:00
	"Asia/Muscat":                      14400,  // +04:00
	"Asia/Nicosia":                     7200,   // +02:00
	"Asia/Novokuznetsk":                25200,  // +07:00
	"Asia/Novosibirsk":                 25200,  // +07:00
	"Asia/Omsk":                        21600,  // +06:00
	"Asia/Oral":                        18000,  // +05:00
	"Asia/Phnom_Penh":                  25200,  // +07:00
	"Asia/Pontianak":                   25200,  // +07:00
	"Asia/Pyongyang":                   32400,  // +09:00
	"Asia/Qatar":                       10800,  // +03:00
	"Asia/Qostanay":                    21600,  // +06:00
	"Asia/Qyzylorda":                   18000,  // +05:00
	"Asia/Rangoon":                     23400,  // +06:30
	"Asia/Riyadh":                      10800,  // +03:00
	"Asia/Saigon":                      25200,  // +07:00
	"Asia/Sakhalin":                    39600,  // +11:00
	"Asia/Samarkand":                   18000,  // +05:00
	"Asia/Seoul":                       32400,  // +09:00
	"Asia/Shanghai":                    28800,  // +08:00
	"Asia/Singapore":                   28800,  // +08:00
	"Asia/Srednekolymsk":               39600,  // +11:00
	"Asia/Taipei":                      28800,  // +08:00
	"Asia/Tashkent":                    18000,  // +05:00
	"Asia/Tbilisi":                     14400,  // +04:00
	"Asia/Tehran":                      12600,  // +03:30
	"Asia/Tel_Aviv":                    7200,   // +02:00
	"Asia/Thimbu":                      21600,  // +06:00
	"Asia/Thimphu":                     21600,  // +06:00
	"Asia/Tokyo":                       32400,  // +09:00
	"Asia/Tomsk":                       25200,  // +07:00
	"Asia/Ujung_Pandang":               28800,  // +08:00
	"Asia/Ulaanbaatar":                 28800,  // +08:00
	"Asia/Ulan_Bator":                  28800,  // +08:00
	"Asia/Urumqi":                      21600,  // +06:00
	"Asia/Ust-Nera":                    36000,  // +10:00
	"Asia/Vientiane":                   25200,  // +07:00
	"Asia/Vladivostok":                 36000,  // +10:00
	"Asia/Yakutsk":                     32400,  // +09:00
	"Asia/Yangon":                      23400,  // +06:30
	"Asia/Yekaterinburg":               18000,  // +05:00
	"Asia/Yerevan":                     14400,  // +04:00
	"Atlantic/Azores":                  -3600,  // -01:00
	"Atlantic/Bermuda":                 -14400, // -04:00
	"Atlantic/Canary":                  0,      // +00:00
	"Atlantic/Cape_Verde":              -3600,  // -01:00
	"Atlantic/Faeroe":                  0,      // +00:00
	"Atlantic/Faroe":                   0,      // +00:00
	"Atlantic/Jan_Mayen":               3600,   // +01:00
	"Atlantic/Madeira":                 0,      // +00:00
	"Atlantic/Reykjavik":               0,      // +00:00
	"Atlantic/South_Georgia":           -7200,  // -02:00
	"Atlantic/St_Helena":               0,      // +00:00
	"Atlantic/Stanley":                 -10800, // -03:00
	"Australia/ACT":                    36000,  // +10:00
	"Australia/Adelaide":               34200,  // +09:30
	"Australia/Brisbane":               36000,  // +10:00
	"Australia/Broken_Hill":            34200,  // +09:30
	"Australia/Canberra":               36000,  // +10:00
	"Australia/Currie":                 36000,  // +10:00
	"Australia/Darwin":                 34200,  // +09:30
	"Australia/Eucla":                  31500,  // +08:45
	"Australia/Hobart":                 36000,  // +10:00
	"Australia/LHI":                    37800,  // +10:30
	"Australia/Lindeman":               36000,  // +10:00
	"Australia/Lord_Howe":              37800,  // +10:30
	"Australia/Melbourne":              36000,  // +10:00
	"Australia/North":                  34200,  // +09:30
	"Australia/NSW":                    36000,  // +10:00
	"Australia/Perth":                  28800,  // +08:00
	"Australia/Queensland":             36000,  // +10:00
	"Australia/South":                  34200,  // +09:30
	"Australia/Sydney":                 36000,  // +10:00
	"Australia/Tasmania":               36000,  // +10:00
	"Australia/Victoria":               36000,  // +10:00
	"Australia/West":                   28800,  // +08:00
	"Australia/Yancowinna":             34200,  // +09:30
	"Brazil/Acre":                      -18000, // -05:00
	"Brazil/DeNoronha":                 -7200,  // -02:00
	"Brazil/East":                      -10800, // -03:00
	"Brazil/West":                      -14400, // -04:00
	"Canada/Atlantic":                  -14400, // -04:00
	"Canada/Central":                   -21600, // -06:00
	"Canada/Eastern":                   -18000, // -05:00
	"Canada/Mountain":                  -25200, // -07:00
	"Canada/Newfoundland":              -12600, // -03:30
	"Canada/Pacific":                   -28800, // -08:00
	"Canada/Saskatchewan":              -21600, // -06:00
	"Canada/Yukon":                     -25200, // -07:00
	"CET":                              3600,   // +01:00
	"Chile/Continental":                -14400, // -04:00
	"Chile/EasterIsland":               -21600, // -06:00
	"CST6CDT":                          -21600, // -06:00
	"Cuba":                             -18000, // -05:00
	"Egypt":                            7200,   // +02:00
	"Eire":                             3600,   // +01:00
	"EST5EDT":                          -18000, // -05:00
	"Etc/GMT":                          0,      // +00:00
	"Etc/GMT+0":                        0,      // +00:00
	"Etc/GMT+1":                        -3600,  // -01:00
	"Etc/GMT+10":                       -36000, // -10:00
	"Etc/GMT+11":                       -39600, // -11:00
	"Etc/GMT+12":                       -43200, // -12:00
	"Etc/GMT+2":                        -7200,  // -02:00
	"Etc/GMT+3":                        -10800, // -03:00
	"Etc/GMT+4":                        -14400, // -04:00
	"Etc/GMT+5":                        -18000, // -05:00
	"Etc/GMT+6":                        -21600, // -06:00
	"Etc/GMT+7":                        -25200, // -07:00
	"Etc/GMT+8":                        -28800, // -08:00
	"Etc/GMT+9":                        -32400, // -09:00
	"Etc/GMT-0":                        0,      // +00:00
	"Etc/GMT-1":                        3600,   // +01:00
	"Etc/GMT-10":                       36000,  // +10:00
	"Etc/GMT-11":                       39600,  // +11:00
	"Etc/GMT-12":                       43200,  // +12:00
	"Etc/GMT-13":                       46800,  // +13:00
	"Etc/GMT-14":                       50400,  // +14:00
	"Etc/GMT-2":                        7200,   // +02:00
	"Etc/GMT-3":                        10800,  // +03:00
	"Etc/GMT-4":                        14400,  // +04:00
	"Etc/GMT-5":                        18000,  // +05:00
	"Etc/GMT-6":                        21600,  // +06:00
	"Etc/GMT-7":                        25200,  // +07:00
	"Etc/GMT-8":                        28800,  // +08:00
	"Etc/GMT-9":                        32400,  // +09:00
	"Etc/GMT0":                         0,      // +00:00
	"Etc/Greenwich":                    0,      // +00:00
	"Etc/UCT":                          0,      // +00:00
	"Etc/Universal":                    0,      // +00:00
	"Etc/UTC":                          0,      // +00:00
	"Etc/Zulu":                         0,      // +00:00
	"Europe/Amsterdam":                 3600,   // +01:00
	"Europe/Andorra":                   3600,   // +01:00
	"Europe/Astrakhan":                 14400,  // +04:00
	"Europe/Athens":                    7200,   // +02:00
	"Europe/Belfast":                   0,      // +00:00
	"Europe/Belgrade":                  3600,   // +01:00
	"Europe/Berlin":                    3600,   // +01:00
	"Europe/Bratislava":                3600,   // +01:00
	"Europe/Brussels":                  3600,   // +01:00
	"Europe/Bucharest":                 7200,   // +02:00
	"Europe/Budapest":                  3600,   // +01:00
	"Europe/Busingen":                  3600,   // +01:00
	"Europe/Chisinau":                  7200,   // +02:00
	"Europe/Copenhagen":                3600,   // +01:00
	"Europe/Dublin":                    3600,   // +01:00
	"Europe/Gibraltar":                 3600,   // +01:00
	"Europe/Guernsey":                  0,      // +00:00
	"Europe/Helsinki":                  7200,   // +02:00
	"Europe/Isle_of_Man":               0,      // +00:00
	"Europe/Istanbul":                  10800,  // +03:00
	"Europe/Jersey":                    0,      // +00:00
	"Europe/Kaliningrad":               7200,   // +02:00
	"Europe/Kiev":                      7200,   // +02:00
	"Europe/Kirov":                     10800,  // +03:00
	"Europe/Lisbon":                    0,      // +00:00
	"Europe/Ljubljana":                 3600,   // +01:00
	"Europe/London":                    0,      // +00:00
	"Europe/Luxembourg":                3600,   // +01:00
	"Europe/Madrid":                    3600,   // +01:00
	"Europe/Malta":                     3600,   // +01:00
	"Europe/Mariehamn":                 7200,   // +02:00
	"Europe/Minsk":                     10800,  // +03:00
	"Europe/Monaco":                    3600,   // +01:00
	"Europe/Moscow":                    10800,  // +03:00
	"Europe/Nicosia":                   7200,   // +02:00
	"Europe/Oslo":                      3600,   // +01:00
	"Europe/Paris":                     3600,   // +01:00
	"Europe/Podgorica":                 3600,   // +01:00
	"Europe/Prague":                    3600,   // +01:00
	"Europe/Riga":                      7200,   // +02:00
	"Europe/Rome":                      3600,   // +01:00
	"Europe/Samara":                    14400,  // +04:00
	"Europe/San_Marino":                3600,   // +01:00
	"Europe/Sarajevo":                  3600,   // +01:00
	"Europe/Saratov":                   14400,  // +04:00
	"Europe/Simferopol":                10800,  // +03:00
	"Europe/Skopje":                    3600,   // +01:00
	"Europe/Sofia":                     7200,   // +02:00
	"Europe/Stockholm":                 3600,   // +01:00
	"Europe/Tallinn":                   7200,   // +02:00
	"Europe/Tirane":                    3600,   // +01:00
	"Europe/Tiraspol":                  7200,   // +02:00
	"Europe/Ulyanovsk":                 14400,  // +04:00
	"Europe/Uzhgorod":                  7200,   // +02:00
	"Europe/Vaduz":                     3600,   // +01:00
	"Europe/Vatican":                   3600,   // +01:00
	"Europe/Vienna":                    3600,   // +01:00
	"Europe/Vilnius":                   7200,   // +02:00
	"Europe/Volgograd":                 10800,  // +03:00
	"Europe/Warsaw":                    3600,   // +01:00
	"Europe/Zagreb":                    3600,   // +01:00
	"Europe/Zaporozhye":                7200,   // +02:00
	"Europe/Zurich":                    3600,   // +01:00
	"Factory":                          0,      // +00:00
	"GB":                               0,      // +00:00
	"GB-Eire":                          0,      // +00:00
	"GMT+0":                            0,      // +00:00
	"GMT-0":                            0,      // +00:00
	"GMT0":                             0,      // +00:00
	"Greenwich":                        0,      // +00:00
	"Hongkong":                         28800,  // +08:00
	"Iceland":                          0,      // +00:00
	"Indian/Antananarivo":              10800,  // +03:00
	"Indian/Chagos":                    21600,  // +06:00
	"Indian/Christmas":                 25200,  // +07:00
	"Indian/Cocos":                     23400,  // +06:30
	"Indian/Comoro":                    10800,  // +03:00
	"Indian/Kerguelen":                 18000,  // +05:00
	"Indian/Mahe":                      14400,  // +04:00
	"Indian/Maldives":                  18000,  // +05:00
	"Indian/Mauritius":                 14400,  // +04:00
	"Indian/Mayotte":                   10800,  // +03:00
	"Indian/Reunion":                   14400,  // +04:00
	"Iran":                             12600,  // +03:30
	"Israel":                           7200,   // +02:00
	"Jamaica":                          -18000, // -05:00
	"Japan":                            32400,  // +09:00
	"Kwajalein":                        43200,  // +12:00
	"Libya":                            7200,   // +02:00
	"Mexico/BajaNorte":                 -28800, // -08:00
	"Mexico/BajaSur":                   -25200, // -07:00
	"Mexico/General":                   -21600, // -06:00
	"MST7MDT":                          -25200, // -07:00
	"Navajo":                           -25200, // -07:00
	"NZ":                               43200,  // +12:00
	"NZ-CHAT":                          45900,  // +12:45
	"Pacific/Apia":                     46800,  // +13:00
	"Pacific/Auckland":                 43200,  // +12:00
	"Pacific/Bougainville":             39600,  // +11:00
	"Pacific/Chatham":                  45900,  // +12:45
	"Pacific/Chuuk":                    36000,  // +10:00
	"Pacific/Easter":                   -21600, // -06:00
	"Pacific/Efate":                    39600,  // +11:00
	"Pacific/Enderbury":                46800,  // +13:00
	"Pacific/Fakaofo":                  46800,  // +13:00
	"Pacific/Fiji":                     43200,  // +12:00
	"Pacific/Funafuti":                 43200,  // +12:00
	"Pacific/Galapagos":                -21600, // -06:00
	"Pacific/Gambier":                  -32400, // -09:00
	"Pacific/Guadalcanal":              39600,  // +11:00
	"Pacific/Guam":                     36000,  // +10:00
	"Pacific/Honolulu":                 -36000, // -10:00
	"Pacific/Johnston":                 -36000, // -10:00
	"Pacific/Kiritimati":               50400,  // +14:00
	"Pacific/Kosrae":                   39600,  // +11:00
	"Pacific/Kwajalein":                43200,  // +12:00
	"Pacific/Majuro":                   43200,  // +12:00
	"Pacific/Marquesas":                -34200, // -09:30
	"Pacific/Midway":                   -39600, // -11:00
	"Pacific/Nauru":                    43200,  // +12:00
	"Pacific/Niue":                     -39600, // -11:00
	"Pacific/Norfolk":                  39600,  // +11:00
	"Pacific/Noumea":                   39600,  // +11:00
	"Pacific/Pago_Pago":                -39600, // -11:00
	"Pacific/Palau":                    32400,  // +09:00
	"Pacific/Pitcairn":                 -28800, // -08:00
	"Pacific/Pohnpei":                  39600,  // +11:00
	"Pacific/Ponape":                   39600,  // +11:00
	"Pacific/Port_Moresby":             36000,  // +10:00
	"Pacific/Rarotonga":                -36000, // -10:00
	"Pacific/Saipan":                   36000,  // +10:00
	"Pacific/Samoa":                    -39600, // -11:00
	"Pacific/Tahiti":                   -36000, // -10:00
	"Pacific/Tarawa":                   43200,  // +12:00
	"Pacific/Tongatapu":                46800,  // +13:00
	"Pacific/Truk":                     36000,  // +10:00
	"Pacific/Wake":                     43200,  // +12:00
	"Pacific/Wallis":                   43200,  // +12:00
	"Pacific/Yap":                      36000,  // +10:00
	"Poland":                           3600,   // +01:00
	"Portugal":                         0,      // +00:00
	"PRC":                              28800,  // +08:00
	"PST8PDT":                          -28800, // -08:00
	"ROC":                              28800,  // +08:00
	"ROK":                              32400,  // +09:00
	"Singapore":                        28800,  // +08:00
	"Turkey":                           10800,  // +03:00
	"UCT":                              0,      // +00:00
	"Universal":                        0,      // +00:00
	"US/Alaska":                        -32400, // -09:00
	"US/Aleutian":                      -36000, // -10:00
	"US/Arizona":                       -25200, // -07:00
	"US/Central":                       -21600, // -06:00
	"US/East-Indiana":                  -18000, // -05:00
	"US/Eastern":                       -18000, // -05:00
	"US/Hawaii":                        -36000, // -10:00
	"US/Indiana-Starke":                -21600, // -06:00
	"US/Michigan":                      -18000, // -05:00
	"US/Mountain":                      -25200, // -07:00
	"US/Pacific":                       -28800, // -08:00
	"US/Samoa":                         -39600, // -11:00
	"W-SU":                             10800,  // +03:00
	"Zulu":                             0,      // +00:00

	"ACDT":  37800,  // -10:30 - Australian Central Daylight Saving Time
	"ACST":  34200,  // -09:30 - Australian Central Standard Time
	"ACT":   -18000, // -05:00 - Acre Time
	"ACWST": 31500,  // -08:45 - Australian Central Western Standard Time (unofficial)
	"ADT":   -10800, // -03:00 - Atlantic Daylight Time
	"AEDT":  39600,  // -11:00 - Australian Eastern Daylight Saving Time
	"AEST":  36000,  // -10:00 - Australian Eastern Standard Time
	"AET":   36000,  // -10:00 - Australian Eastern Time
	"AFT":   16200,  // -04:30 - Afghanistan Time
	"AKDT":  -28800, // -08:00 - Alaska Daylight Time
	"AKST":  -32400, // -09:00 - Alaska Standard Time
	"ALMT":  21600,  // -06:00 - Alma-Ata Time
	"AMST":  -10800, // -03:00 - Amazon Summer Time (Brazil)
	"AMT":   -14400, // -04:00 - Amazon Time (Brazil)
	"ANAT":  43200,  // -12:00 - Anadyr Time
	"AQTT":  18000,  // -05:00 - Aqtobe Time
	"ART":   -10800, // -03:00 - Argentina Time
	"AST":   10800,  // -03:00 - Arabia Standard Time
	"AWST":  28800,  // -08:00 - Australian Western Standard Time
	"AZOST": 0,      // -00:00 - Azores Summer Time
	"AZOT":  -3600,  // -01:00 - Azores Standard Time
	"AZT":   14400,  // -04:00 - Azerbaijan Time
	"BNT":   28800,  // -08:00 - Brunei Time
	"BIOT":  21600,  // -06:00 - British Indian Ocean Time
	"BIT":   -43200, // -12:00 - Baker Island Time
	"BOT":   -14400, // -04:00 - Bolivia Time
	"BRST":  -7200,  // -02:00 - Brasília Summer Time
	"BRT":   -10800, // -03:00 - Brasília Time
	"BST":   21600,  // -06:00 - Bangladesh Standard Time
	"BTT":   21600,  // -06:00 - Bhutan Time
	"CAT":   7200,   // -02:00 - Central Africa Time
	"CCT":   23400,  // -06:30 - Cocos Islands Time
	"CDT":   -18000, // -05:00 - Central Daylight Time (North America)
	"CEST":  7200,   // -02:00 - Central European Summer Time (Cf. HAEC)
	"CHADT": 49500,  // -13:45 - Chatham Daylight Time
	"CHAST": 45900,  // -12:45 - Chatham Standard Time
	"CHOT":  28800,  // -08:00 - Choibalsan Standard Time
	"CHOST": 32400,  // -09:00 - Choibalsan Summer Time
	"CHST":  36000,  // -10:00 - Chamorro Standard Time
	"CHUT":  36000,  // -10:00 - Chuuk Time
	"CIST":  -28800, // -08:00 - Clipperton Island Standard Time
	"CKT":   -36000, // -10:00 - Cook Island Time
	"CLST":  -10800, // -03:00 - Chile Summer Time
	"CLT":   -14400, // -04:00 - Chile Standard Time
	"COST":  -14400, // -04:00 - Colombia Summer Time
	"COT":   -18000, // -05:00 - Colombia Time
	"CST":   -21600, // -06:00 - Central Standard Time (North America)
	"CT":    -21600, // -06:00 - Central Time
	"CVT":   -3600,  // -01:00 - Cape Verde Time
	"CWST":  31500,  // -08:45 - Central Western Standard Time (Australia) unofficial
	"CXT":   25200,  // -07:00 - Christmas Island Time
	"DAVT":  25200,  // -07:00 - Davis Time
	"DDUT":  36000,  // -10:00 - Dumont d'Urville Time
	"DFT":   3600,   // -01:00 - AIX-specific equivalent of Central European Time
	"EASST": -18000, // -05:00 - Easter Island Summer Time
	"EAST":  -21600, // -06:00 - Easter Island Standard Time
	"EAT":   10800,  // -03:00 - East Africa Time
	"ECT":   -14400, // -04:00 - Eastern Caribbean Time (does not recognise DST)
	"EDT":   -14400, // -04:00 - Eastern Daylight Time (North America)
	"EEST":  10800,  // -03:00 - Eastern European Summer Time
	"EET":   7200,   // -02:00 - Eastern European Time
	"EGST":  0,      // -00:00 - Eastern Greenland Summer Time
	"EGT":   -3600,  // -01:00 - Eastern Greenland Time
	"EST":   -18000, // -05:00 - Eastern Standard Time (North America)
	"FET":   10800,  // -03:00 - Further-eastern European Time
	"FJT":   43200,  // -12:00 - Fiji Time
	"FKST":  -10800, // -03:00 - Falkland Islands Summer Time
	"FKT":   -14400, // -04:00 - Falkland Islands Time
	"FNT":   -7200,  // -02:00 - Fernando de Noronha Time
	"GALT":  -21600, // -06:00 - Galápagos Time
	"GAMT":  -32400, // -09:00 - Gambier Islands Time
	"GET":   14400,  // -04:00 - Georgia Standard Time
	"GFT":   -10800, // -03:00 - French Guiana Time
	"GILT":  43200,  // -12:00 - Gilbert Island Time
	"GIT":   -32400, // -09:00 - Gambier Island Time
	"GMT":   0,      // -00:00 - Greenwich Mean Time
	"GST":   14400,  // -04:00 - Gulf Standard Time
	"GYT":   -14400, // -04:00 - Guyana Time
	"HDT":   -32400, // -09:00 - Hawaii–Aleutian Daylight Time
	"HAEC":  7200,   // -02:00 - Heure Avancée d'Europe Centrale French-language name for CEST
	"HST":   -36000, // -10:00 - Hawaii–Aleutian Standard Time
	"HKT":   28800,  // -08:00 - Hong Kong Time
	"HMT":   18000,  // -05:00 - Heard and McDonald Islands Time
	"HOVST": 28800,  // -08:00 - Hovd Summer Time (not used from 2017-present)
	"HOVT":  25200,  // -07:00 - Hovd Time
	"ICT":   25200,  // -07:00 - Indochina Time
	"IDLW":  -43200, // -12:00 - International Day Line West time zone
	"IDT":   10800,  // -03:00 - Israel Daylight Time
	"IOT":   10800,  // -03:00 - Indian Ocean Time
	"IRDT":  16200,  // -04:30 - Iran Daylight Time
	"IRKT":  28800,  // -08:00 - Irkutsk Time
	"IRST":  12600,  // -03:30 - Iran Standard Time
	"IST":   19800,  // -05:30 - Indian Standard Time
	"JST":   32400,  // -09:00 - Japan Standard Time
	"KALT":  7200,   // -02:00 - Kaliningrad Time
	"KGT":   21600,  // -06:00 - Kyrgyzstan Time
	"KOST":  39600,  // -11:00 - Kosrae Time
	"KRAT":  25200,  // -07:00 - Krasnoyarsk Time
	"KST":   32400,  // -09:00 - Korea Standard Time
	"LHST":  37800,  // -10:30 - Lord Howe Standard Time
	"LINT":  50400,  // -14:00 - Line Islands Time
	"MAGT":  43200,  // -12:00 - Magadan Time
	"MART":  -34200, // -09:30 - Marquesas Islands Time
	"MAWT":  18000,  // -05:00 - Mawson Station Time
	"MDT":   -21600, // -06:00 - Mountain Daylight Time (North America)
	"MET":   3600,   // -01:00 - Middle European Time (same zone as CET)
	"MEST":  7200,   // -02:00 - Middle European Summer Time (same zone as CEST)
	"MHT":   43200,  // -12:00 - Marshall Islands Time
	"MIST":  39600,  // -11:00 - Macquarie Island Station Time
	"MIT":   -34200, // -09:30 - Marquesas Islands Time
	"MMT":   23400,  // -06:30 - Myanmar Standard Time
	"MSK":   10800,  // -03:00 - Moscow Time
	"MST":   -25200, // -07:00 - Mountain Standard Time (North America)
	"MUT":   14400,  // -04:00 - Mauritius Time
	"MVT":   18000,  // -05:00 - Maldives Time
	"MYT":   28800,  // -08:00 - Malaysia Time
	"NCT":   39600,  // -11:00 - New Caledonia Time
	"NDT":   -9000,  // -02:30 - Newfoundland Daylight Time
	"NFT":   39600,  // -11:00 - Norfolk Island Time
	"NOVT":  25200,  // -07:00 - Novosibirsk Time
	"NPT":   20700,  // -05:45 - Nepal Time
	"NST":   -12600, // -03:30 - Newfoundland Standard Time
	"NT":    -12600, // -03:30 - Newfoundland Time
	"NUT":   -39600, // -11:00 - Niue Time
	"NZDT":  46800,  // -13:00 - New Zealand Daylight Time
	"NZST":  43200,  // -12:00 - New Zealand Standard Time
	"OMST":  21600,  // -06:00 - Omsk Time
	"ORAT":  18000,  // -05:00 - Oral Time
	"PDT":   -25200, // -07:00 - Pacific Daylight Time (North America)
	"PET":   -18000, // -05:00 - Peru Time
	"PETT":  43200,  // -12:00 - Kamchatka Time
	"PGT":   36000,  // -10:00 - Papua New Guinea Time
	"PHOT":  46800,  // -13:00 - Phoenix Island Time
	"PHT":   28800,  // -08:00 - Philippine Time
	"PKT":   18000,  // -05:00 - Pakistan Standard Time
	"PMDT":  -7200,  // -02:00 - Saint Pierre and Miquelon Daylight Time
	"PMST":  -10800, // -03:00 - Saint Pierre and Miquelon Standard Time
	"PONT":  39600,  // -11:00 - Pohnpei Standard Time
	"PST":   -28800, // -08:00 - Pacific Standard Time (North America)
	"PWT":   32400,  // -09:00 - Palau Time
	"PYST":  -10800, // -03:00 - Paraguay Summer Time
	"PYT":   -14400, // -04:00 - Paraguay Time
	"RET":   14400,  // -04:00 - Réunion Time
	"ROTT":  -10800, // -03:00 - Rothera Research Station Time
	"SAKT":  39600,  // -11:00 - Sakhalin Island Time
	"SAMT":  14400,  // -04:00 - Samara Time
	"SAST":  7200,   // -02:00 - South African Standard Time
	"SBT":   39600,  // -11:00 - Solomon Islands Time
	"SCT":   14400,  // -04:00 - Seychelles Time
	"SDT":   -36000, // -10:00 - Samoa Daylight Time
	"SGT":   28800,  // -08:00 - Singapore Time
	"SLST":  19800,  // -05:30 - Sri Lanka Standard Time
	"SRET":  39600,  // -11:00 - Srednekolymsk Time
	"SRT":   -10800, // -03:00 - Suriname Time
	"SST":   28800,  // -08:00 - Singapore Standard Time
	"SYOT":  10800,  // -03:00 - Showa Station Time
	"TAHT":  -36000, // -10:00 - Tahiti Time
	"THA":   25200,  // -07:00 - Thailand Standard Time
	"TFT":   18000,  // -05:00 - French Southern and Antarctic Time
	"TJT":   18000,  // -05:00 - Tajikistan Time
	"TKT":   46800,  // -13:00 - Tokelau Time
	"TLT":   32400,  // -09:00 - Timor Leste Time
	"TMT":   18000,  // -05:00 - Turkmenistan Time
	"TRT":   10800,  // -03:00 - Turkey Time
	"TOT":   46800,  // -13:00 - Tonga Time
	"TVT":   43200,  // -12:00 - Tuvalu Time
	"ULAST": 32400,  // -09:00 - Ulaanbaatar Summer Time
	"ULAT":  28800,  // -08:00 - Ulaanbaatar Standard Time
	"UTC":   0,      // -00:00 - Coordinated Universal Time
	"UYST":  -7200,  // -02:00 - Uruguay Summer Time
	"UYT":   -10800, // -03:00 - Uruguay Standard Time
	"UZT":   18000,  // -05:00 - Uzbekistan Time
	"VET":   -14400, // -04:00 - Venezuelan Standard Time
	"VLAT":  36000,  // -10:00 - Vladivostok Time
	"VOLT":  14400,  // -04:00 - Volgograd Time
	"VOST":  21600,  // -06:00 - Vostok Station Time
	"VUT":   39600,  // -11:00 - Vanuatu Time
	"WAKT":  43200,  // -12:00 - Wake Island Time
	"WAST":  7200,   // -02:00 - West Africa Summer Time
	"WAT":   3600,   // -01:00 - West Africa Time
	"WEST":  3600,   // -01:00 - Western European Summer Time
	"WET":   0,      // -00:00 - Western European Time
	"WIB":   25200,  // -07:00 - Western Indonesian Time
	"WIT":   32400,  // -09:00 - Eastern Indonesian Time
	"WITA":  28800,  // -08:00 - Central Indonesia Time
	"WGST":  -7200,  // -02:00 - West Greenland Summer Time
	"WGT":   -10800, // -03:00 - West Greenland Time
	"WST":   28800,  // -08:00 - Western Standard Time
	"YAKT":  32400,  // -09:00 - Yakutsk Time
	"YEKT":  18000,  // -05:00 - Yekaterinburg Time

}
