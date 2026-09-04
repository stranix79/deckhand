// Command genicons draws the remote's home-screen icons: an ink rounded
// square with a brass dot and a sea ring, the same marks as the landing.
// Run: go run ./tools/genicons web/remote
package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

func main() {
	dir := "web/remote"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	for _, size := range []int{180, 192, 512} {
		img := draw(size)
		f, err := os.Create(filepath.Join(dir, "icon-"+itoa(size)+".png")) //nolint:gosec // dev tool, path is an argument
		if err != nil {
			panic(err)
		}
		if err := png.Encode(f, img); err != nil {
			panic(err)
		}
		_ = f.Close()
	}
}

func draw(n int) *image.NRGBA {
	ink := color.NRGBA{0x12, 0x23, 0x3B, 0xFF}
	brass := color.NRGBA{0xC9, 0xA2, 0x27, 0xFF}
	sea := color.NRGBA{0x1E, 0x6E, 0x74, 0xFF}
	fog := color.NRGBA{0xE9, 0xEE, 0xEF, 0xFF}
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	f := float64(n)
	cx, cy := f*0.5, f*0.5
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			c := ink
			// A "slide" rectangle in fog, 16:9, with a sea header band and a brass laser dot.
			l, t, r, b := f*0.14, f*0.26, f*0.86, f*0.74
			if px >= l && px <= r && py >= t && py <= b {
				c = fog
				if py <= t+f*0.10 {
					c = sea
				}
			}
			d := math.Hypot(px-(cx+f*0.18), py-(cy+f*0.08))
			if d <= f*0.075 {
				c = brass
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
