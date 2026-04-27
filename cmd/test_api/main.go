package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
)

func main() {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	// Login
	loginBody := `{"email":"customer@test.com","password":"password123"}`
	resp, err := client.Post("http://localhost:8080/api/login", "application/json", strings.NewReader(loginBody))
	if err != nil {
		fmt.Printf("Login error: %v\n", err)
		return
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	fmt.Printf("Login: %d %s\n", resp.StatusCode, string(body))
	fmt.Printf("Cookies: %v\n", jar.Cookies(resp.Request.URL))

	// Check-login
	resp2, err := client.Get("http://localhost:8080/api/check-login")
	if err != nil {
		fmt.Printf("Check-login error: %v\n", err)
		return
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	fmt.Printf("Check-login: %d %s\n", resp2.StatusCode, string(body2))

	// Customer profile
	resp3, err := client.Get("http://localhost:8080/api/customer/profile")
	if err != nil {
		fmt.Printf("Profile error: %v\n", err)
		return
	}
	body3, _ := io.ReadAll(resp3.Body)
	resp3.Body.Close()
	fmt.Printf("Customer profile: %d %s\n", resp3.StatusCode, string(body3))
}
