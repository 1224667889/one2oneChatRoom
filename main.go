package main

import (
	"github.com/gin-gonic/gin"
	"one2oneChatRoom/ws"
)

func main() {

	gin.SetMode(gin.ReleaseMode) //线上环境

	go ws.Manager.Start()
	r := gin.Default()
	r.GET("/ws",ws.WsHandler)
	r.Run(":8282") // listen and serve on 0.0.0.0:8080
}