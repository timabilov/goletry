package controllers

import (
	"context"
	"embed"
	"fmt"
	"io"
	"lessnoteapi/models"
	"lessnoteapi/services"
	"log"
	"net/http"
	"os"
	"text/template"

	firebase "firebase.google.com/go/v4"
	"github.com/go-playground/validator"
	"github.com/hibiken/asynq"
	echojwt "github.com/labstack/echo-jwt"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"gorm.io/gorm"
)

type CustomValidator struct {
	validator *validator.Validate
}

func (cv *CustomValidator) Validate(i interface{}) error {
	if err := cv.validator.Struct(i); err != nil {
		// Optionally, you could return the error to give each route more control over the status code
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return nil
}

type Template struct {
	templates *template.Template
}

func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

//go:embed templates
var embededFiles embed.FS

func SetupServer(
	db *gorm.DB,
	googleService services.GoogleServiceProvider,
	awsService services.AWSServiceProvider,
	firebaseApp *firebase.App,
	asynqClient *asynq.Client,
	asynqInspector *asynq.Inspector,
) *echo.Echo {

	fmt.Println(firebaseApp, "Firebase app")
	err := awsService.InitPresignClient(context.Background())
	if err != nil {
		log.Fatal("Failed to initialize AWS provider: S3")
	}

	e := echo.New()
	// e.Server.Addr = "http://192.168.0.2:80"
	templates, err := template.ParseFS(embededFiles, "templates/*.html")
	if err != nil {
		fmt.Println(err)
	}

	t := &Template{
		templates: templates,
	}
	e.Renderer = t
	v := validator.New()
	v.RegisterValidation("platform", models.ValidatePlatform)
	v.RegisterValidation("language", models.ValidateLanguage)
	e.Validator = &CustomValidator{validator: v}
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("__db", db)
			c.Set("__asynqclient", asynqClient)
			c.Set("__asynqinspector", asynqInspector)
			return next(c)
		}
	})

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept},
	}))
	// e.GET("/privacy-policy", func(c echo.Context) error {
	// 	return c.Render(http.StatusOK, "privacypolicy.html", nil)

	// })
	// e.GET("/rules", func(c echo.Context) error {
	// 	return c.Render(http.StatusOK, "rules.html", nil)

	// })

	authGroup := e.Group("auth")

	controller := AuthController{Google: googleService, FirebaseApp: firebaseApp}
	controller.ProfileRoutes(authGroup)

	companyController := CompanyController{AWSService: awsService, FirebaseApp: firebaseApp}
	companyGroup := e.Group("/company/:companyId", echojwt.JWT([]byte(os.Getenv("JWT_SECRET"))), UserCompanyMiddleware)
	companyController.CompanyRoutes(companyGroup)

	shopGroup := e.Group("shop", echojwt.JWT([]byte(os.Getenv("JWT_SECRET"))))
	shopGroup.Use(UserMiddleware)

	// managed with public token or company id as param depending on settings!

	profileController := ProfileController{}
	profileGroup := shopGroup.Group("/profile")
	profileController.ProfileRoutes(profileGroup)

	userDataController := UserDataController{AWSService: awsService, FirebaseApp: firebaseApp}
	userDataGroup := shopGroup.Group("/userdata")
	userDataController.UserDataRoutes(userDataGroup)

	noteController := NoteRoutes{AWSService: awsService, FirebaseApp: firebaseApp}
	noteGroup := companyGroup.Group("/notes")
	noteController.NoteRoutes(noteGroup)

	webhooksController := WebhooksController{Google: googleService, FirebaseApp: firebaseApp}
	webhookGroup := e.Group("/webhooks")
	webhooksController.SetupRoutes(webhookGroup)

	return e
}
