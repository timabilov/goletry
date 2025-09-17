package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"github.com/getsentry/sentry-go"
	"github.com/golang-jwt/jwt"
)

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

type MyData struct {
	Name    string `json:"name"`
	Age     string `json:"age"`
	Msg     string `json:"msg"`
	Channel string `json:"channel"`
}

func sendIOSNotificationDirect(messages []*messaging.Message) []error {
	teamID := "VUWPX5QW93"
	keyID := "4932TH49HQ"
	// ClientID is the "Services ID" value that you get when navigating to your "sign in with Apple"-enabled service ID
	bundleID := "com.skripe.leitnerai"
	privateKeyPEM, err := DecodeBase64EnvPrivateKey("APPLE_PUSH_KEY_BASE64")

	if err != nil {
		log.Println("Error getting Apple private key:", err)
		return []error{err}
	}

	// Your .p8 private key content (paste the entire content here)

	// Parse the private key
	block, _ := pem.Decode([]byte(privateKeyPEM))
	key, _ := x509.ParsePKCS8PrivateKey(block.Bytes)
	privateKey := key.(*ecdsa.PrivateKey)

	// Create JWT token
	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"iss": teamID,
		"iat": time.Now().Unix(),
	})
	token.Header["kid"] = keyID
	jwtToken, _ := token.SignedString(privateKey)

	// Create notification payload
	errors := []error{}
	for _, message := range messages {
		payload := map[string]interface{}{
			"aps": map[string]interface{}{
				"alert": map[string]string{
					"title":    message.APNS.Payload.Aps.Alert.Title,
					"subtitle": message.APNS.Payload.Aps.Alert.SubTitle,
					"body":     message.APNS.Payload.Aps.Alert.Body,
				},
				// "badge": 1,
			},
			"foo":     "bar", // Custom data
			"data":    map[string]interface{}{"note_id": "123"},
			"payload": map[string]interface{}{"type": "123"},
		}

		payloadBytes, _ := json.Marshal(payload)
		// Send to APNS
		url := fmt.Sprintf("https://api.push.apple.com/3/device/%s", message.Token)
		req, _ := http.NewRequest("POST", url, bytes.NewBuffer(payloadBytes))

		req.Header.Set("Authorization", "Bearer "+jwtToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("apns-topic", bundleID)

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			errors = append(errors, err)
			sentry.CaptureMessage(fmt.Sprintf("Error sending push for user data %s %s", message.Token, message.APNS.Payload.Aps.Alert.Title))
			fmt.Printf("No Response got error for %s: %v\n", message.Token, err)

			continue
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("%v Response for %s: %s\n", resp.StatusCode, message.Token, string(body))
		time.Sleep(500 * time.Millisecond) // To avoid hitting rate limits
	}
	return errors

}

func main() {
	// Example of incorrect JSON with the invalid character
	// invalidJSON := "{\"channel\":\"buupr\\niya\",\"name\":\"john\", \"msg\":\"doe\"}"
	// // charmap.
	// var data MyData
	// err := json.Unmarshal([]byte(invalidJSON), &data)
	// if err != nil {
	// 	log.Fatalf("Error unmarshalling JSON: %v", err)
	// }

	// fmt.Println("Unmarshalled data:", data)
	app, err := firebase.NewApp(context.Background(), nil)
	if err != nil {
		log.Fatalf("error initializing firebase app: %v\n", err)
		return
	}
	client, err := app.Messaging(context.Background())
	if err != nil {
		log.Fatalf("error getting Messaging client: %v\n", err)
		return
	}
	var messages []*messaging.Message
	messages = append(messages, &messaging.Message{

		Notification: &messaging.Notification{
			Title: "Title",
			Body:  "message",
		},
		APNS: &messaging.APNSConfig{
			FCMOptions: &messaging.APNSFCMOptions{
				AnalyticsLabel: "lessnote",
			},
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					Alert: &messaging.ApsAlert{
						Title:    "Title",
						SubTitle: "Subtitle",
						Body:     "message",
					},
					// Sound: "default",
					CustomData: map[string]interface{}{
						"note_id":     fmt.Sprintf("%d", 834),
						"type":        "quiz_alert",
						"question_id": fmt.Sprintf("%d", 3829),
					},
				},
			},
		},
		Android: &messaging.AndroidConfig{
			// TTL: &oneHour,
			Notification: &messaging.AndroidNotification{
				Priority:  messaging.AndroidNotificationPriority(messaging.PriorityMax),
				ChannelID: "lessnote-high-priority",
				// VibrateTimingMillis: ,
				// Icon:     "stock_ticker_update",
				// Color:    "#f45342",
			},
			Data: map[string]string{"note_id": fmt.Sprintf("%d", 834), "type": "quiz_alert", "question_id": fmt.Sprintf("%d", 3829)},
		},
		Token: "e051243033b2fff34c79d4862290e56e689096414f8a09f72111016d29d7846e",
	})

	br, err := client.SendEach(context.Background(), messages)
	if err != nil {
		log.Fatalln(err)
	} else {
		fmt.Println("Push Fails: ", br.FailureCount)

		for _, fail := range br.Responses {
			if fail != nil {
				fmt.Println(fail.Error, fail.MessageID, fail.Success)
			}
		}
		fmt.Println("Notifications sent")
	}

	sendIOSNotificationDirect(messages)

}
