package local

import (
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// ASCIIQR renders a URL as a QR code made of half-block characters, small
// enough for a terminal (two rows of modules per text line).
func ASCIIQR(url string) string {
	q, err := qrcode.New(url, qrcode.Low)
	if err != nil {
		return ""
	}
	q.DisableBorder = false
	return strings.TrimRight(q.ToSmallString(false), "\n")
}
