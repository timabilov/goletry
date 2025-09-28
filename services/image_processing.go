package services

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
)

// WhitenBackgroundFeathered applies a soft threshold to whiten the background.
// It uses a transition range to smoothly blend pixels towards white, avoiding hard edges.
// It also protects a central area of the image.
// - imageBytes: The input image as a byte slice.
// - lowerThreshold: The brightness value (0-255) at which the whitening effect begins.
// - upperThreshold: The brightness value (0-255) at which pixels become pure white.
// - centralProtectionRatio: The central area (0.0-1.0) to protect from any changes.
func WhitenBackgroundFeathered(imageBytes []byte, lowerThreshold, upperThreshold uint8, centralProtectionRatio float64) ([]byte, error) {
	if lowerThreshold >= upperThreshold {
		return nil, fmt.Errorf("lowerThreshold must be less than upperThreshold")
	}
	if centralProtectionRatio < 0.0 || centralProtectionRatio > 1.0 {
		return nil, fmt.Errorf("centralProtectionRatio must be between 0.0 and 1.0")
	}

	img, _, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	bounds := img.Bounds()
	width, height := bounds.Max.X, bounds.Max.Y
	newImg := image.NewRGBA(bounds)

	// Calculate the protected rectangle
	protectedWidth := int(float64(width) * centralProtectionRatio)
	protectedHeight := int(float64(height) * centralProtectionRatio)
	x0 := (width - protectedWidth) / 2
	y0 := (height - protectedHeight) / 2
	x1 := x0 + protectedWidth
	y1 := y0 + protectedHeight

	// Pre-calculate the transition range to avoid division in the loop
	transitionRange := float64(upperThreshold - lowerThreshold)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			originalColor := img.At(x, y)

			// If inside the protected area, just copy the pixel
			if x >= x0 && x < x1 && y >= y0 && y < y1 {
				newImg.Set(x, y, originalColor)
				continue
			}

			// Outside the protected area, apply the feathered whitening logic
			r, g, b, a := originalColor.RGBA()
			r8 := uint8(r >> 8)
			g8 := uint8(g >> 8)
			b8 := uint8(b >> 8)
			a8 := uint8(a >> 8)

			// Use luminance for a more accurate measure of brightness
			luminance := 0.299*float64(r8) + 0.587*float64(g8) + 0.114*float64(b8)

			if luminance <= float64(lowerThreshold) {
				// Pixel is too dark, leave it untouched
				newImg.Set(x, y, originalColor)
			} else if luminance >= float64(upperThreshold) {
				// Pixel is bright enough, make it pure white
				newImg.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: a8})
			} else {
				// THE CORE LOGIC: We are in the transition zone.
				// Calculate how far into the transition range this pixel is (0.0 to 1.0)
				blendFactor := (luminance - float64(lowerThreshold)) / transitionRange

				// Linearly interpolate each channel towards white (255)
				// new_color = original_color * (1 - factor) + white * factor
				newR := uint8(math.Round(float64(r8)*(1.0-blendFactor) + 255.0*blendFactor))
				newG := uint8(math.Round(float64(g8)*(1.0-blendFactor) + 255.0*blendFactor))
				newB := uint8(math.Round(float64(b8)*(1.0-blendFactor) + 255.0*blendFactor))

				newImg.Set(x, y, color.RGBA{R: newR, G: newG, B: newB, A: a8})
			}
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, newImg); err != nil {
		return nil, fmt.Errorf("failed to encode image to png: %w", err)
	}
	return buf.Bytes(), nil
}
