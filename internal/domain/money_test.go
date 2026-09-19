//go:build unit

package domain

import (
	"encoding/json"
	"errors"
	"testing"
)

// TestParseMoneyCreatesValidMoney verifies valid fixed-scale parsing.
func TestParseMoneyCreatesValidMoney(t *testing.T) {
	brl, err := NewCurrency("BRL")
	if err != nil {
		t.Fatalf("unexpected currency error: %v", err)
	}

	money, err := ParseMoney("25.00", brl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if money.Amount() != "25.00" {
		t.Errorf("expected 25.00, got %s", money.Amount())
	}

	if !money.Currency().Equal(brl) {
		t.Error("expected BRL currency")
	}
}

// TestParseMoneyRejectsInvalidExternalAmounts verifies strict external parsing.
func TestParseMoneyRejectsInvalidExternalAmounts(t *testing.T) {
	brl, err := NewCurrency("BRL")
	if err != nil {
		t.Fatalf("unexpected currency error: %v", err)
	}

	tests := []string{
		"",
		"25",
		"25.0",
		"25.000",
		"-25.00",
		"025.00",
		"1e2",
		"NaN",
		"Infinity",
		"1000000000000000000.00",
	}

	for _, amount := range tests {
		t.Run(amount, func(t *testing.T) {
			_, err := ParseMoney(amount, brl)

			if !errors.Is(err, ErrInvalidMoney) {
				t.Fatalf("expected ErrInvalidMoney, got %v", err)
			}
		})
	}
}

// TestMoneyArithmetic verifies exact addition and subtraction.
func TestMoneyArithmetic(t *testing.T) {
	brl, _ := NewCurrency("BRL")

	oneHundred, _ := ParseMoney("100.00", brl)
	twentyFive, _ := ParseMoney("25.00", brl)

	sum, err := oneHundred.Add(twentyFive)
	if err != nil {
		t.Fatalf("unexpected add error: %v", err)
	}

	if sum.Amount() != "125.00" {
		t.Errorf("expected 125.00, got %s", sum.Amount())
	}

	difference, err := twentyFive.Sub(oneHundred)
	if err != nil {
		t.Fatalf("unexpected sub error: %v", err)
	}

	if difference.Amount() != "-75.00" {
		t.Errorf("expected -75.00, got %s", difference.Amount())
	}

	if !difference.IsNegative() {
		t.Error("expected negative internal difference")
	}
}

// TestMoneyRejectsCurrencyMismatch verifies arithmetic requires matching currencies.
func TestMoneyRejectsCurrencyMismatch(t *testing.T) {
	brl, _ := NewCurrency("BRL")
	usd, _ := NewCurrency("USD")

	brlMoney, _ := ParseMoney("10.00", brl)
	usdMoney, _ := ParseMoney("10.00", usd)

	_, err := brlMoney.Add(usdMoney)

	if !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("expected ErrCurrencyMismatch, got %v", err)
	}
}

// TestMoneyRejectsOverflow verifies calculations stay inside NUMERIC(20,2).
func TestMoneyRejectsOverflow(t *testing.T) {
	brl, _ := NewCurrency("BRL")

	maximum, _ := ParseMoney("999999999999999999.99", brl)
	cent, _ := ParseMoney("0.01", brl)

	_, err := maximum.Add(cent)

	if !errors.Is(err, ErrInvalidMoney) {
		t.Fatalf("expected ErrInvalidMoney, got %v", err)
	}
}

// TestMoneyJSONUsesExternalContract verifies Money JSON serialization.
func TestMoneyJSONUsesExternalContract(t *testing.T) {
	brl, _ := NewCurrency("BRL")
	money, _ := ParseMoney("25.00", brl)

	data, err := json.Marshal(money)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}

	expected := `{"amount":"25.00","currency":"BRL"}`

	if string(data) != expected {
		t.Errorf("expected %s, got %s", expected, data)
	}
}
