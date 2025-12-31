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
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"gopkg.in/ini.v1"
)

const (
	cacheDir            = ".docker/aws-mfa-cache"
	defaultDuration     = 43200 // 12 hours
	expiryBufferSeconds = 300   // 5 minutes
	settingsFile        = ".docker/aws-mfa-cache/settings.json"
)

// CredentialSource represents where to look for AWS credentials
type CredentialSource string

const (
	SourceAuto        CredentialSource = "auto"
	SourceLinux       CredentialSource = "linux"
	SourceWSL2        CredentialSource = "wsl2"
	SourceWindows     CredentialSource = "windows"
	SourceCustom      CredentialSource = "custom"
)

// Settings stores user preferences
type Settings struct {
	CredentialSource CredentialSource `json:"credentialSource"`
	CustomConfigPath string           `json:"customConfigPath,omitempty"`
	CustomCredsPath  string           `json:"customCredsPath,omitempty"`
	WSL2Distro       string           `json:"wsl2Distro,omitempty"`
}

// EnvironmentInfo provides information about the runtime environment
type EnvironmentInfo struct {
	IsWSL2          bool             `json:"isWsl2"`
	IsWindows       bool             `json:"isWindows"`
	IsLinux         bool             `json:"isLinux"`
	IsMacOS         bool             `json:"isMacOS"`
	WSL2Distros     []string         `json:"wsl2Distros,omitempty"`
	DetectedPaths   []AWSPathInfo    `json:"detectedPaths"`
	ActiveSource    CredentialSource `json:"activeSource"`
	HomeDir         string           `json:"homeDir"`
	WindowsHomeDir  string           `json:"windowsHomeDir,omitempty"`
}

// AWSPathInfo describes a potential AWS config location
type AWSPathInfo struct {
	Source      CredentialSource `json:"source"`
	ConfigPath  string           `json:"configPath"`
	CredsPath   string           `json:"credsPath"`
	Exists      bool             `json:"exists"`
	Description string           `json:"description"`
}

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
	Source    string `json:"source,omitempty"`
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

var currentSettings *Settings

// WSL2 and environment detection

func isWSL2() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	// Check for WSL2 specific indicators
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	version := strings.ToLower(string(data))
	return strings.Contains(version, "microsoft") || strings.Contains(version, "wsl")
}

func getWSL2Distros() []string {
	if runtime.GOOS != "windows" {
		return nil
	}
	cmd := exec.Command("wsl", "--list", "--quiet")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var distros []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Remove BOM and null characters from WSL output
		line = strings.Trim(line, "\x00\xef\xbb\xbf")
		if line != "" {
			distros = append(distros, line)
		}
	}
	return distros
}

func getWindowsHomeFromWSL2() string {
	if !isWSL2() {
		return ""
	}
	// Try to get Windows username
	cmd := exec.Command("cmd.exe", "/c", "echo %USERPROFILE%")
	output, err := cmd.Output()
	if err != nil {
		// Fallback: try common paths
		possibleUsers := []string{}
		entries, _ := os.ReadDir("/mnt/c/Users")
		for _, e := range entries {
			if e.IsDir() && e.Name() != "Public" && e.Name() != "Default" && e.Name() != "Default User" {
				possibleUsers = append(possibleUsers, e.Name())
			}
		}
		if len(possibleUsers) == 1 {
			return filepath.Join("/mnt/c/Users", possibleUsers[0])
		}
		return ""
	}
	winPath := strings.TrimSpace(string(output))
	// Convert Windows path to WSL path
	if strings.HasPrefix(winPath, "C:") {
		return "/mnt/c" + strings.ReplaceAll(winPath[2:], "\\", "/")
	}
	return ""
}

func getWSL2PathFromWindows(distro string, linuxPath string) string {
	// Convert a Linux path to Windows accessible path via \\wsl$
	return fmt.Sprintf("\\\\wsl$\\%s%s", distro, strings.ReplaceAll(linuxPath, "/", "\\"))
}

// Path discovery

func discoverAWSPaths() []AWSPathInfo {
	var paths []AWSPathInfo

	// Native Linux/macOS path
	homeDir, _ := os.UserHomeDir()
	nativePath := AWSPathInfo{
		Source:      SourceLinux,
		ConfigPath:  filepath.Join(homeDir, ".aws", "config"),
		CredsPath:   filepath.Join(homeDir, ".aws", "credentials"),
		Description: "Native home directory",
	}
	if runtime.GOOS == "darwin" {
		nativePath.Source = SourceLinux // Treat macOS same as Linux
		nativePath.Description = "macOS home directory"
	}
	_, err := os.Stat(nativePath.ConfigPath)
	nativePath.Exists = err == nil
	paths = append(paths, nativePath)

	// WSL2-specific paths
	if isWSL2() {
		// Windows home from WSL2
		winHome := getWindowsHomeFromWSL2()
		if winHome != "" {
			winPath := AWSPathInfo{
				Source:      SourceWindows,
				ConfigPath:  filepath.Join(winHome, ".aws", "config"),
				CredsPath:   filepath.Join(winHome, ".aws", "credentials"),
				Description: "Windows home directory (via /mnt/c)",
			}
			_, err := os.Stat(winPath.ConfigPath)
			winPath.Exists = err == nil
			paths = append(paths, winPath)
		}
	}

	// Windows native paths
	if runtime.GOOS == "windows" {
		userProfile := os.Getenv("USERPROFILE")
		if userProfile != "" {
			winPath := AWSPathInfo{
				Source:      SourceWindows,
				ConfigPath:  filepath.Join(userProfile, ".aws", "config"),
				CredsPath:   filepath.Join(userProfile, ".aws", "credentials"),
				Description: "Windows USERPROFILE",
				Exists:      true,
			}
			_, err := os.Stat(winPath.ConfigPath)
			winPath.Exists = err == nil
			paths = append(paths, winPath)
		}

		// Check WSL2 distros from Windows
		for _, distro := range getWSL2Distros() {
			wslPath := AWSPathInfo{
				Source:      SourceWSL2,
				ConfigPath:  getWSL2PathFromWindows(distro, "/home"),
				CredsPath:   getWSL2PathFromWindows(distro, "/home"),
				Description: fmt.Sprintf("WSL2 distro: %s", distro),
			}
			paths = append(paths, wslPath)
		}
	}

	return paths
}

func getEnvironmentInfo() *EnvironmentInfo {
	info := &EnvironmentInfo{
		IsWSL2:    isWSL2(),
		IsWindows: runtime.GOOS == "windows",
		IsLinux:   runtime.GOOS == "linux" && !isWSL2(),
		IsMacOS:   runtime.GOOS == "darwin",
	}

	info.HomeDir, _ = os.UserHomeDir()
	info.DetectedPaths = discoverAWSPaths()

	if isWSL2() {
		info.WindowsHomeDir = getWindowsHomeFromWSL2()
	}

	if runtime.GOOS == "windows" {
		info.WSL2Distros = getWSL2Distros()
	}

	settings := loadSettings()
	info.ActiveSource = settings.CredentialSource

	return info
}

// Settings management

func getSettingsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, settingsFile)
}

func loadSettings() *Settings {
	if currentSettings != nil {
		return currentSettings
	}

	settings := &Settings{
		CredentialSource: SourceAuto,
	}

	data, err := os.ReadFile(getSettingsPath())
	if err == nil {
		json.Unmarshal(data, settings)
	}

	currentSettings = settings
	return settings
}

func saveSettings(settings *Settings) error {
	currentSettings = settings

	dir := filepath.Dir(getSettingsPath())
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(getSettingsPath(), data, 0600)
}

// AWS path resolution based on settings

func getAWSConfigPath() string {
	settings := loadSettings()

	switch settings.CredentialSource {
	case SourceCustom:
		if settings.CustomConfigPath != "" {
			return settings.CustomConfigPath
		}
	case SourceWindows:
		if isWSL2() {
			winHome := getWindowsHomeFromWSL2()
			if winHome != "" {
				return filepath.Join(winHome, ".aws", "config")
			}
		} else if runtime.GOOS == "windows" {
			userProfile := os.Getenv("USERPROFILE")
			return filepath.Join(userProfile, ".aws", "config")
		}
	case SourceLinux, SourceWSL2:
		// Use native Linux path
	case SourceAuto:
		// Auto-detect: prefer existing paths
		paths := discoverAWSPaths()
		for _, p := range paths {
			if p.Exists {
				return p.ConfigPath
			}
		}
	}

	// Default to native home
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".aws", "config")
}

func getAWSCredentialsPath() string {
	settings := loadSettings()

	switch settings.CredentialSource {
	case SourceCustom:
		if settings.CustomCredsPath != "" {
			return settings.CustomCredsPath
		}
	case SourceWindows:
		if isWSL2() {
			winHome := getWindowsHomeFromWSL2()
			if winHome != "" {
				return filepath.Join(winHome, ".aws", "credentials")
			}
		} else if runtime.GOOS == "windows" {
			userProfile := os.Getenv("USERPROFILE")
			return filepath.Join(userProfile, ".aws", "credentials")
		}
	case SourceLinux, SourceWSL2:
		// Use native Linux path
	case SourceAuto:
		// Auto-detect: prefer existing paths
		paths := discoverAWSPaths()
		for _, p := range paths {
			if p.Exists {
				return p.CredsPath
			}
		}
	}

	// Default to native home
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".aws", "credentials")
}

// Cache management

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

// Profile management

func getProfiles() ([]ProfileInfo, error) {
	configPath := getAWSConfigPath()
	cfg, err := ini.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config from %s: %w", configPath, err)
	}

	var profiles []ProfileInfo
	settings := loadSettings()

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
			Source:    string(settings.CredentialSource),
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

func getProfileCredentials(profile string) (accessKey, secretKey string, err error) {
	credsPath := getAWSCredentialsPath()
	cfg, err := ini.Load(credsPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to load AWS credentials: %w", err)
	}

	section, err := cfg.GetSection(profile)
	if err != nil {
		return "", "", fmt.Errorf("profile not found in credentials: %s", profile)
	}

	accessKey = section.Key("aws_access_key_id").String()
	secretKey = section.Key("aws_secret_access_key").String()

	if accessKey == "" || secretKey == "" {
		return "", "", fmt.Errorf("missing credentials for profile: %s", profile)
	}

	return accessKey, secretKey, nil
}

func performMFALogin(ctx context.Context, profile, tokenCode string, duration int32) (*CachedCredentials, error) {
	mfaSerial, err := getMFASerial(profile)
	if err != nil {
		return nil, err
	}

	// Get base credentials from the credentials file
	accessKey, secretKey, err := getProfileCredentials(profile)
	if err != nil {
		return nil, err
	}

	// Load AWS config with explicit credentials
	configPath := getAWSConfigPath()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigFiles([]string{configPath}),
		config.WithSharedCredentialsFiles([]string{getAWSCredentialsPath()}),
		config.WithSharedConfigProfile(profile),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
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

// HTTP Handlers

func handleGetEnvironment(c echo.Context) error {
	return c.JSON(http.StatusOK, getEnvironmentInfo())
}

func handleGetSettings(c echo.Context) error {
	return c.JSON(http.StatusOK, loadSettings())
}

func handleUpdateSettings(c echo.Context) error {
	var settings Settings
	if err := c.Bind(&settings); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid settings",
		})
	}

	if err := saveSettings(&settings); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Failed to save settings",
			Details: err.Error(),
		})
	}

	return c.JSON(http.StatusOK, settings)
}

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
			if !strings.HasSuffix(f, "settings.json") {
				os.Remove(f)
			}
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

	// Load settings on startup
	loadSettings()

	e := echo.New()
	e.HideBanner = true

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// Environment and settings routes
	e.GET("/environment", handleGetEnvironment)
	e.GET("/settings", handleGetSettings)
	e.PUT("/settings", handleUpdateSettings)

	// Profile and credential routes
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
	fmt.Printf("Environment: WSL2=%v, Windows=%v, Linux=%v\n", isWSL2(), runtime.GOOS == "windows", runtime.GOOS == "linux")

	if err := http.Serve(listener, e); err != nil && err != io.EOF {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
