package services

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type AWSServiceProvider interface {
	InitPresignClient(ctx context.Context) error
	PresignLink(ctx context.Context, bucketName string, fileName string) (string, error)
	UploadToPresignedURL(ctx context.Context, bucketName, url string, fileContent []byte) (string, int, error)
	GetPresignedR2FileReadURL(ctx context.Context, bucketName, fileKey string) (string, error)
}

type AWSService struct {
	S3PresignClient *s3.PresignClient
}

func (awsService *AWSService) InitPresignClient(ctx context.Context) error {
	var accountId = GetEnv("R2_ACCOUNT_ID", "")
	var accessKeyId = GetEnv("R2_ACCESS_KEY_ID", "")
	var accessKeySecret = GetEnv("R2_ACCESS_KEY_SECRET", "")
	r2Resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL: fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountId),
		}, nil
	})
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithEndpointResolverWithOptions(r2Resolver),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyId, accessKeySecret, "")),
	)
	if err != nil {
		log.Fatalf("unable to load SDK config, %v", err)
	}

	s3Client := s3.NewFromConfig(cfg)

	presignClient := s3.NewPresignClient(s3Client)

	awsService.S3PresignClient = presignClient
	return err
}

func (awsService *AWSService) PresignLink(ctx context.Context, bucketName string, fileName string) (string, error) {
	request, err := awsService.S3PresignClient.PresignPutObject(context.TODO(), &s3.PutObjectInput{Bucket: &bucketName, Key: &fileName})
	return request.URL, err
}

func (awsService *AWSService) GetPresignedR2FileReadURL(ctx context.Context, bucketName, fileKey string) (string, error) {

	// Generate presigned URL for GetObject
	presignedGetRequest, err := awsService.S3PresignClient.PresignGetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(fileKey),
	})
	if err != nil {
		return "", fmt.Errorf("failed to presign request: %v", err)
	}

	// Create HTTP client to fetch the file content
	return presignedGetRequest.URL, nil

}

func (awsService *AWSService) UploadToPresignedURL(ctx context.Context, bucketName, url string, fileContent []byte) (string, int, error) {
	// Detect MIME type from file content
	mimeType := http.DetectContentType(fileContent)
	fmt.Println("Detected MIME type:", mimeType)
	// Validate if the file is an audio type (optional)
	allowedMimeTypes := map[string]bool{
		"image/png":  true,
		"image/jpeg": true,
		"image/heic": true,
	}
	if !allowedMimeTypes[mimeType] {
		return "", 0, fmt.Errorf("unsupported file type: %s", mimeType)
	}

	// Create a buffer with the file content
	body := bytes.NewReader(fileContent)

	// Create HTTP PUT request
	req, err := http.NewRequest("PUT", url, body)
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return "", 0, err
	}

	// Set the detected Content-Type header
	req.Header.Set("Content-Type", mimeType)

	// Create HTTP client and execute request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error uploading file: %v\n", err)
		return "", resp.StatusCode, err
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response: %v\n", err)
		return "", resp.StatusCode, err
	}

	// Return response content and status code
	return string(respBody), resp.StatusCode, nil
}
