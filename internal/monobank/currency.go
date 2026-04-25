package monobank

import "strings"

// MapCurrency maps an ISO-4217 alpha-3 currency code (case-insensitive) to its
// ISO-4217 numeric equivalent expected by the Monobank acquiring API. Unknown
// currencies return an *APIError with ErrCode="unsupported_currency".
func MapCurrency(code string) (int, error) {
	switch strings.ToUpper(code) {
	case "UAH":
		return 980, nil
	case "USD":
		return 840, nil
	case "EUR":
		return 978, nil
	default:
		return 0, &APIError{ErrCode: "unsupported_currency", ErrText: code}
	}
}
