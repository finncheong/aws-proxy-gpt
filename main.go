package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/akrylysov/algnhsa"
	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
)

type Claims struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.StandardClaims
}

const (
	openaiURL = "https://api.openai.com"
)

func GenerateToken(username string, role string) (string, error) {
	// Create the JWT claims, which includes the username and role
	claims := &Claims{
		Username: username,
		Role:     role,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: time.Now().Add(time.Hour * 24 * 7).Unix(), // Token will expire after 7 day
		},
	}

	// Create the JWT token with the claims
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign the token with a secret key
	secret := os.Getenv("SECRET_KEY")
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func ValidateToken(tokenString string) (*Claims, error) {
	// Parse the JWT token
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify that the signing method is valid
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		// Get the secret key
		secret := os.Getenv("SECRET_KEY")
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}

	// Extract the JWT claims
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	} else {
		return nil, fmt.Errorf("invalid token")
	}
}

func main() {
	r := gin.Default()

	// 跨域设置
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}
		token := c.Request.Header.Get("Authorization")
		tokenString := strings.Replace(token, "Bearer ", "", 1)
		_, err := ValidateToken(tokenString)
		if err != nil {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	})

	// 任意路径都会被匹配
	r.Any("/*any", func(ctx *gin.Context) {
		url := openaiURL + ctx.Request.URL.Path

		req, err := http.NewRequest(ctx.Request.Method, url, ctx.Request.Body)
		if err != nil {
			ctx.String(http.StatusInternalServerError, err.Error())
			return
		}
		apiKey := os.Getenv("OPENAI_API_KEY")
		ctx.Request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
		req.Header.Set("x-real-ip", "")
		req.Header = ctx.Request.Header
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			ctx.String(http.StatusInternalServerError, err.Error())
			return
		}
		defer resp.Body.Close()
		// 转发响应头
		for k, v := range resp.Header {
			ctx.Header(k, strings.Join(v, ","))
		}

		ctx.Status(resp.StatusCode)

		// 转发响应体
		for {
			buff := make([]byte, 256)
			var n int
			n, err = resp.Body.Read(buff)
			if err != nil {
				break
			}
			_, err = ctx.Writer.Write(buff[:n])
			if err != nil {
				break
			}
			ctx.Writer.Flush()
		}

		if err != nil && err != io.EOF {
			ctx.String(http.StatusInternalServerError, err.Error())
			return
		}
	})

	// 启动 Lambda 函数
	algnhsa.ListenAndServe(r, nil)
	// 启动本地服务，使用时请注释掉上面的 algnhsa.ListenAndServe(r, nil)
	// r.Run(":12450")
}
