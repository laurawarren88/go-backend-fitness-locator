package controllers

import (
	"fmt"
	"io"
	"log"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/laurawarren88/go_spa_backend.git/models"
	"gorm.io/gorm"
)

type PlaceController struct {
	DB *gorm.DB
}

func NewPlaceController(db *gorm.DB) *PlaceController {
	return &PlaceController{DB: db}
}

func (pc *PlaceController) CheckActivityOwnership(ctx *gin.Context) {
	userID, _ := ctx.Get("userID")
	activityID := ctx.Param("id")

	var place models.Place
	if err := pc.DB.Where("id = ?", activityID).First(&place).Error; err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"isOwner": false})
		return
	}

	var user models.User
	if err := pc.DB.First(&user, userID).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"isOwner": false})
		return
	}

	isOwner := place.UserID == userID.(uint) || user.IsAdmin
	ctx.JSON(http.StatusOK, gin.H{"isOwner": isOwner})
}

func (pc *PlaceController) RenderCreateActivityForm(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{
		"title": "Create a New Activity",
	})
}

func (pc *PlaceController) CreateActivity(ctx *gin.Context) {
	if ctx.Request.Method == "OPTIONS" {
		ctx.Status(http.StatusOK)
		return
	}

	userID, exists := ctx.Get("userID")
	if !exists {
		log.Println("No userID found in context")
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	userIDUint, ok := userID.(uint)
	if !ok {
		log.Printf("Failed to convert userID to uint: %v", userID)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID format"})
		return
	}

	var user models.User
	if err := pc.DB.First(&user, userIDUint).Error; err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	log.Printf("User found: %+v", user)

	type PlaceTextFields struct {
		Name            string  `form:"name" json:"name"`
		Vicinity        string  `form:"vicinity" json:"vicinity"`
		City            string  `form:"city" json:"city"`
		Postcode        string  `form:"postcode" json:"postcode"`
		Phone           string  `form:"phone" json:"phone"`
		Email           string  `form:"email" json:"email"`
		Website         string  `form:"website" json:"website"`
		OpeningHours    string  `form:"opening_hours" json:"opening_hours"`
		Description     string  `form:"description" json:"description"`
		Type            string  `form:"type" json:"type"`
		Latitude        float64 `form:"latitude" json:"latitude"`
		Longitude       float64 `form:"longitude" json:"longitude"`
		Logo            string  `json:"logo" form:"logo" gorm:"size:255"`
		FacilitiesImage string  `json:"facilities_image" form:"facilities_image" gorm:"size:255"`
	}

	var placeFields PlaceTextFields
	contentType := ctx.GetHeader("Content-Type")

	if strings.HasPrefix(contentType, "application/json") {
		if err := ctx.ShouldBindJSON(&placeFields); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON input: " + err.Error()})
			return
		}
	} else if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := ctx.Request.ParseMultipartForm(32 << 20); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse form: " + err.Error()})
			return
		}

		placeFields.Name = ctx.Request.FormValue("name")
		placeFields.Vicinity = ctx.Request.FormValue("vicinity")
		placeFields.City = ctx.Request.FormValue("city")
		placeFields.Postcode = ctx.Request.FormValue("postcode")
		placeFields.Phone = ctx.Request.FormValue("phone")
		placeFields.Email = ctx.Request.FormValue("email")
		placeFields.Website = ctx.Request.FormValue("website")
		placeFields.OpeningHours = ctx.Request.FormValue("opening_hours")
		placeFields.Description = ctx.Request.FormValue("description")
		placeFields.Type = ctx.Request.FormValue("type")

		latitude, err := strconv.ParseFloat(ctx.Request.FormValue("latitude"), 64)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid latitude value"})
			return
		}
		placeFields.Latitude = latitude

		longitude, err := strconv.ParseFloat(ctx.Request.FormValue("longitude"), 64)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid longitude value"})
			return
		}
		placeFields.Longitude = longitude

		placeFields.Logo = ctx.Request.FormValue("logo")
		placeFields.FacilitiesImage = ctx.Request.FormValue("facilities_image")

		if logoFile, err := ctx.FormFile("logo"); err == nil {
			sanitizedFilename := fmt.Sprintf("%d_%s", time.Now().Unix(), filepath.Base(logoFile.Filename))
			logoFilePath := "./uploads/logos/" + sanitizedFilename

			if err := os.MkdirAll("./uploads/logos", os.ModePerm); err != nil {
				ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create directory"})
				return
			}

			if err := ctx.SaveUploadedFile(logoFile, logoFilePath); err != nil {
				ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save logo file"})
				return
			}

			placeFields.Logo = logoFilePath
		} else {
			log.Printf("No logo uploaded or error occurred: %v", err)
		}

		if facilitiesImageFile, err := ctx.FormFile("facilities_image"); err == nil {
			sanitizedFilename := fmt.Sprintf("%d_%s", time.Now().Unix(), filepath.Base(facilitiesImageFile.Filename))
			facilitiesImageFilePath := "./uploads/facilities/" + sanitizedFilename

			if err := os.MkdirAll("./uploads/facilities", os.ModePerm); err != nil {
				ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create directory"})
				return
			}

			if err := ctx.SaveUploadedFile(facilitiesImageFile, facilitiesImageFilePath); err != nil {
				ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save facilities image"})
				return
			}

			placeFields.FacilitiesImage = facilitiesImageFilePath
		} else {
			log.Printf("No facility image uploaded or error occurred: %v", err)
		}
	}

	if placeFields.Name == "" || placeFields.Latitude == 0 || placeFields.Longitude == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Missing required fields"})
		return
	}

	log.Printf("Activity Text Fields: %+v\n", placeFields)

	activity := models.Place{
		Name:            placeFields.Name,
		Vicinity:        placeFields.Vicinity,
		City:            placeFields.City,
		Postcode:        placeFields.Postcode,
		Phone:           placeFields.Phone,
		Email:           placeFields.Email,
		Website:         placeFields.Website,
		OpeningHours:    placeFields.OpeningHours,
		Description:     placeFields.Description,
		Type:            placeFields.Type,
		Latitude:        placeFields.Latitude,
		Longitude:       placeFields.Longitude,
		Logo:            placeFields.Logo,
		FacilitiesImage: placeFields.FacilitiesImage,
		UserID:          userIDUint,
	}

	if err := pc.DB.Create(&activity).Error; err != nil {
		log.Println("Error saving to database:", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create activity"})
		return
	}

	// After creating the activity
	if err := pc.DB.Preload("User").First(&activity, activity.ID).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve activity with user details"})
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"message":  "Activity created successfully",
		"activity": activity,
	})
}

func (pc *PlaceController) GetPlaceLocator(ctx *gin.Context) {
	var places []models.Place
	var filteredPlaces []models.Place

	// Add debug logging for initial query
	log.Printf("Starting place lookup")

	result := pc.DB.Preload("User").Find(&places)
	if result.Error != nil {
		log.Printf("Database error: %v", result.Error)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	log.Printf("Found %d places in database", len(places))

	typeParam := ctx.Query("type")
	latParam := ctx.Query("lat")
	lngParam := ctx.Query("lng")
	radiusParam := ctx.Query("radius")

	// Only filter if we have coordinates and radius
	if latParam != "" && lngParam != "" && radiusParam != "" {
		lat, _ := strconv.ParseFloat(latParam, 64)
		lng, _ := strconv.ParseFloat(lngParam, 64)
		radius, _ := strconv.ParseFloat(radiusParam, 64)

		for _, place := range places {
			// Type filter (case insensitive)
			if typeParam != "" && !strings.EqualFold(place.Type, typeParam) {
				continue
			}

			distance := calculateDistance(lat, lng, place.Latitude, place.Longitude)
			log.Printf("Distance for place %s: %f meters", place.Name, distance)

			if distance <= radius {
				filteredPlaces = append(filteredPlaces, place)
			}
		}
	}

	log.Printf("Filtered to %d places", len(filteredPlaces))

	ctx.JSON(http.StatusOK, gin.H{
		"places":  filteredPlaces,
		"message": "Locator Page",
		"total":   len(filteredPlaces),
	})
}

func (pc *PlaceController) GetActivityById(ctx *gin.Context) {
	id := ctx.Param("id")

	// Ensure the ID is a valid integer
	var place models.Place
	if err := pc.DB.Preload("User").First(&place, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Activity not found"})
		} else {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve activity"})
		}
		return
	}

	// Return the activity as JSON
	ctx.JSON(http.StatusOK, gin.H{
		"id":               place.ID,
		"name":             place.Name,
		"vicinity":         place.Vicinity,
		"city":             place.City,
		"postcode":         place.Postcode,
		"phone":            place.Phone,
		"email":            place.Email,
		"website":          place.Website,
		"opening_hours":    place.OpeningHours,
		"description":      place.Description,
		"type":             place.Type,
		"latitude":         place.Latitude,
		"longitude":        place.Longitude,
		"logo":             place.Logo,
		"facilities_image": place.FacilitiesImage,
		"userID":           place.UserID,
		"user":             place.User,
	})
}

func (pc *PlaceController) RenderEditActivityForm(ctx *gin.Context) {
	id := ctx.Param("id")
	var existingPlace models.Place

	// Find the activity by ID
	if err := pc.DB.First(&existingPlace, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Activity not found"})
		} else {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve activity"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"title":    "Update Activity Form",
		"activity": existingPlace,
	})
}

func (pc *PlaceController) UpdateActivity(ctx *gin.Context) {
	if ctx.Request.Method == "OPTIONS" {
		ctx.Status(http.StatusOK)
		return
	}

	id := ctx.Param("id")
	var existingPlace models.Place

	if err := pc.DB.First(&existingPlace, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Activity not found"})
		} else {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve activity"})
		}
		return
	}

	if err := ctx.Request.ParseMultipartForm(32 << 20); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse form data"})
		return
	}

	form := ctx.Request.MultipartForm

	if name := form.Value["name"]; len(name) > 0 {
		existingPlace.Name = name[0]
	}
	if vicinity := form.Value["vicinity"]; len(vicinity) > 0 {
		existingPlace.Vicinity = vicinity[0]
	}
	if city := form.Value["city"]; len(city) > 0 {
		existingPlace.City = city[0]
	}
	if postcode := form.Value["postcode"]; len(postcode) > 0 {
		existingPlace.Postcode = postcode[0]
	}
	if phone := form.Value["phone"]; len(phone) > 0 {
		existingPlace.Phone = phone[0]
	}
	if email := form.Value["email"]; len(email) > 0 {
		existingPlace.Email = email[0]
	}
	if website := form.Value["website"]; len(website) > 0 {
		existingPlace.Website = website[0]
	}
	if openingHours := form.Value["opening_hours"]; len(openingHours) > 0 {
		existingPlace.OpeningHours = openingHours[0]
	}
	if description := form.Value["description"]; len(description) > 0 {
		existingPlace.Description = description[0]
	}
	if typeField := form.Value["type"]; len(typeField) > 0 {
		existingPlace.Type = typeField[0]
	}
	if latitude := form.Value["latitude"]; len(latitude) > 0 {
		lat, err := strconv.ParseFloat(latitude[0], 64)
		if err == nil {
			existingPlace.Latitude = lat
		}
	}
	if longitude := form.Value["longitude"]; len(longitude) > 0 {
		lon, err := strconv.ParseFloat(longitude[0], 64)
		if err == nil {
			existingPlace.Longitude = lon
		}
	}
	if files, ok := form.File["logo"]; ok && len(files) > 0 {
		if existingPlace.Logo != "" {
			if err := os.Remove(existingPlace.Logo); err != nil {
				ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete existing logo"})
				return
			}
		}
		file := files[0]
		sanitizedFilename := fmt.Sprintf("%d_%s", time.Now().Unix(), filepath.Base(file.Filename))
		sanitizedFilename = filepath.Clean(sanitizedFilename)
		logoFilePath := "./uploads/logos/" + sanitizedFilename
		if err := saveUploadedFile(file, logoFilePath); err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save logo"})
			return
		}
		existingPlace.Logo = logoFilePath
	}
	if files, ok := form.File["facilities_image"]; ok && len(files) > 0 {
		if existingPlace.FacilitiesImage != "" {
			if err := os.Remove(existingPlace.FacilitiesImage); err != nil {
				ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete existing facilities image"})
				return
			}
		}
		file := files[0]
		sanitizedFilename := fmt.Sprintf("%d_%s", time.Now().Unix(), filepath.Base(file.Filename))
		sanitizedFilename = filepath.Clean(sanitizedFilename)
		facilitiesImageFilePath := "./uploads/facilities/" + sanitizedFilename
		if err := saveUploadedFile(file, facilitiesImageFilePath); err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save facilities image"})
			return
		}
		existingPlace.FacilitiesImage = facilitiesImageFilePath
	}

	if UserID := form.Value["userID"]; len(UserID) > 0 {
		userID, err := strconv.Atoi(UserID[0])
		if err == nil {
			existingPlace.UserID = uint(userID)
		}
	}

	if err := pc.DB.Save(&existingPlace).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update activity"})
		return
	}

	log.Printf("Received Form Values: %+v", form.Value)
	log.Printf("Received Files: %+v", form.File)

	ctx.JSON(http.StatusOK, gin.H{
		"message":  "Activity updated successfully",
		"activity": existingPlace,
	})
}

func (pc *PlaceController) RenderDeleteActivityForm(ctx *gin.Context) {
	id := ctx.Param("id")
	var existingPlace models.Place

	if err := pc.DB.First(&existingPlace, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Activity not found"})
		} else {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve activity"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"title":    "Delete Activity Form",
		"activity": existingPlace,
	})
}

func (pc *PlaceController) DeleteActivity(ctx *gin.Context) {
	if ctx.Request.Method == "OPTIONS" {
		ctx.Status(http.StatusOK)
		return
	}

	id := ctx.Param("id")
	var place models.Place

	// Check if the activity exists
	if err := pc.DB.First(&place, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Activity not found"})
		} else {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve activity"})
		}
		return
	}

	if place.Logo != "" {
		logoPath := place.Logo
		// fmt.Println("Logo Path:", logoPath)
		if fileExists(logoPath) {
			// fmt.Println("File exists:", logoPath)
			if err := os.Remove(logoPath); err != nil {
				fmt.Printf("Failed to delete logo file: %s: %s\n", logoPath, err)
			}
		}
	}

	// Handle facilities image deletion
	if place.FacilitiesImage != "" {
		facilitiesPath := place.FacilitiesImage
		// fmt.Println("Facilities Path:", facilitiesPath)
		if fileExists(facilitiesPath) {
			// fmt.Println("File exists:", facilitiesPath)
			if err := os.Remove(facilitiesPath); err != nil {
				fmt.Printf("Failed to delete facilities image file: %s: %s\n", facilitiesPath, err)
			}
		}
	}

	if err := pc.DB.Delete(&place).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete activity"})
		return
	}

	fmt.Println("Request Method:", ctx.Request.Method)
	ctx.JSON(http.StatusOK, gin.H{"message": "Activity deleted successfully"})
}

func fileExists(filePath string) bool {
	info, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func saveUploadedFile(file *multipart.FileHeader, path string) error {
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(path)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371e3 // Earth radius in meters
	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	deltaLat := (lat2 - lat1) * math.Pi / 180
	deltaLon := (lon2 - lon1) * math.Pi / 180

	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(deltaLon/2)*math.Sin(deltaLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadius * c
}
