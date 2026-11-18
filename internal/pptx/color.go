package pptx

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// defaultClrMap is how a master maps the logical colours used by shapes
// (text 1, background 1…) to the theme slots. Dark templates swap them.
var defaultClrMap = map[string]string{"bg1": "lt1", "tx1": "dk1", "bg2": "lt2", "tx2": "dk2"}

// presetColors are the few named colours people actually use.
var presetColors = map[string]string{
	"black": "000000", "white": "FFFFFF", "red": "FF0000", "green": "008000", "blue": "0000FF",
	"yellow": "FFFF00", "gray": "808080", "grey": "808080", "orange": "FFA500", "purple": "800080",
	"ltGray": "D3D3D3", "dkGray": "A9A9A9", "cyan": "00FFFF", "magenta": "FF00FF",
}

var fontName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,60}$`)

// loadTheme reads the colour scheme, the body font and the line widths of
// the first theme.
func (d *Deck) loadTheme(presRels map[string]rel) {
	d.theme = map[string]string{"dk1": "000000", "lt1": "FFFFFF"}
	for _, r := range presRels {
		if !strings.HasSuffix(r.typ, "/theme") || r.external {
			continue
		}
		root, err := d.part(r.target)
		if err != nil {
			return
		}
		for _, slot := range root.find("clrScheme").children {
			if c := slot.child("srgbClr"); c != nil {
				d.theme[slot.name] = c.attr("val")
			} else if c := slot.child("sysClr"); c != nil {
				d.theme[slot.name] = c.attr("lastClr")
			}
		}
		if f := root.find("minorFont").child("latin").attr("typeface"); fontName.MatchString(f) {
			d.font = f
		}
		for _, ln := range root.find("lnStyleLst").all("ln") {
			w, _ := strconv.ParseFloat(ln.attr("w"), 64)
			d.lineWidths = append(d.lineWidths, w)
		}
		return
	}
}

// colorOf resolves the colour held by an element such as <a:solidFill>,
// <a:fillRef> or <a:gs>, applying the usual modifiers. ok is false when the
// element holds no colour this converter understands.
func (d *Deck) colorOf(holder *node, clrMap map[string]string) (css string, ok bool) {
	if holder == nil {
		return "", false
	}
	var c *node
	hex := ""
	for _, ch := range holder.children {
		switch ch.name {
		case "srgbClr":
			hex = ch.attr("val")
		case "sysClr":
			hex = ch.attr("lastClr")
		case "prstClr":
			hex = presetColors[ch.attr("val")]
		case "schemeClr":
			slot := ch.attr("val")
			if m, mapped := clrMap[slot]; mapped {
				slot = m
			} else if m, mapped := defaultClrMap[slot]; mapped {
				slot = m
			}
			hex = d.theme[slot]
		default:
			continue
		}
		c = ch
		break
	}
	if c == nil || len(hex) != 6 {
		return "", false
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return "", false
	}
	r, g, b := float64(v>>16&0xff), float64(v>>8&0xff), float64(v&0xff)
	alpha := 1.0
	for _, m := range c.children {
		val, err := strconv.ParseFloat(m.attr("val"), 64)
		if err != nil {
			continue
		}
		f := val / 100000
		switch m.name {
		case "alpha":
			alpha = f
		case "shade": // towards black
			r, g, b = r*f, g*f, b*f
		case "tint": // towards white
			r, g, b = r*f+255*(1-f), g*f+255*(1-f), b*f+255*(1-f)
		case "lumMod":
			h, s, l := rgbToHSL(r, g, b)
			r, g, b = hslToRGB(h, s, math.Min(1, l*f))
		case "lumOff":
			h, s, l := rgbToHSL(r, g, b)
			r, g, b = hslToRGB(h, s, math.Min(1, math.Max(0, l+f)))
		}
	}
	ri, gi, bi := clamp255(r), clamp255(g), clamp255(b)
	if alpha < 0.995 {
		return fmt.Sprintf("rgba(%d,%d,%d,%s)", ri, gi, bi, strconv.FormatFloat(math.Round(alpha*100)/100, 'f', -1, 64)), true
	}
	return fmt.Sprintf("#%02x%02x%02x", ri, gi, bi), true
}

func clamp255(v float64) int {
	return int(math.Round(math.Min(255, math.Max(0, v))))
}

func rgbToHSL(r, g, b float64) (h, s, l float64) {
	r, g, b = r/255, g/255, b/255
	hi, lo := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	l = (hi + lo) / 2
	if hi == lo {
		return 0, 0, l
	}
	d := hi - lo
	if l > 0.5 {
		s = d / (2 - hi - lo)
	} else {
		s = d / (hi + lo)
	}
	switch hi {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h / 6, s, l
}

func hslToRGB(h, s, l float64) (r, g, b float64) {
	if s == 0 {
		return l * 255, l * 255, l * 255
	}
	q := l + s - l*s
	if l < 0.5 {
		q = l * (1 + s)
	}
	p := 2*l - q
	hue := func(t float64) float64 {
		switch {
		case t < 0:
			t++
		case t > 1:
			t--
		}
		switch {
		case t < 1.0/6:
			return p + (q-p)*6*t
		case t < 0.5:
			return q
		case t < 2.0/3:
			return p + (q-p)*(2.0/3-t)*6
		}
		return p
	}
	return hue(h+1.0/3) * 255, hue(h) * 255, hue(h-1.0/3) * 255
}
