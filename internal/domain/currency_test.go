//go:build unit

package domain

import (
	"errors"
	"testing"
)

// TestNewCurrencyCreatesValidCurrency verifies valid currency creation.
func TestNewCurrencyCreatesValidCurrency(t *testing.T) {
	currency, err := NewCurrency("BRL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if currency.Code() != "BRL" {
		t.Errorf("expected BRL, got %s", currency.Code())
	}

	if !currency.IsValid() {
		t.Error("expected currency to be valid")
	}
}

// TestNewCurrencyRejectsInvalidCodes verifies invalid currency formats are rejected.
func TestNewCurrencyRejectsInvalidCodes(t *testing.T) {
	tests := []string{
		"",
		"BR",
		"BRLL",
		"brl",
		"BrL",
		"123",
		" BRL",
		"BRL ",
	}

	for _, code := range tests {
		t.Run(code, func(t *testing.T) {
			_, err := NewCurrency(code)

			if !errors.Is(err, ErrInvalidCurrency) {
				t.Fatalf("expected ErrInvalidCurrency, got %v", err)
			}
		})
	}
}

// TestCurrencyEqualComparesCodes verifies currency equality by code.
func TestCurrencyEqualComparesCodes(t *testing.T) {
	brl, err := NewCurrency("BRL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	usd, err := NewCurrency("USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !brl.Equal(brl) {
		t.Error("expected BRL to equal BRL")
	}

	if brl.Equal(usd) {
		t.Error("expected BRL to differ from USD")
	}
}

// TestCurrencyZeroValueIsInvalid verifies the zero value cannot represent a currency.
func TestCurrencyZeroValueIsInvalid(t *testing.T) {
	var currency Currency

	if currency.IsValid() {
		t.Error("expected zero-value currency to be invalid")
	}
}
