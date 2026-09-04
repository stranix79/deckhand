package deck

import "strings"

// natural sorts file names the way a human expects: "2-intro.html" before
// "10-end.html". Digit runs are compared as numbers, everything else
// byte-wise and case-insensitively. No locale is involved so that a deck
// sorts identically on every OS.
type natural []string

func (n natural) Len() int           { return len(n) }
func (n natural) Swap(i, j int)      { n[i], n[j] = n[j], n[i] }
func (n natural) Less(i, j int) bool { return NaturalLess(n[i], n[j]) }

// NaturalLess is the comparison used by natural.
func NaturalLess(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	for a != "" && b != "" {
		ca, cb := a[0], b[0]
		if isDigit(ca) && isDigit(cb) {
			na, ra := leadingNumber(a)
			nb, rb := leadingNumber(b)
			if na != nb {
				return na < nb
			}
			a, b = ra, rb
			continue
		}
		if ca != cb {
			return ca < cb
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// leadingNumber returns the numeric value of the digit run at the start of s
// (leading zeros ignored, capped to avoid overflow) and the rest of s.
func leadingNumber(s string) (uint64, string) {
	var n uint64
	i := 0
	for i < len(s) && isDigit(s[i]) {
		if n < 1<<60 {
			n = n*10 + uint64(s[i]-'0')
		}
		i++
	}
	return n, s[i:]
}
