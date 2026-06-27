package brand

import (
	"regexp"
	"strings"

	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/PuerkitoBio/goquery"
)

// RGB represents a color in Red, Green, Blue color space.
// Each component ranges from 0 to 255.
// Example: RGB{R: 255, G: 0, B: 0} represents pure red.
type RGB struct {
	R int
	G int
	B int
}

// HSL represents a color in Hue, Saturation, Lightness color space.
// H ranges from 0 to 360 (degrees), S and L range from 0 to 100 (percent).
// Example: HSL{H: 0, S: 100, L: 50} represents pure red.
type HSL struct {
	H int
	S int
	L int
}

// processHex normalizes a hex color string to a 6-character uppercase hex format.
// It handles the following inputs:
//   - "#FF0000" -> "FF0000"
//   - "FF0000" -> "FF0000"
//   - "F00" -> "FF0000" (3-digit expansion)
//   - "#F00" -> "FF0000"
//
// Returns the normalized 6-character hex string without the # prefix.
// Example: processHex("#ff6700") returns "FF6700"
func processHex(color string) string {
	hex := strings.TrimPrefix(strings.ToUpper(color), "#")

	if len(hex) == 3 {
		return string(hex[0]) + string(hex[0]) + string(hex[1]) + string(hex[1]) + string(hex[2]) + string(hex[2])
	}
	if len(hex) == 4 {
		return string(hex[0]) + string(hex[0]) + string(hex[1]) + string(hex[1]) + string(hex[2]) + string(hex[2])
	}
	if len(hex) > 6 {
		hex = hex[:6]
	}
	return hex
}

// hexToRGB converts a hex color string to its RGB components.
// Input should be a 6-character hex string (without #).
// Returns an RGB struct with R, G, B values (0-255).
//
// Example:
//
//	hexToRGB("FF0000") returns RGB{R: 255, G: 0, B: 0} (pure red)
//	hexToRGB("#00FF00") returns RGB{R: 0, G: 255, B: 0} (pure green)
func hexToRGB(hex string) RGB {
	hex = processHex(hex)
	r := parseHexByte(hex[0:2])
	g := parseHexByte(hex[2:4])
	b := parseHexByte(hex[4:6])
	return RGB{R: r, G: g, B: b}
}

// parseHexByte converts a 2-character hex string to its decimal value.
// Input should be exactly 2 hex characters (0-9, A-F).
//
// Example:
//
//	parseHexByte("FF") returns 255
//	parseHexByte("0A") returns 10
func parseHexByte(s string) int {
	var val int
	for _, c := range s {
		val *= 16
		switch {
		case c >= '0' && c <= '9':
			val += int(c - '0')
		case c >= 'A' && c <= 'F':
			val += int(c-'A') + 10
		}
	}
	return val
}

// rgbToHSL converts RGB color values to HSL color space.
// Input values should be 0-255 for R, G, B.
// Returns HSL struct with H (0-360 degrees), S and L (0-100 percent).
//
// Example:
//
//	rgbToHSL(255, 0, 0) returns HSL{H: 0, S: 100, L: 50} (red)
//	rgbToHSL(0, 255, 0) returns HSL{H: 120, S: 100, L: 50} (green)
//	rgbToHSL(128, 128, 128) returns HSL{H: 0, S: 0, L: 50} (gray)
func rgbToHSL(r, g, b int) HSL {
	rFloat := float64(r) / 255.0
	gFloat := float64(g) / 255.0
	bFloat := float64(b) / 255.0

	max := maxFloat(rFloat, gFloat, bFloat)
	min := minFloat(rFloat, gFloat, bFloat)

	var h, s, l float64
	l = (max + min) / 2

	if max == min {
		h = 0
		s = 0
	} else {
		d := max - min
		if l > 0.5 {
			s = d / (2 - max - min)
		} else {
			s = d / (max + min)
		}

		switch max {
		case rFloat:
			h = (gFloat - bFloat) / d
			if gFloat < bFloat {
				h += 6
			}
		case gFloat:
			h = (bFloat-rFloat)/d + 2
		case bFloat:
			h = (rFloat-gFloat)/d + 4
		}
		h /= 6
	}

	return HSL{
		H: int(h * 360),
		S: int(s * 100),
		L: int(l * 100),
	}
}

func maxFloat(a, b, c float64) float64 {
	if a > b {
		if a > c {
			return a
		}
		return c
	}
	if b > c {
		return b
	}
	return c
}

func minFloat(a, b, c float64) float64 {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// colorDistance calculates the perceptual distance between two colors.
// It uses a weighted combination of RGB Euclidean distance and HSL distance.
// HSL is weighted more heavily (2x) as it better represents human color perception.
//
// The formula is: RGB_distance + (HSL_distance * 2)
//
// Example:
//
//	colorDistance(255, 0, 0, 0, 100, 50, 254, 0, 0, 0, 100, 50)
//	returns a small number (colors are very similar)
func colorDistance(r1, g1, b1 int, h1, s1, l1 int, r2, g2, b2 int, h2, s2, l2 int) float64 {
	rgbDist := float64((r1-r2)*(r1-r2) + (g1-g2)*(g1-g2) + (b1-b2)*(b1-b2))
	hslDist := float64((h1-h2)*(h1-h2) + (s1-s2)*(s1-s2) + (l1-l2)*(l1-l2))
	return rgbDist + hslDist*2
}

// findClosestColorName finds the human-readable name for a hex color.
// It first checks for an exact match in the colorNamesMap.
// If no exact match, it calculates the perceptual distance to all colors
// in the map and returns the name of the closest match.
//
// The algorithm uses weighted RGB+HSL distance for better color perception.
//
// Example:
//
//	findClosestColorName("#ff6700") returns "Blaze Orange" (exact match)
//	findClosestColorName("#FF8080") returns "Light Coral" (closest match)
func findClosestColorName(hex string) string {
	processedHex := processHex(hex)

	// Try exact match first (O(1) lookup)
	if name, ok := colorNamesMap[processedHex]; ok {
		return name
	}

	// Calculate distances to all colors and find closest
	rgb := hexToRGB(processedHex)
	hsl := rgbToHSL(rgb.R, rgb.G, rgb.B)

	minDistance := -1.0
	closestName := "#" + processedHex

	for nameHex, name := range colorNamesMap {
		nameRGB := hexToRGB(nameHex)
		nameHSL := rgbToHSL(nameRGB.R, nameRGB.G, nameRGB.B)

		dist := colorDistance(
			rgb.R, rgb.G, rgb.B, hsl.H, hsl.S, hsl.L,
			nameRGB.R, nameRGB.G, nameRGB.B, nameHSL.H, nameHSL.S, nameHSL.L,
		)

		if minDistance == -1 || dist < minDistance {
			minDistance = dist
			closestName = name
		}
	}

	return closestName
}

// getColorName is a convenience wrapper for findClosestColorName.
// It takes any hex color string and returns its human-readable name.
//
// Example:
//
//	getColorName("#ff6700") returns "Blaze Orange"
//	getColorName("F54900") returns "Flame Orange"
func getColorName(hex string) string {
	return findClosestColorName(hex)
}

// extractColors extracts brand colors from an HTML document.
// It parses all hex color codes from <style> tags, removes duplicates,
// validates them, and assigns human-readable names.
//
// Returns a slice of BrandColor structs with hex codes and names.
// If no colors are found, returns a default black color.
//
// Example return:
//
//	[]BrandColor{
//	  {Hex: "#ff6700", Name: "Blaze Orange"},
//	  {Hex: "#0a0a0a", Name: "Rich Black"},
//	}
func extractColors(doc *goquery.Document) []types.BrandColor {
	var colors []types.BrandColor
	colorSet := make(map[string]bool)

	cssColors := extractColorsFromCSS(doc)
	for _, c := range cssColors {
		normalized := strings.ToLower(c)
		if !colorSet[normalized] && isValidColor(normalized) {
			colorSet[normalized] = true
			colorName := getColorName(normalized)
			colors = append(colors, types.BrandColor{
				Hex:  normalized,
				Name: colorName,
			})
		}
	}

	if len(colors) == 0 {
		colors = append(colors, types.BrandColor{Hex: "#000000", Name: "Black"})
	}

	return colors
}

// extractColorsFromCSS extracts all hex color codes from <style> tags in the document.
// It uses regex to find patterns like #RGB, #RRGGBB, #RRGGBBAA.
//
// Returns a slice of hex color strings (with # prefix).
//
// Example:
//
//	extractColorsFromCSS(doc) returns ["#ff6700", "#0a0a0a", "#ffffff"]
func extractColorsFromCSS(doc *goquery.Document) []string {
	var colors []string
	colorRegex := regexp.MustCompile(`#[0-9a-fA-F]{3,8}`)

	doc.Find("style").Each(func(i int, s *goquery.Selection) {
		css := s.Text()
		matches := colorRegex.FindAllString(css, -1)
		colors = append(colors, matches...)
	})

	return colors
}

// isValidColor validates whether a hex color string is properly formatted.
// Valid formats are:
//   - #RGB (4 characters, e.g., "#F00")
//   - #RRGGBB (7 characters, e.g., "#FF0000")
//   - #RRGGBBAA (9 characters, e.g., "#FF0000FF")
//
// Returns true if valid, false otherwise.
//
// Example:
//
//	isValidColor("#ff6700") returns true
//	isValidColor("#F00") returns true
//	isValidColor("ff6700") returns false (missing #)
//	isValidColor("#ff") returns false (invalid length)
func isValidColor(c string) bool {
	if strings.HasPrefix(c, "#") {
		if len(c) == 4 || len(c) == 7 || len(c) == 9 {
			return true
		}
	}
	return false
}
