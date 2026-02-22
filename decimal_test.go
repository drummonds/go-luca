package luca

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestMovementExponent(t *testing.T) {
	tests := []struct {
		from, to, want int
	}{
		{-2, -2, -2},
		{-2, -5, -5},
		{-5, -2, -5},
		{0, -2, -2},
		{-3, -3, -3},
	}
	for _, tt := range tests {
		got := MovementExponent(tt.from, tt.to)
		if got != tt.want {
			t.Errorf("MovementExponent(%d, %d) = %d, want %d", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestIntToDecimal(t *testing.T) {
	tests := []struct {
		amount   int64
		exponent int
		want     string
	}{
		{1500, -2, "15.00"},
		{100000, -5, "1.00000"},
		{42, 0, "42"},
		{-250, -2, "-2.50"},
	}
	for _, tt := range tests {
		got := IntToDecimal(tt.amount, tt.exponent)
		if got.StringFixed(int32(-tt.exponent)) != tt.want {
			t.Errorf("IntToDecimal(%d, %d) = %s, want %s", tt.amount, tt.exponent, got.StringFixed(int32(-tt.exponent)), tt.want)
		}
	}
}

func TestDecimalToInt(t *testing.T) {
	tests := []struct {
		d        string
		exponent int
		want     int64
	}{
		{"15.00", -2, 1500},
		{"1.00000", -5, 100000},
		{"15.007", -2, 1500}, // truncates
		{"-2.50", -2, -250},
	}
	for _, tt := range tests {
		d, _ := decimal.NewFromString(tt.d)
		got := DecimalToInt(d, tt.exponent)
		if got != tt.want {
			t.Errorf("DecimalToInt(%s, %d) = %d, want %d", tt.d, tt.exponent, got, tt.want)
		}
	}
}

func TestScaleAmount(t *testing.T) {
	tests := []struct {
		amount   int64
		from, to int
		want     int64
	}{
		{1500, -2, -2, 1500},    // same exponent
		{1500, -2, -5, 1500000}, // scale up precision
		{1500000, -5, -2, 1500}, // scale down (exact)
		{1500007, -5, -2, 1500}, // scale down (truncates)
		{-250, -2, -5, -250000}, // negative amount
	}
	for _, tt := range tests {
		got := ScaleAmount(tt.amount, tt.from, tt.to)
		if got != tt.want {
			t.Errorf("ScaleAmount(%d, %d, %d) = %d, want %d", tt.amount, tt.from, tt.to, got, tt.want)
		}
	}
}
