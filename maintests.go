package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"io/ioutil"
	"log"
	"os"

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

// --- Example Usage ---
func main() {
	// Let's use the problematic dummy image with a soft shadow again.
	fmt.Println("Read 'input.png' with a gradient shadow.")

	imageBytes, err := ioutil.ReadFile("input.png")
	if err != nil {
		log.Fatalf("Failed to read input image: %v", err)
	}

	// --- Tuning Parameters ---
	// We only need one threshold to create the initial hard mask.
	// Pick a value that reliably separates the subject from the true background.
	var threshold uint8 = 240

	// This is the most important parameter. It controls the "feather" radius.
	// Higher numbers = softer edges. Start with something like 3.0 or 4.0.
	var blurSigma float64 = 4.0

	fmt.Printf("Applying smooth whitening with threshold %d and blur sigma %.1f\n", threshold, blurSigma)

	whitenedBytes, err := WhitenBackgroundSmooth(imageBytes, threshold, blurSigma)
	if err != nil {
		log.Fatalf("Failed to whiten background: %v", err)
	}

	err = ioutil.WriteFile("output_smooth.png", whitenedBytes, 0644)
	if err != nil {
		log.Fatalf("Failed to write output image: %v", err)
	}

	fmt.Println("Successfully processed image and saved to 'output_smooth.png'.")

	// Clean up
	// os.Remove("input_smooth.png")
	// os.Remove("output_smooth.png")
}

// Helper to create a sample image with a gradient/shadow (re-used from previous example)
func createShadowDummyImage(filename string) error {
	width, height := 400, 200
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	bgColor := uint8(248)
	subjectColor := color.RGBA{R: 150, G: 150, B: 150, A: 255}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if y > 150 && y < 180 {
				progress := float64(y-150) / 30.0
				shadowVal := uint8(230*(1-progress) + float64(bgColor)*progress)
				img.Set(x, y, color.RGBA{R: shadowVal, G: shadowVal, B: shadowVal, A: 255})
			} else {
				img.Set(x, y, color.RGBA{R: bgColor, G: bgColor, B: bgColor, A: 255})
			}
		}
	}
	for y := 50; y < 150; y++ {
		for x := 150; x < 250; x++ {
			img.Set(x, y, subjectColor)
		}
	}
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
