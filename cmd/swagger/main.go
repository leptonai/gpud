package main

import (
	"log"

	"github.com/gin-gonic/gin"

	_ "github.com/leptonai/gpud/docs/apis"
)

func main() {
	r := gin.Default()

	r.StaticFile("/static/api/v1/openapi.yaml", "./docs/apis/swagger.json")

	api := r.Group("/api")
	v1 := api.Group("/v1")

	v1.StaticFile("/docs", "./docs/apis/index.html")

	log.Panic(r.Run(":80"))
}
