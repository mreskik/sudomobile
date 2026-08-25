package order

import "strconv"

// mustFloat: parse string numeric(20,2) hasil pricing.CalculateLine() -- SELALU valid (angka
// yang diformat sendiri lewat strconv.FormatFloat di pricing package), jadi error di sini
// artinya bug internal, bukan input user -- aman dianggap 0 daripada panic, tapi harusnya gak
// pernah kejadian.
func mustFloat(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', 2, 64)
}
