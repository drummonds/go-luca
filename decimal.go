package luca

import "github.com/shopspring/decimal"

// MovementExponent returns the exponent at which a movement amount should be
// stored, given the exponents of the from and to accounts. It picks the
// higher-precision (more negative) exponent so no information is lost.
func MovementExponent(fromExp, toExp int) int {
	if fromExp < toExp {
		return fromExp
	}
	return toExp
}

// IntToDecimal converts an integer amount with the given exponent to a
// shopspring Decimal.  For example IntToDecimal(1500, -2) → 15.00.
func IntToDecimal(amount Amount, exponent int) decimal.Decimal {
	return decimal.New(int64(amount), int32(exponent))
}

// DecimalToInt converts a shopspring Decimal to an integer amount at the given
// exponent, truncating toward zero.
// For example DecimalToInt(15.007, -2) → 1500.
func DecimalToInt(d decimal.Decimal, exponent int) Amount {
	// Shift: d * 10^(-exponent)  →  integer in smallest unit
	shifted := d.Shift(int32(-exponent))
	return Amount(shifted.IntPart())
}

// ScaleAmount converts an integer amount from one exponent to another.
// For example ScaleAmount(150000, -5, -2) → 150  (sub-pence truncated).
func ScaleAmount(amount Amount, fromExponent, toExponent int) Amount {
	if fromExponent == toExponent {
		return amount
	}
	d := IntToDecimal(amount, fromExponent)
	return DecimalToInt(d, toExponent)
}
