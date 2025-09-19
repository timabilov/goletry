package main

import (
	"context"
	"letryapi/dbhelper"
	"letryapi/services"
	"letryapi/tasks"
	"log"
	"os"

	firebase "firebase.google.com/go/v4"
	"github.com/hibiken/asynq"
)

func NewQuizAlertTask() *asynq.Task {
	return asynq.NewTask("generatestudy:alert", []byte{})
}

func runScheduler() {

	scheduler := asynq.NewScheduler(asynq.RedisClientOpt{Addr: os.Getenv("ASYNC_BROKER_ADDRESS")}, &asynq.SchedulerOpts{

		LogLevel: asynq.InfoLevel,
	})

	// Schedule daily tasks with different cron expressions
	tasks := []struct {
		cron string
		task *asynq.Task
		desc string
	}{
		{
			cron: "0 10,14,18 * * *", // 10:00 AM, 2:00 PM, 6:00 PM daily
			task: NewQuizAlertTask(),
			desc: "Quiz alert notifications",
		},
	}

	// Register all tasks
	for _, t := range tasks {
		entryID, err := scheduler.Register(t.cron, t.task)
		if err != nil {
			log.Fatalf("Failed to register task '%s': %v", t.desc, err)
		}
		log.Printf("Registered task '%s' with ID: %s, cron: %s", t.desc, entryID, t.cron)
	}

	log.Println("Starting scheduler...")
	if err := scheduler.Run(); err != nil {
		log.Fatalf("Scheduler failed: %v", err)
	}
}

func main() {
	// Initialize asynq server
	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: os.Getenv("ASYNC_BROKER_ADDRESS")},
		asynq.Config{Concurrency: 10, Queues: map[string]int{
			"generate": 7,
		}},
	)
	awsService := &services.AWSService{}
	llmProcessor := &services.GoogleLLMNoteProcessor{}
	err := awsService.InitPresignClient(context.Background())
	if err != nil {
		log.Fatal("[Queue] Failed to initialize AWS provider: S3")
	}
	app, err := firebase.NewApp(context.Background(), nil)
	if err != nil {
		log.Fatalf("error initializing firebase app: %v\n", err)
		return
	}
	// Set up task handler
	mux := asynq.NewServeMux()
	db := dbhelper.SetupDB()
	mux.HandleFunc("generate:tryon", func(ctx context.Context, t *asynq.Task) error {
		return tasks.HandleTryOnGenerationTask(ctx, t, db, llmProcessor, awsService)
	})
	mux.HandleFunc("generate:process_clothing", func(ctx context.Context, t *asynq.Task) error {
		return tasks.ProcessClothingTask(ctx, t, db, llmProcessor, awsService, app)
	})
	mux.HandleFunc("generate:avatar", func(ctx context.Context, t *asynq.Task) error {
		return tasks.ProcessClothingTask(ctx, t, db, llmProcessor, awsService, app)
	})

	go runScheduler()
	// Run the worker
	if err := srv.Run(mux); err != nil {
		log.Fatal(err)
	}
}
