package types

// CURRENCY_CODES_SYMBOLS is a map of 3 digit ISO currency codes to their symbols
// TODO add more currencies or look for a library
var CURRENCY_CODES_SYMBOLS = map[string]string{
	"usd": "$",
	"eur": "€",
	"gbp": "£",
	"aud": "AU$",
	"cad": "CA$",
	"chf": "CHF",
	"sek": "kr",
	"nzd": "NZ$",
	"hkd": "HK$",
	"sgd": "S$",
	"jpy": "¥",
	"cny": "¥",
	"inr": "₹",
	"brl": "R$",
	"rub": "₽",
	"mxn": "MX$",
	"krw": "₩",
	"try": "₺",
	"zar": "R",
	"myr": "RM",
}

// GetCurrencySymbol returns the symbol for a given currency code
// if the code is not found, it returns the code itself
func GetCurrencySymbol(code string) string {
	if symbol, ok := CURRENCY_CODES_SYMBOLS[code]; ok {
		return symbol
	}
	return code
}
