package data

import "testing"

func TestMoneyText(t *testing.T) {
	cases := []struct {
		side int
		v    float64
		want string
	}{
		{RU, 410, "36,9 млрд ₽"},
		{UA, 150, "6,15 млрд ₴"},
		{RU, 6.5, "585 млн ₽"},
		{RU, 0.0025, "225 тыс. ₽"},
		{UA, 1100, "45,1 млрд ₴"},
		{RU, 0, "0 ₽"},
	}
	for _, c := range cases {
		if got := MoneyText(c.side, c.v); got != c.want {
			t.Errorf("MoneyText(%d, %v) = %q, want %q", c.side, c.v, got, c.want)
		}
	}
}
