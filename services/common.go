package services

import (
	"archive/zip"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/getsentry/sentry-go"
)

var allowedAudioExtensions = []string{".m4a", ".mp3", ".ogg", ".wav"}
var allowedImageExtensions = []string{".jpg", ".jpeg", ".png", ".heic", ".heif", ".webp"}
var allowedDocumentExtensions = []string{".pdf", ".txt"}

var allowedExtensions = combineExtensions(
	allowedAudioExtensions,
	allowedImageExtensions,
	allowedDocumentExtensions,
)

func combineExtensions(slices ...[]string) []string {
	var result []string
	for _, slice := range slices {
		result = append(result, slice...)
	}
	return result
}

// Helper function to determine note type based on file extensions
func DetermineNoteType(paths []string) string {
	if len(paths) == 0 {
		return "multi" // Default to multi if no paths
	}

	// Track the type of the first file
	var firstType string
	for _, path := range paths {
		ext := strings.ToLower(filepath.Ext(path))
		var currentType string

		// Determine the type of the current file
		switch {
		case slices.Contains(allowedAudioExtensions, ext):
			currentType = "audio"
		case slices.Contains(allowedImageExtensions, ext):
			currentType = "image"
		default:
			return "multi" // Unknown extension, default to multi
		}

		// Set firstType for the first file
		if firstType == "" {
			firstType = currentType
			continue
		}

		// If any file type differs, return "multi"
		if currentType != firstType {
			return "multi"
		}
	}

	return firstType
}

func StrPointer(str string) *string {
	if str == "" {
		return nil
	}
	return &str
}

func ReadFileFromUrl(url string) ([]byte, error) {
	httpClient := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %v", err)
	}

	// Set headers to prevent caching
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")

	// Perform the HTTP request
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get response: %v", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("failed to fetch file, status code: %d", resp.StatusCode)
	}

	// Read the file content
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	return content, nil
}

func GetEnv(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}
	return value
}

func CreateTempFile(data []byte, filename string) (string, error) {
	// Create a temporary file with the given filename as a pattern
	ext := filepath.Ext(filename)
	tempFile, err := os.CreateTemp("", "temp-*"+ext)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %v", err)
	}
	defer tempFile.Close()
	fmt.Println("Byte length:", len(data))
	// Write bytes to the temporary file
	if _, err := tempFile.Write(data); err != nil {
		return "", fmt.Errorf("failed to write to temp file: %v", err)
	}

	// Return the path to the temporary file
	return tempFile.Name(), nil
}

// ExtractZipImages extracts image files from a zip and creates temporary files for them.
// Only processes files in the root directory of the zip.
// Returns a slice of temporary file paths and any error encountered.
func ExtractZipImages(zipBytes []byte, zipFileName string, noteID uint) ([]string, error) {
	// Create temp file for zip
	zipPath, err := CreateTempFile(zipBytes, zipFileName)
	if err != nil {
		return nil, fmt.Errorf("error creating temp zip file: %w", err)
	}
	defer os.Remove(zipPath)

	// Open zip file
	zipReader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("error opening zip file: %w", err)
	}
	defer zipReader.Close()

	var tempFiles []string
	if len(zipReader.File) == 0 {
		return nil, fmt.Errorf("zip file is empty")
	}
	if len(zipReader.File) > 10 {
		return nil, fmt.Errorf("Zip file contains more than 10 files!")
	}
	for i, file := range zipReader.File {
		// Skip directories and non-root files
		if file.FileInfo().IsDir() || strings.Contains(file.Name, "/") || strings.Contains(file.Name, "\\") {
			continue
		}

		// Check if file is allowed asset
		ext := strings.ToLower(filepath.Ext(file.Name))
		imageAllowedExtensions := []string{".jpg", ".jpeg", ".png", ".gif", ".webp"}
		isAllowed := false

		// if extension is not in the allowed list, raise error
		for _, allowedExt := range imageAllowedExtensions {
			if ext == allowedExt {
				isAllowed = true
				break
			}
		}
		if !isAllowed {
			sentry.CaptureException(fmt.Errorf("[Note: %v] file %s is not a valid image file", noteID, file.Name))
			continue
		}
		// Check if file size is less than 50MB
		if file.UncompressedSize64 > 50*1024*1024 {
			// sentry.CaptureException(fmt.Errorf("[Note: %v] file %s is larger than 5MB", noteID, file.Name))
			return nil, fmt.Errorf("[Note: %v] file %s is larger than 60MB", noteID, file.Name)
		}
		// Open file in zip
		f, err := file.Open()
		if err != nil {
			sentry.CaptureException(fmt.Errorf("[Note: %v] error reading file %s from zip: %w", noteID, file.Name, err))
			continue
		}

		// Read image bytes
		imgBytes, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			sentry.CaptureException(fmt.Errorf("[Note: %v] error reading image bytes %s: %w", noteID, file.Name, err))
			continue
		}

		// Create temp file for image
		imgFileName := fmt.Sprintf("%s_%d%s", strings.TrimSuffix(zipFileName, filepath.Ext(zipFileName)), i, ext)
		imgPath, err := CreateTempFile(imgBytes, imgFileName)
		if err != nil {
			sentry.CaptureException(fmt.Errorf("[Note: %v] error creating temp file for image %s: %w", noteID, file.Name, err))
			continue
		}
		tempFiles = append(tempFiles, imgPath)
	}

	if len(tempFiles) == 0 {
		return nil, fmt.Errorf("no valid image files found in zip")
	}
	return tempFiles, nil
}

func ExtractZipMaterialFiles(zipBytes []byte, zipFileName string, noteID uint) ([]string, string, error) {
	// Create temp file for zip
	zipPath, err := CreateTempFile(zipBytes, zipFileName)
	if err != nil {
		return nil, "", fmt.Errorf("error creating temp zip file: %w", err)
	}
	defer os.Remove(zipPath)

	// Open zip file
	zipReader, err := zip.OpenReader(zipPath)
	if err != nil {
		fmt.Printf("[Note: %v] error opening zip file: %v", noteID, err)
		sentry.CaptureException(fmt.Errorf("[Note: %v] error opening zip file: %w", noteID, err))
		return nil, "", fmt.Errorf("error opening zip file: %w", err)
	}
	defer zipReader.Close()

	var tempFiles []string
	var textContent string
	if len(zipReader.File) == 0 {
		fmt.Printf("[Note: %v] zip empty: %v", noteID, err)
		sentry.CaptureException(fmt.Errorf("[Note: %v] zip empty: %w", noteID, err))
		return nil, "", fmt.Errorf("zip file is empty")
	}
	if len(zipReader.File) > 10 {
		fmt.Printf("[Note: %v] zip more than 10 files: %v", noteID, err)
		sentry.CaptureException(fmt.Errorf("[Note: %v] zip more than 10 files: %w", noteID, err))
		return nil, "", fmt.Errorf("zip file contains more than 10 files")
	}
	for i, file := range zipReader.File {
		// Skip directories and non-root files
		if file.FileInfo().IsDir() || strings.Contains(file.Name, "/") || strings.Contains(file.Name, "\\") {
			continue
		}

		// Check if file is allowed asset
		fmt.Printf("[Note: %v] Zip contains file: %s\n", noteID, file.Name)
		ext := strings.ToLower(filepath.Ext(file.Name))
		isAllowed := false

		// if extension is not in the allowed list, raise error
		for _, allowedExt := range allowedExtensions {
			if ext == allowedExt {
				isAllowed = true
				break
			}
		}
		if !isAllowed {
			fmt.Printf("[Note: %v] file %s is not a valid allowed document file", noteID, file.Name)
			sentry.CaptureException(fmt.Errorf("[Note: %v] file %s is not a valid allowed document file", noteID, file.Name))
			continue
		}
		// Check if file size is less than 50MB
		if file.UncompressedSize64 > 50*1024*1024 {
			// sentry.CaptureException(fmt.Errorf("[Note: %v] file %s is larger than 5MB", noteID, file.Name))
			fmt.Printf("[Note: %v] zip > 50MB: %v", noteID, err)
			sentry.CaptureException(fmt.Errorf("[Note: %v]  zip > 50MB: %w", noteID, err))
			return nil, "", fmt.Errorf("[Note: %v] file %s is larger than 60MB", noteID, file.Name)
		}
		// Open file in zip
		f, err := file.Open()
		if err != nil {
			fmt.Printf("[Note: %v] Error reading file: %s %v", noteID, file.Name, err)
			sentry.CaptureException(fmt.Errorf("[Note: %v] error reading file %s from zip: %w", noteID, file.Name, err))
			continue
		}

		// Read document bytes
		documentBytes, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			fmt.Printf("[Note: %v] Error reading document bytes: %s %v", noteID, file.Name, err)
			sentry.CaptureException(fmt.Errorf("[Note: %v] error reading document bytes %s: %w", noteID, file.Name, err))
			continue
		}

		if ext == ".txt" {
			// Convert bytes to string as transcript llm text
			textContent = string(documentBytes)
			// Skip creating a temp file for text files
			continue
		}
		// Create temp file for document
		materialFileName := fmt.Sprintf("%s_%d%s", strings.TrimSuffix(zipFileName, filepath.Ext(zipFileName)), i, ext)
		imgPath, err := CreateTempFile(documentBytes, materialFileName)
		if err != nil {
			fmt.Printf("[Note: %v] Error creating temp file for document: %s %v", noteID, file.Name, err)
			sentry.CaptureException(fmt.Errorf("[Note: %v] error creating temp file for document %s: %w", noteID, file.Name, err))
			continue
		}
		tempFiles = append(tempFiles, imgPath)
	}

	if len(tempFiles) == 0 && textContent == "" {
		fmt.Printf("[Note: %v] no valid document files in zip provided nor we have text ", noteID)
		sentry.CaptureException(fmt.Errorf("[Note: %v] no valid document files in zip provided nor we have text ", noteID))
		return nil, "", fmt.Errorf("no valid document files found in zip nor we have text")
	}
	return tempFiles, textContent, nil
}

func DecodeBase64EnvPrivateKey(envKey string) (string, error) {
	// Get base64 encoded private key from environment
	base64Key := os.Getenv(envKey)
	if base64Key == "" {
		return "", fmt.Errorf("%s environment variable is not set", envKey)
	}

	// Decode from base64
	decodedBytes, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64 private key: %v", err)
	}

	// Convert to string
	secret := string(decodedBytes)

	return secret, nil
}
