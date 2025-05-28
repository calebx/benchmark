package main

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mdlayher/vsock"
)

type Req struct {
	XID string `json:"xid"`
}

type Resp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	r := gin.Default()

	gin.SetMode(gin.ReleaseMode)

	r.POST("/echo", func(c *gin.Context) {
		var req Req

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, Resp{
				Code:    400,
				Message: "Bad Request 7",
			})
			return
		}

		xid := req.XID
		if len(xid) > 64 {
			xid = xid[:64]
		}

		time.Sleep(time.Millisecond)
		c.JSON(http.StatusOK, Resp{
			Code:    200,
			Message: reverse(xid),
		})
	})

	lis, err := vsock.Listen(5005, nil)
	if err != nil {
		panic(err)
	}
	defer lis.Close()

	srv := &http.Server{
		Handler: r,
	}

	log.Println("Gin HTTP server running on vsock:5005 ...")
	if err := srv.Serve(lis); err != nil && err != http.ErrServerClosed {
		log.Fatalf("srv.Serve failed: %v", err)
	}
}

func reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}
