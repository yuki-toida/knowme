package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	_ "github.com/jinzhu/gorm/dialects/mysql"

	"github.com/yuki-toida/knowme/config"
	"github.com/yuki-toida/knowme/controller"
	"github.com/yuki-toida/knowme/model"
)

func init() {
	config.Initialize()
	model.Migrate()
}

func main() {
	router := gin.Default()
	router.LoadHTMLFiles("index.html")

	if config.Config.Env == "local" {
		router.StaticFS("/static", http.Dir("static"))
	}

	router.GET("/healthz", controller.CalendarHealthz)
	router.GET("/", controller.Calendar)
	router.POST("/init", controller.CalendarInit)
	router.POST("/", controller.CalendarAdd)
	router.PUT("/", controller.CalendarDelete)

	router.Run(":" + config.Config.Server.Port)
}
