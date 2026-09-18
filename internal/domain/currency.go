package domain

import "regexp"

var currencyCodePattern = regexp.MustCompile(`^[A-Z]{3}$`)

type Currency struct {
	code string
}

// NewCurrency creates a currency from a valid three-letter uppercase code.
func NewCurrency(code string) (Currency, error) {
	if !currencyCodePattern.MatchString(code) {
		return Currency{}, ErrInvalidCurrency
	}

	return Currency{code: code}, nil
}

// IsValid reports whether the currency contains a valid code.
func (c Currency) IsValid() bool {
	return currencyCodePattern.MatchString(c.code)
}

// Code returns the currency code.
func (c Currency) Code() string {
	return c.code
}

// Equal reports whether two currencies are equal.
func (c Currency) Equal(other Currency) bool {
	return c.code == other.code
}

// String returns the currency code.
func (c Currency) String() string {
	return c.code
}
