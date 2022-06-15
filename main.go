package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// /subscribe?address
func subscribe(c *gin.Context) {
	c.Query("address")

}

func _main() {
	router := gin.Default()
	router.GET("/subscribe",
		func(context *gin.Context) {
			context.JSON(http.StatusOK, gin.H{
				"message": "pong",
			})
		})
	router.Run() // listen and serve on 0.0.0.0:8080 (for windows "localhost:8080")
}
