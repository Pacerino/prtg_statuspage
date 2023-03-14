package main

import (
	"net"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/sirupsen/logrus"
)

type Incidents struct {
	gorm.Model
	IncidentDeviceID uint   `form:"incidentdeviceid" gorm:"unique" binding:"required"` // ID des Probes
	IncidentType     string `form:"incidenttype" binding:"required"`                   // Typ des Ereignis (Up, Down, Warning)
	IncidentDate     string `form:"incidentdate" binding:"required"`                   // Datum des Ereignis
	IncidentTime     string `form:"incidenttime" binding:"required"`                   // Zeit des Ereignis
	IncidentDetails  string `form:"incidentdetails" binding:"required"`                // Details des Ereignis
}

type Handler struct {
	http *gin.Engine
	db   *gorm.DB
}

var log = &logrus.Logger{
	Out:       os.Stdout,
	Formatter: new(logrus.TextFormatter),
	Hooks:     make(logrus.LevelHooks),
	Level:     logrus.InfoLevel,
}

func init() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	for _, env := range []string{"API_TOKEN", "DB_NAME", "HTML_TITLE", "HTTP_PORT", "HTTP_HOST"} {
		if len(os.Getenv(env)) == 0 {
			log.Fatalf("Missing %s from environment", env)
		}
	}
	db, err := gorm.Open(sqlite.Open(os.Getenv("DB_NAME")), &gorm.Config{})
	if err != nil {
		log.Panic("failed to connect database")
	}
	if err = db.AutoMigrate(&Incidents{}); err != nil {
		log.Panic("failed to migrate database")
	}
	handler := &Handler{
		http: gin.Default(),
		db:   db,
	}
	handler.http.LoadHTMLGlob("templates/*")
	handler.Routes()
	handler.Run()
}

func main() {}

func (h *Handler) Run() {

	h.http.Use(Logger(log), gin.Recovery())

	h.http.SetTrustedProxies(nil)

	server := &http.Server{Handler: h.http}
	l, err := net.Listen("tcp4", os.Getenv("HTTP_HOST")+":"+os.Getenv("HTTP_PORT"))
	if err != nil {
		log.Fatal(err)
	}
	log.Info("Server started on " + os.Getenv("HTTP_HOST") + ":" + os.Getenv("HTTP_PORT"))
	if err = server.Serve(l); err != nil {
		log.Fatal(err)
	}
}

func (h *Handler) Routes() {
	h.http.GET("/", h.showIncidents)

	authorized := h.http.Group("/api")
	authorized.Use(TokenAuthMiddleware())
	{
		authorized.POST("/incident", h.createIncident)
	}
}

func TokenAuthMiddleware() gin.HandlerFunc {
	requiredToken := os.Getenv("API_TOKEN")
	return func(c *gin.Context) {
		token := c.Query("api_token")
		if token == "" {
			log.Error("API token required")
			c.AbortWithStatusJSON(401, gin.H{"error": "API token required"})
			return
		}
		if token != requiredToken {
			log.Error("Invalid API Token")
			c.AbortWithStatusJSON(401, gin.H{"error": "Invalid API Token"})
			return
		}
		c.Next()
	}
}

func (h *Handler) createIncident(c *gin.Context) {
	var incident Incidents
	if err := c.ShouldBind(&incident); err != nil {
		log.Error(err.Error())
		c.AbortWithStatusJSON(400, gin.H{"error": err.Error()})
		return
	}
	if incident.IncidentType == "Warnung" || incident.IncidentType == "Fehler" {
		// Gerät ist in Warnung, erstelle Fehlermeldung
		if err := h.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "incident_device_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"incident_type"}),
		}).Create(&incident).Error; err != nil {
			log.Error(err.Error())
			c.AbortWithStatusJSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"message": "Incident created successfully"})
	} else if incident.IncidentType == "OK" {
		// Gerät ist nicht mehr in Warnung, lösche aus der DB
		h.db.Delete(&incident, "incident_device_id = ?", incident.IncidentDeviceID)
		c.JSON(200, gin.H{"message": "Incident deleted successfully"})
	} else {
		log.Error("Unkown incident type" + incident.IncidentType)
		c.AbortWithStatusJSON(500, gin.H{"error": "Unkown incident type" + incident.IncidentType})
	}

}

func (h *Handler) showIncidents(c *gin.Context) {
	var incidents []Incidents
	err := h.db.Find(&incidents).Error
	if err != nil {
		c.AbortWithStatusJSON(500, gin.H{"error": err.Error()})
		return
	}
	c.HTML(http.StatusOK, "index.tmpl", gin.H{
		"incidents": incidents,
		"title":     os.Getenv("HTML_TITLE"),
	})
}
