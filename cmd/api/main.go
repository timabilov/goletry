package main

import (
	"context"
	"lessnoteapi/controllers"
	"lessnoteapi/dbhelper"
	"lessnoteapi/services"
	"lessnoteapi/telegram"
	"log"
	"os"
	"time"

	// "github.com/getsentry/sentry-go"
	firebase "firebase.google.com/go/v4"
	"github.com/getsentry/sentry-go"
	sentryecho "github.com/getsentry/sentry-go/echo"
	"github.com/hibiken/asynq"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	rcToken := os.Getenv("RC_WEBHOOK_TOKEN")
	if rcToken == "" {
		log.Fatal("RC_WEBHOOK_TOKEN environment variable is not set!")
	}
	err := sentry.Init(sentry.ClientOptions{
		// Either set your DSN here or set the SENTRY_DSN environment variable.
		Dsn: "https://eebea6b875147cde5d2071479ec8c062@o4506513441685504.ingest.us.sentry.io/4509185560477696",
		// Either set environment and release here or set the SENTRY_ENVIRONMENT
		// and SENTRY_RELEASE environment variables.
		Environment: services.GetEnv("ENV", "local"),
		Release:     "lessnotego@1.0.0",
		// Enable printing of SDK debug messages.
		// Useful when getting started or trying to figure something out.
		Debug: false,
		// Set TracesSampleRate to 1.0 to capture 100%
		// of transactions for performance monitoring.
		// We recommend adjusting this value in production,
		TracesSampleRate: 1.0,
	})
	if err != nil {
		log.Fatalf("sentry.Init: %s", err)
	}
	defer sentry.Recover()
	defer sentry.Flush(2 * time.Second)
	// filename := "mykey.txt"
	// request, err := presignClint.PresignPutObject(context.TODO(), &s3.PutObjectInput{Bucket: &bucketName, Key: &filename})

	// if err != nil {
	// 	log.Fatalf("unable to presign generate!!!, %v", err)
	// }

	// http.Header()
	db := dbhelper.SetupDB()

	app, err := firebase.NewApp(context.Background(), nil)
	if err != nil {
		log.Fatalf("error initializing firebase app: %v\n", err)
		return
	}
	asynqClient := asynq.NewClient(asynq.RedisClientOpt{Addr: os.Getenv("ASYNC_BROKER_ADDRESS")})
	asynqInspector := asynq.NewInspector(asynq.RedisClientOpt{Addr: os.Getenv("ASYNC_BROKER_ADDRESS")})
	e := controllers.SetupServer(
		db, services.GoogleService{}, &services.AWSService{}, app,
		asynqClient, asynqInspector,
	)
	e.Debug = true
	if os.Getenv("TELEGRAM_BOT") == "true" {

		telegram.RunWordBot(e, db)

	} else {
		e.Use(middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(3)))
		e.Use(middleware.Logger())
		e.Use(middleware.Recover())

		// Once it's done, you can attach the handler as one of your middleware
		e.Use(sentryecho.New(sentryecho.Options{Repanic: true}))
		e.Logger.Fatal(e.Start(":8083"))
	}
}
