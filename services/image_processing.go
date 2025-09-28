package services

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
)

// WhitenBackgroundWithProtectedCenter whitens the background of an image while protecting a central rectangular area.
//   - imageBytes: The input image as a byte slice.
//   - threshold: Any pixel (R,G,B) outside the protected area with values >= threshold will be turned white.
//   - centralProtectionRatio: A value from 0.0 to 1.0. A ratio of 0.7 means the central 70% of the
//     image's width and height will be protected from changes.
func WhitenBackgroundWithProtectedCenter(imageBytes []byte, threshold uint8, centralProtectionRatio float64) ([]byte, error) {
	if centralProtectionRatio < 0.0 || centralProtectionRatio > 1.0 {
		return nil, fmt.Errorf("centralProtectionRatio must be between 0.0 and 1.0")
	}

	// 1. Decode the input bytes into an image.Image
	img, _, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// 2. Get the image bounds and create a new image
	bounds := img.Bounds()
	width, height := bounds.Max.X, bounds.Max.Y
	newImg := image.NewRGBA(bounds)

	// 3. Calculate the coordinates of the central protected rectangle
	protectedWidth := int(float64(width) * centralProtectionRatio)
	protectedHeight := int(float64(height) * centralProtectionRatio)

	// Top-left corner (x0, y0)
	x0 := (width - protectedWidth) / 2
	y0 := (height - protectedHeight) / 2
	// Bottom-right corner (x1, y1)
	x1 := x0 + protectedWidth
	y1 := y0 + protectedHeight

	fmt.Printf("Image Dimensions: %dx%d\n", width, height)
	fmt.Printf("Protected Area (%.f%%): from (%d, %d) to (%d, %d)\n", centralProtectionRatio*100, x0, y0, x1, y1)

	// 4. Iterate over each pixel
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			originalColor := img.At(x, y)

			// THE CORE LOGIC: Check if the pixel is INSIDE the protected rectangle
			if x >= x0 && x < x1 && y >= y0 && y < y1 {
				// We are inside the safe zone, so just copy the original pixel.
				newImg.Set(x, y, originalColor)
			} else {
				// We are OUTSIDE the safe zone, so apply the whitening logic.
				r, g, b, a := originalColor.RGBA()
				r8 := uint8(r >> 8)
				g8 := uint8(g >> 8)
				b8 := uint8(b >> 8)
				a8 := uint8(a >> 8)

				if r8 >= threshold && g8 >= threshold && b8 >= threshold {
					// Pixel is light enough, make it pure white.
					newImg.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: a8})
				} else {
					// Pixel is too dark to be background, keep its original color.
					newImg.Set(x, y, originalColor)
				}
			}
		}
	}

	// 5. Encode the new image into a byte buffer as a PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, newImg); err != nil {
		return nil, fmt.Errorf("failed to encode image to png: %w", err)
	}

	return buf.Bytes(), nil
}
