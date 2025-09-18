package services

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"letryapi/models"
	"log"
	"net/http"
	"os"
	"regexp"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"github.com/getsentry/sentry-go"
	"github.com/golang-jwt/jwt"
	"google.golang.org/api/idtoken"
	"gorm.io/gorm"
)

type GoogleServiceProvider interface {
	ValidateIdToken(ctx context.Context, idToken string, audience string) (*idtoken.Payload, error)
	GetUserSubscriptionStatus(ctx context.Context, appUserId string) ([]byte, error)
}

type GoogleService struct {
}

func (gs GoogleService) ValidateIdToken(ctx context.Context, idToken string, audience string) (*idtoken.Payload, error) {

	return idtoken.Validate(context.Background(), idToken, audience)
}

func (gs GoogleService) GetUserSubscriptionStatus(ctx context.Context, appUserId string) ([]byte, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.revenuecat.com/v1/subscribers/%s", appUserId), nil)

	API_KEY := os.Getenv("RC_API_KEY")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", API_KEY))

	res, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		return nil, err

	}
	defer res.Body.Close()

	b, err := io.ReadAll(res.Body)

	return b, err
}

func stringMapToInterfaceMap(stringMap map[string]string) map[string]interface{} {
	interfaceMap := make(map[string]interface{})
	for key, value := range stringMap {
		interfaceMap[key] = value
	}
	return interfaceMap
}

func SendNotification(fbApp *firebase.App, db *gorm.DB, userId uint, title string, message string, customData map[string]string) {
	client, err := fbApp.Messaging(context.Background())
	if err != nil {
		fmt.Println("Error initing FB client", err)
		fmt.Println("Abort push: ", title)
		return
	}
	var tokens []models.UserPushToken
	result := db.Model(models.UserPushToken{}).Where(
		"user_account_id = ? and active = true", userId,
	).Find(&tokens)
	if result.Error != nil {
		fmt.Println("Error pushing expire notification", result.Error)
	} else {

	}

	var androidMessages []*messaging.Message
	var iOSMessages []*messaging.Message
	var iosCustomData map[string]interface{}
	if customData != nil {
		iosCustomData = stringMapToInterfaceMap(customData)
	}
	for _, token := range tokens {
		fmt.Println("Push notification to token: ", token.Token, token.Platform, token.CreatedAt, " ID:", token.ID, "User ID:", token.UserAccountID)
		message := &messaging.Message{

			Notification: &messaging.Notification{
				Title: title,
				Body:  message,
			},
			APNS: &messaging.APNSConfig{
				FCMOptions: &messaging.APNSFCMOptions{
					AnalyticsLabel: "lessnote",
				},
				Payload: &messaging.APNSPayload{
					Aps: &messaging.Aps{
						ContentAvailable: true,
						Alert: &messaging.ApsAlert{
							Title: title,
							Body:  message,
						},
						Sound: "default",
					},
					CustomData: iosCustomData,
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
				Data: customData,
			},
			Token: token.Token,
		}
		if token.Platform == "ios" {
			iOSMessages = append(iOSMessages, message)
		} else {

			androidMessages = append(androidMessages, message)
		}
	}
	if len(androidMessages) > 0 {
		br, err := client.SendEach(context.Background(), androidMessages)
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
	}
	if len(iOSMessages) > 0 {
		errors := sendIOSNotificationDirect(iOSMessages)
		if len(errors) > 0 {
			fmt.Println("iOS Push Fails: ", len(errors))
			for _, err := range errors {
				fmt.Println(err)
			}
		} else {
			fmt.Println("iOS Notifications sent")
		}
	}
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

var YoutubeURLRegex = regexp.MustCompile(
	`(?i)^(?:https?:\/\/)?(?:www\.|m\.)?(?:(?:youtube\.com\/(?:watch\?v=|v\/|embed\/|live\/))|(?:youtu\.be|y2u\.be)\/)([a-zA-Z0-9_-]{11})(?:[?&].*)?$`,
)
