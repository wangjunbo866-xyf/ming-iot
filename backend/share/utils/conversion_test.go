package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecimalToAny(t *testing.T) {
	num := int64(1699865466064867328)
	bit62 := DecimalToAny(num, 62)
	ans := AnyToDecimal(bit62, 62)
	if ans != num {
		t.Fatalf("DecimalToAny/AnyToDecimal round trip = %d, want %d", ans, num)
	}
}

func TestToLen(t *testing.T) {
	src := FillZeroToLen("123", 11)
	ans := AnyToDecimal("00000000i23", 62)
	bit62 := DecimalToAny(ans, 62)
	assert.Equal(t, "00000000123", src)
	assert.Equal(t, int64(69319), ans)
	assert.Equal(t, "i23", bit62)
}
