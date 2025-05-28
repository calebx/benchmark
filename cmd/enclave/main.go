package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
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
				Message: "Bad Request 6",
			})
			return
		}

		xid := req.XID
		if len(xid) > 64 {
			xid = xid[:64]
		}

		c.JSON(http.StatusOK, Resp{
			Code:    200,
			Message: reverse(xid),
		})
	})

	_ = r.Run(":5005")
}

func reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}
