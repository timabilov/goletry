package services

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"

	"github.com/disintegration/imaging"
)

// WhitenBackgroundSmooth composites the original image over a white background using a blurred mask.
// This creates a very smooth, professional-looking transition without hard edges or artifacts.
// - imageBytes: The input image as a byte slice.
// - threshold: The brightness value (0-255) used to identify the background for the initial mask.
// - blurSigma: The strength of the Gaussian blur applied to the mask. Higher values mean a softer, wider transition. A good starting value is 3.0 to 5.0.
func WhitenBackgroundSmooth(imageBytes []byte, threshold uint8, blurSigma float64) ([]byte, error) {
	// 1. Decode the original image. We'll need it for the final composite.
	originalImg, _, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}
	bounds := originalImg.Bounds()
	width, height := bounds.Max.X, bounds.Max.Y

	// 2. GENERATE A HARD MASK
	// The mask is a grayscale image. White = background, Black = foreground.
	mask := image.NewGray(bounds)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := originalImg.At(x, y)
			r, g, b, _ := c.RGBA()
			r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(b>>8)

			// Use luminance to determine if a pixel is background.
			// This is more accurate than a simple RGB check.
			luminance := 0.299*float64(r8) + 0.587*float64(g8) + 0.114*float64(b8)

			if luminance >= float64(threshold) {
				mask.SetGray(x, y, color.Gray{Y: 255}) // White: part of the background to be replaced
			} else {
				mask.SetGray(x, y, color.Gray{Y: 0}) // Black: part of the foreground to keep
			}
		}
	}

	// 3. SMOOTH THE MASK
	// This is the key step. We blur the hard mask to create a soft, feathered transition.
	// The 'blurSigma' parameter controls how soft the edge becomes.
	blurredMask := imaging.Blur(mask, blurSigma)

	// 4. COMPOSITE THE FINAL IMAGE
	// Create a new pure white canvas.
	whiteBg := image.NewNRGBA(bounds)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			whiteBg.Set(x, y, color.White)
		}
	}

	// Create the destination image.
	finalImg := image.NewNRGBA(bounds)

	// Iterate and blend pixel by pixel using the blurred mask.
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Get the original pixel's color
			r, g, b, a := originalImg.At(x, y).RGBA()

			// Get the mask's alpha value for this pixel. The blurred mask is grayscale,
			// so the R component is the gray value.
			maskAlpha, _, _, _ := blurredMask.At(x, y).RGBA()

			// Normalize the mask value from 0-65535 to 0.0-1.0
			// This alpha determines how much of the original image shows through.
			// Note: We INVERT the mask's logic.
			// White on mask (65535) means background -> 0% original image.
			// Black on mask (0) means foreground -> 100% original image.
			alpha := 1.0 - float64(maskAlpha)/65535.0

			// Linear interpolation: Final = Original * alpha + White * (1 - alpha)
			finalR := float64(r)*alpha + 65535.0*(1.0-alpha)
			finalG := float64(g)*alpha + 65535.0*(1.0-alpha)
			finalB := float64(b)*alpha + 65535.0*(1.0-alpha)

			finalImg.SetNRGBA(x, y, color.NRGBA{
				R: uint8(finalR / 257),
				G: uint8(finalG / 257),
				B: uint8(finalB / 257),
				A: uint8(a / 257),
			})
		}
	}

	// 5. Encode the final, beautifully blended image to PNG bytes
	var buf bytes.Buffer
	if err := png.Encode(&buf, finalImg); err != nil {
		return nil, fmt.Errorf("failed to encode final image: %w", err)
	}
	return buf.Bytes(), nil
}
