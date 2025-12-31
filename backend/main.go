package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"gopkg.in/ini.v1"
)

const (
	cacheDir            = ".docker/aws-mfa-cache"
	defaultDuration     = 43200 // 12 hours
	expiryBufferSeconds = 300   // 5 minutes
)

type CachedCredentials struct {
	AccessKeyID     string    `json:"accessKeyId"`
	SecretAccessKey string    `json:"secretAccessKey"`
	SessionToken    string    `json:"sessionToken"`
	Expiration      time.Time `json:"expiration"`
	Profile         string    `json:"profile"`
}

type ProfileInfo struct {
	Name      string `json:"name"`
	Region    string `json:"region"`
	MFASerial string `json:"mfaSerial"`
}

type LoginRequest struct {
	Profile   string `json:"profile"`
	TokenCode string `json:"tokenCode"`
	Duration  int32  `json:"duration,omitempty"`
}

type StatusResponse struct {
	Profile       string     `json:"profile"`
	Authenticated bool       `json:"authenticated"`
	Expiration    *time.Time `json:"expiration,omitempty"`
	TimeRemaining string     `json:"timeRemaining,omitempty"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

func getCacheDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, cacheDir)
}

func getCacheFile(profile string) string {
	if profile == "" {
		profile = "default"
	}
	return filepath.Join(getCacheDir(), profile+".json")
}

func getAWSConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".aws", "config")
}

func getAWSCredentialsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".aws", "credentials")
}

func loadCachedCredentials(profile string) (*CachedCredentials, error) {
	cacheFile := getCacheFile(profile)
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return nil, err
	}

	var creds CachedCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, err
	}

	return &creds, nil
}

func saveCachedCredentials(creds *CachedCredentials) error {
	if err := os.MkdirAll(getCacheDir(), 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}

	cacheFile := getCacheFile(creds.Profile)
	return os.WriteFile(cacheFile, data, 0600)
}

func isCredentialsValid(creds *CachedCredentials) bool {
	if creds == nil {
		return false
	}
	return time.Now().Add(time.Duration(expiryBufferSeconds) * time.Second).Before(creds.Expiration)
}

func getProfiles() ([]ProfileInfo, error) {
	configPath := getAWSConfigPath()
	cfg, err := ini.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	var profiles []ProfileInfo

	for _, section := range cfg.Sections() {
		name := section.Name()
		if name == "DEFAULT" {
			continue
		}

		// Handle profile name format
		profileName := name
		if len(name) > 8 && name[:8] == "profile " {
			profileName = name[8:]
		}

		mfaSerial := section.Key("mfa_serial").String()
		if mfaSerial == "" {
			continue // Skip profiles without MFA
		}

		profiles = append(profiles, ProfileInfo{
			Name:      profileName,
			Region:    section.Key("region").String(),
			MFASerial: mfaSerial,
		})
	}

	return profiles, nil
}

func getMFASerial(profile string) (string, error) {
	configPath := getAWSConfigPath()
	cfg, err := ini.Load(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to load AWS config: %w", err)
	}

	sectionName := profile
	if profile != "default" {
		sectionName = "profile " + profile
	}

	section, err := cfg.GetSection(sectionName)
	if err != nil {
		return "", fmt.Errorf("profile not found: %s", profile)
	}

	mfaSerial := section.Key("mfa_serial").String()
	if mfaSerial == "" {
		return "", fmt.Errorf("no mfa_serial configured for profile: %s", profile)
	}

	return mfaSerial, nil
}

func performMFALogin(ctx context.Context, profile, tokenCode string, duration int32) (*CachedCredentials, error) {
	mfaSerial, err := getMFASerial(profile)
	if err != nil {
		return nil, err
	}

	// Load AWS config for the profile
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(profile),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create STS client
	stsClient := sts.NewFromConfig(cfg)

	// Get session token with MFA
	result, err := stsClient.GetSessionToken(ctx, &sts.GetSessionTokenInput{
		DurationSeconds: aws.Int32(duration),
		SerialNumber:    aws.String(mfaSerial),
		TokenCode:       aws.String(tokenCode),
	})
	if err != nil {
		return nil, fmt.Errorf("MFA authentication failed: %w", err)
	}

	creds := &CachedCredentials{
		AccessKeyID:     *result.Credentials.AccessKeyId,
		SecretAccessKey: *result.Credentials.SecretAccessKey,
		SessionToken:    *result.Credentials.SessionToken,
		Expiration:      *result.Credentials.Expiration,
		Profile:         profile,
	}

	if err := saveCachedCredentials(creds); err != nil {
		return nil, fmt.Errorf("failed to cache credentials: %w", err)
	}

	return creds, nil
}

func formatTimeRemaining(expiration time.Time) string {
	remaining := time.Until(expiration)
	if remaining < 0 {
		return "expired"
	}

	hours := int(remaining.Hours())
	minutes := int(remaining.Minutes()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// Handlers

func handleGetProfiles(c echo.Context) error {
	profiles, err := getProfiles()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Failed to load profiles",
			Details: err.Error(),
		})
	}
	return c.JSON(http.StatusOK, profiles)
}

func handleGetStatus(c echo.Context) error {
	profile := c.QueryParam("profile")
	if profile == "" {
		profile = "default"
	}

	creds, err := loadCachedCredentials(profile)
	if err != nil || !isCredentialsValid(creds) {
		return c.JSON(http.StatusOK, StatusResponse{
			Profile:       profile,
			Authenticated: false,
		})
	}

	return c.JSON(http.StatusOK, StatusResponse{
		Profile:       profile,
		Authenticated: true,
		Expiration:    &creds.Expiration,
		TimeRemaining: formatTimeRemaining(creds.Expiration),
	})
}

func handleGetAllStatus(c echo.Context) error {
	profiles, err := getProfiles()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Failed to load profiles",
			Details: err.Error(),
		})
	}

	var statuses []StatusResponse
	for _, p := range profiles {
		creds, err := loadCachedCredentials(p.Name)
		status := StatusResponse{
			Profile:       p.Name,
			Authenticated: err == nil && isCredentialsValid(creds),
		}
		if status.Authenticated {
			status.Expiration = &creds.Expiration
			status.TimeRemaining = formatTimeRemaining(creds.Expiration)
		}
		statuses = append(statuses, status)
	}

	return c.JSON(http.StatusOK, statuses)
}

func handleLogin(c echo.Context) error {
	var req LoginRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid request body",
		})
	}

	if req.Profile == "" {
		req.Profile = "default"
	}
	if req.Duration == 0 {
		req.Duration = defaultDuration
	}
	if req.TokenCode == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Token code is required",
		})
	}

	creds, err := performMFALogin(c.Request().Context(), req.Profile, req.TokenCode, req.Duration)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "Authentication failed",
			Details: err.Error(),
		})
	}

	return c.JSON(http.StatusOK, StatusResponse{
		Profile:       creds.Profile,
		Authenticated: true,
		Expiration:    &creds.Expiration,
		TimeRemaining: formatTimeRemaining(creds.Expiration),
	})
}

func handleGetCredentials(c echo.Context) error {
	profile := c.QueryParam("profile")
	if profile == "" {
		profile = "default"
	}

	creds, err := loadCachedCredentials(profile)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "No cached credentials found",
		})
	}

	if !isCredentialsValid(creds) {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "Credentials expired",
		})
	}

	return c.JSON(http.StatusOK, creds)
}

func handleGetEnvFile(c echo.Context) error {
	profile := c.QueryParam("profile")
	if profile == "" {
		profile = "default"
	}

	creds, err := loadCachedCredentials(profile)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "No cached credentials found",
		})
	}

	if !isCredentialsValid(creds) {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "Credentials expired",
		})
	}

	envContent := fmt.Sprintf("AWS_ACCESS_KEY_ID=%s\nAWS_SECRET_ACCESS_KEY=%s\nAWS_SESSION_TOKEN=%s\n",
		creds.AccessKeyID, creds.SecretAccessKey, creds.SessionToken)

	return c.String(http.StatusOK, envContent)
}

func handleClearCredentials(c echo.Context) error {
	profile := c.QueryParam("profile")

	if profile == "" {
		// Clear all
		files, _ := filepath.Glob(filepath.Join(getCacheDir(), "*.json"))
		for _, f := range files {
			os.Remove(f)
		}
		return c.JSON(http.StatusOK, map[string]string{"message": "All credentials cleared"})
	}

	cacheFile := getCacheFile(profile)
	if err := os.Remove(cacheFile); err != nil && !os.IsNotExist(err) {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to clear credentials",
		})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Credentials cleared for " + profile})
}

func handleExportEnvFile(c echo.Context) error {
	profile := c.QueryParam("profile")
	if profile == "" {
		profile = "default"
	}

	outputPath := c.QueryParam("path")
	if outputPath == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Output path is required",
		})
	}

	creds, err := loadCachedCredentials(profile)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "No cached credentials found",
		})
	}

	if !isCredentialsValid(creds) {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "Credentials expired",
		})
	}

	envContent := fmt.Sprintf("AWS_ACCESS_KEY_ID=%s\nAWS_SECRET_ACCESS_KEY=%s\nAWS_SESSION_TOKEN=%s\n",
		creds.AccessKeyID, creds.SecretAccessKey, creds.SessionToken)

	if err := os.WriteFile(outputPath, []byte(envContent), 0600); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Failed to write env file",
			Details: err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Env file written to " + outputPath,
		"path":    outputPath,
	})
}

func main() {
	var socketPath string
	flag.StringVar(&socketPath, "socket", "/run/guest-services/backend.sock", "Unix socket path")
	flag.Parse()

	// Ensure cache directory exists
	os.MkdirAll(getCacheDir(), 0700)

	e := echo.New()
	e.HideBanner = true

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// Routes
	e.GET("/profiles", handleGetProfiles)
	e.GET("/status", handleGetStatus)
	e.GET("/status/all", handleGetAllStatus)
	e.POST("/login", handleLogin)
	e.GET("/credentials", handleGetCredentials)
	e.GET("/env", handleGetEnvFile)
	e.POST("/env/export", handleExportEnvFile)
	e.DELETE("/credentials", handleClearCredentials)

	// Health check
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	// Remove existing socket file
	os.Remove(socketPath)

	// Listen on Unix socket
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to listen on socket: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Backend listening on %s\n", socketPath)
	if err := http.Serve(listener, e); err != nil && err != io.EOF {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
