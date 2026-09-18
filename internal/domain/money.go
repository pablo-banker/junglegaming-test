package domain

import (
	"encoding/json"
	"regexp"

	"github.com/shopspring/decimal"
)

var (
	moneyAmountPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,17})\.[0-9]{2}$`)
	maxMoneyAmount     = decimal.RequireFromString("999999999999999999.99")
)

type Money struct {
	amount   decimal.Decimal
	currency Currency
}

// ParseMoney creates Money from an external fixed-scale decimal string.
func ParseMoney(amount string, currency Currency) (Money, error) {
	if !currency.IsValid() {
		return Money{}, ErrInvalidCurrency
	}

	if !moneyAmountPattern.MatchString(amount) {
		return Money{}, ErrInvalidMoney
	}

	value, err := decimal.NewFromString(amount)
	if err != nil {
		return Money{}, ErrInvalidMoney
	}

	return newMoney(value, currency)
}

// Zero creates a zero monetary value for a valid currency.
func Zero(currency Currency) (Money, error) {
	if !currency.IsValid() {
		return Money{}, ErrInvalidCurrency
	}

	return Money{
		amount:   decimal.Zero,
		currency: currency,
	}, nil
}

// Add returns the sum of two monetary values.
func (m Money) Add(other Money) (Money, error) {
	if err := m.validateOperation(other); err != nil {
		return Money{}, err
	}

	return newMoney(m.amount.Add(other.amount), m.currency)
}

// Sub returns the difference between two monetary values.
func (m Money) Sub(other Money) (Money, error) {
	if err := m.validateOperation(other); err != nil {
		return Money{}, err
	}

	return newMoney(m.amount.Sub(other.amount), m.currency)
}

// Negate returns the same monetary value with the opposite sign.
func (m Money) Negate() (Money, error) {
	if !m.IsValid() {
		return Money{}, ErrInvalidMoney
	}

	return newMoney(m.amount.Neg(), m.currency)
}

// Compare compares two monetary values.
func (m Money) Compare(other Money) (int, error) {
	if err := m.validateOperation(other); err != nil {
		return 0, err
	}

	return m.amount.Cmp(other.amount), nil
}

// Equal reports whether amount and currency are equal.
func (m Money) Equal(other Money) bool {
	return m.currency.Equal(other.currency) && m.amount.Equal(other.amount)
}

// IsValid reports whether Money contains a valid currency and supported amount.
func (m Money) IsValid() bool {
	return m.currency.IsValid() && isMoneyAmountInRange(m.amount)
}

// IsZero reports whether the amount is zero.
func (m Money) IsZero() bool {
	return m.amount.IsZero()
}

// IsPositive reports whether the amount is greater than zero.
func (m Money) IsPositive() bool {
	return m.amount.IsPositive()
}

// IsNegative reports whether the amount is less than zero.
func (m Money) IsNegative() bool {
	return m.amount.IsNegative()
}

// Amount returns the amount with exactly two decimal places.
func (m Money) Amount() string {
	return m.amount.StringFixed(2)
}

// Currency returns the Money currency.
func (m Money) Currency() Currency {
	return m.currency
}

// MarshalJSON serializes Money using the external contract.
func (m Money) MarshalJSON() ([]byte, error) {
	if !m.IsValid() {
		return nil, ErrInvalidMoney
	}

	return json.Marshal(struct {
		Amount   string `json:"amount"`
		Currency string `json:"currency"`
	}{
		Amount:   m.Amount(),
		Currency: m.currency.Code(),
	})
}

// newMoney creates Money for internal calculations.
func newMoney(amount decimal.Decimal, currency Currency) (Money, error) {
	if !currency.IsValid() {
		return Money{}, ErrInvalidCurrency
	}

	if !isMoneyAmountInRange(amount) {
		return Money{}, ErrInvalidMoney
	}

	return Money{
		amount:   amount,
		currency: currency,
	}, nil
}

// validateOperation validates Money values before arithmetic or comparison.
func (m Money) validateOperation(other Money) error {
	if !m.IsValid() || !other.IsValid() {
		return ErrInvalidMoney
	}

	if !m.currency.Equal(other.currency) {
		return ErrCurrencyMismatch
	}

	return nil
}

// isMoneyAmountInRange reports whether an amount fits NUMERIC(20,2).
func isMoneyAmountInRange(amount decimal.Decimal) bool {
	return amount.Abs().Cmp(maxMoneyAmount) <= 0
}
