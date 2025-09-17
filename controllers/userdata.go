package controllers

import (
	"lessnoteapi/services"

	firebase "firebase.google.com/go/v4"
	"github.com/labstack/echo/v4"
)

type UserDataController struct {
	AWSService  services.AWSServiceProvider
	FirebaseApp *firebase.App
}

func (controller *UserDataController) UserDataRoutes(g *echo.Group) {

	// g.POST("/request-batch-image-upload", func(c echo.Context) error {
	// 	db := c.Get("__db").(*gorm.DB)
	// 	user := c.Get("currentUser").(models.UserAccount)
	// 	// company := user.Memberships[0].Company
	// 	var request = new(models.ProductImagesUploadRequestIn)
	// 	var productUrls []models.ProductUrlRequstOut
	// 	var productIds []string
	// 	productToImageName := map[string]string{}
	// 	if err := c.Bind(request); err != nil {
	// 		fmt.Println("Error bind data", err)
	// 		return err
	// 	}
	// 	for _, productRequest := range request.Products {
	// 		productIds = append(productIds, productRequest.ProductId)
	// 		productToImageName[productRequest.ProductId] = productRequest.ImageName
	// 	}
	// 	log.Println("Request product urls with size: ", len(productIds), " for user: ", user.ID)

	// 	var myCompanyIds []uint
	// 	for _, membership := range user.Memberships {
	// 		if membership.Active {

	// 			myCompanyIds = append(myCompanyIds, membership.CompanyID)
	// 		} else {
	// 			log.Println("non active membership for m id ", membership.ID)
	// 		}
	// 	}
	// 	var results []models.Product
	// 	result := db.Where("ID in (?) and company_id in (?)", productIds, myCompanyIds).Order("id asc").Find(&results)
	// 	if result.Error != nil {
	// 		log.Println("Error fetching products for image upload ", result.Error)
	// 		return c.JSON(http.StatusInternalServerError, echo.Map{
	// 			"message": "Error while uploading images.",
	// 		})
	// 	}

	// 	if len(results) == 0 {
	// 		log.Println("No active memberships found for user, ignore image updates..")
	// 	}
	// 	for _, product := range results {
	// 		// Operations on each record in the batch
	// 		log.Println("Image url provided, generate presign link for ", product.ID)
	// 		var bucketName = services.GetEnv("R2_BUCKET_NAME", "")

	// 		localImageName := productToImageName[product.ID]
	// 		if localImageName == "" {
	// 			log.Println("Set empty image for product ", product.ID, "..")
	// 			product.ImageUrl = StrPointer("")

	// 			productUrls = append(productUrls, models.ProductUrlRequstOut{
	// 				ProductId:         product.ID,
	// 				UploadUrl:         "",
	// 				ProductFileName:   localImageName,
	// 				ProductRemoteName: "",
	// 			})
	// 			err := db.Select("image_url").Updates(&product).Error
	// 			if err != nil {
	// 				log.Printf("Error saving empty/null product link!, %v, %s", product.ID, err)
	// 				return c.JSON(http.StatusInternalServerError, echo.Map{
	// 					"message": "Error while uploading images.",
	// 				})
	// 			}
	// 			continue
	// 		}
	// 		nameChunks := strings.Split(localImageName, "/")
	// 		if len(nameChunks) == 0 {
	// 			nameChunks = []string{
	// 				"jpg",
	// 			}
	// 			log.Println("file name is empty!: ", localImageName, "for id ", product.ID)
	// 		}

	// 		fileName := fmt.Sprintf("%s-%v-%s", product.ID, time.Now().UnixMilli(), nameChunks[len(nameChunks)-1])
	// 		safeFileName := fmt.Sprintf("products/%s", fileName)
	// 		product.ImageUrl = &safeFileName
	// 		err := db.Select("image_url").Updates(&product).Error
	// 		if err != nil {
	// 			log.Printf("Error saving product link!, %v, %s", product.ID, err)
	// 			return c.JSON(http.StatusInternalServerError, echo.Map{
	// 				"message": "Error while uploading images.",
	// 			})
	// 		}
	// 		url, err := controller.AWSService.PresignLink(context.Background(), bucketName, safeFileName)
	// 		if err != nil {
	// 			log.Printf("Unable to presign generate for %s!, %s", product.Name, err)
	// 			return c.JSON(http.StatusInternalServerError, echo.Map{
	// 				"message": "Error while uploading images.",
	// 			})
	// 		}

	// 		productUrls = append(productUrls, models.ProductUrlRequstOut{
	// 			ProductId:         product.ID,
	// 			UploadUrl:         url,
	// 			ProductFileName:   localImageName,
	// 			ProductRemoteName: safeFileName,
	// 		})
	// 	}
	// 	return c.JSON(http.StatusOK, models.ProductImagesUploadRequestOut{
	// 		Products: productUrls,
	// 	})
	// })

	// g.POST("/request-batch-image-upload-v2", func(c echo.Context) error {
	// 	db := c.Get("__db").(*gorm.DB)
	// 	user := c.Get("currentUser").(models.UserAccount)
	// 	// company := user.Memberships[0].Company
	// 	var request = new(models.ProductImagesUploadRequestIn)
	// 	var productUrls []models.ProductUrlRequstOut
	// 	var productIds []string
	// 	productToImageName := map[string]string{}
	// 	if err := c.Bind(request); err != nil {
	// 		fmt.Println("Error bind data", err)
	// 		return err
	// 	}
	// 	for _, productRequest := range request.Products {
	// 		productIds = append(productIds, productRequest.ProductId)
	// 		productToImageName[productRequest.ProductId] = productRequest.ImageName
	// 	}
	// 	log.Println("Request product urls with size: ", len(productIds), " for user: ", user.ID)

	// 	var myCompanyIds []uint
	// 	for _, membership := range user.Memberships {
	// 		if membership.Active && string(membership.Company.Subscription) != "free" {

	// 			myCompanyIds = append(myCompanyIds, membership.CompanyID)
	// 		} else {
	// 			log.Println("non active membership for m id ", membership.ID, "name ", user.Name, "sub status", string(membership.Company.Subscription))
	// 		}
	// 	}
	// 	var results []models.Product
	// 	result := db.Where("ID in (?) and company_id in (?)", productIds, myCompanyIds).Order("id asc").Find(&results)
	// 	if result.Error != nil {
	// 		log.Println("Error fetching products for image upload ", result.Error)
	// 		return c.JSON(http.StatusInternalServerError, echo.Map{
	// 			"message": "Error while uploading images.",
	// 		})
	// 	}

	// 	if len(results) == 0 {
	// 		log.Println("No active memberships found for user, ignore image updates..")
	// 	}
	// 	for _, product := range results {
	// 		// Operations on each record in the batch
	// 		var bucketName = services.GetEnv("R2_BUCKET_NAME", "")

	// 		localImageName := productToImageName[product.ID]
	// 		log.Println("Image url provided, generate presign link for ", product.ID, " local image name provided: ", localImageName)
	// 		if localImageName == "" {
	// 			log.Println("Set empty image for product ", product.ID, "..")
	// 			product.ImageUrl = StrPointer("")

	// 			productUrls = append(productUrls, models.ProductUrlRequstOut{
	// 				ProductId:         product.ID,
	// 				UploadUrl:         "",
	// 				ProductFileName:   localImageName,
	// 				ProductRemoteName: "",
	// 			})
	// 			err := db.Select("image_url").Updates(&product).Error
	// 			if err != nil {
	// 				log.Printf("Error saving empty/null product link!, %v, %s", product.ID, err)
	// 				return c.JSON(http.StatusInternalServerError, echo.Map{
	// 					"message": "Error while uploading images.",
	// 				})
	// 			}
	// 			continue
	// 		}
	// 		// prefixPath, fileName, found := strings.Split(localImageName, "/")
	// 		nameChunks := strings.Split(localImageName, "/")
	// 		fileName := nameChunks[0] // worst bad case
	// 		if len(nameChunks) > 1 {
	// 			// ideally we need that!
	// 			fileName = nameChunks[len(nameChunks)-1]
	// 		}
	// 		fileName = strings.ReplaceAll(fileName, "-", "")

	// 		dbfileName := fmt.Sprintf("%s-%v-%s", product.ID, time.Now().UnixMilli(), fileName)
	// 		safeFileName := fmt.Sprintf("products/%s", dbfileName)
	// 		product.ImageUrl = &safeFileName
	// 		err := db.Select("image_url").Updates(&product).Error
	// 		if err != nil {
	// 			log.Printf("Error saving product link!, %v, %s", product.ID, err)
	// 			return c.JSON(http.StatusInternalServerError, echo.Map{
	// 				"message": "Error while uploading images.",
	// 			})
	// 		}
	// 		url, err := controller.AWSService.PresignLink(context.Background(), bucketName, safeFileName)
	// 		if err != nil {
	// 			log.Printf("Unable to presign generate for %s!, %s", product.Name, err)
	// 			return c.JSON(http.StatusInternalServerError, echo.Map{
	// 				"message": "Error while uploading images.",
	// 			})
	// 		}

	// 		productUrls = append(productUrls, models.ProductUrlRequstOut{
	// 			ProductId:         product.ID,
	// 			UploadUrl:         url,
	// 			ProductFileName:   localImageName,
	// 			ProductRemoteName: safeFileName,
	// 		})
	// 	}
	// 	return c.JSON(http.StatusOK, models.ProductImagesUploadRequestOut{
	// 		Products: productUrls,
	// 	})
	// })

}
