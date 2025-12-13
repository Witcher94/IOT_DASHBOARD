package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pfaka/iot-dashboard/internal/config"
	"github.com/pfaka/iot-dashboard/internal/database"
	"github.com/pfaka/iot-dashboard/internal/middleware"
	"github.com/pfaka/iot-dashboard/internal/models"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type AuthService struct {
	cfg         *config.Config
	db          *database.DB
	oauthConfig *oauth2.Config
}

type GoogleUserInfo struct {
	ID      string `json:"id"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

func NewAuthService(cfg *config.Config, db *database.DB) *AuthService {
	oauthConfig := &oauth2.Config{
		ClientID:     cfg.GoogleClientID,
		ClientSecret: cfg.GoogleSecret,
		RedirectURL:  cfg.GoogleCallback,
		Scopes:       []string{"openid", "profile", "email"},
		Endpoint:     google.Endpoint,
	}

	return &AuthService{
		cfg:         cfg,
		db:          db,
		oauthConfig: oauthConfig,
	}
}

func (s *AuthService) GetGoogleAuthURL(state string) string {
	return s.oauthConfig.AuthCodeURL(state)
}

func (s *AuthService) HandleGoogleCallback(ctx context.Context, code string) (*models.User, string, error) {
	token, err := s.oauthConfig.Exchange(ctx, code)
	if err != nil {
		return nil, "", fmt.Errorf("failed to exchange code: %w", err)
	}

	client := s.oauthConfig.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return nil, "", fmt.Errorf("failed to get user info: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response: %w", err)
	}

	var userInfo GoogleUserInfo
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, "", fmt.Errorf("failed to unmarshal user info: %w", err)
	}

	// Create or update user
	user := &models.User{
		Email:    userInfo.Email,
		Name:     userInfo.Name,
		Picture:  userInfo.Picture,
		GoogleID: userInfo.ID,
		IsAdmin:  userInfo.Email == s.cfg.AdminEmail,
	}

	if err := s.db.UpsertUserByGoogleID(ctx, user); err != nil {
		return nil, "", fmt.Errorf("failed to upsert user: %w", err)
	}

	// Generate JWT
	jwtToken, err := s.GenerateJWT(user)
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate JWT: %w", err)
	}

	return user, jwtToken, nil
}

func (s *AuthService) GenerateJWT(user *models.User) (string, error) {
	claims := &middleware.Claims{
		UserID:  user.ID.String(),
		Email:   user.Email,
		IsAdmin: user.IsAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.cfg.JWTSecret))
}

func (s *AuthService) ValidateToken(tokenString string) (*middleware.Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &middleware.Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(s.cfg.JWTSecret), nil
	})

	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(*middleware.Claims)
	if !ok {
		return nil, fmt.Errorf("invalid claims")
	}

	return claims, nil
}

func (s *AuthService) RefreshToken(ctx context.Context, tokenString string) (string, error) {
	claims, err := s.ValidateToken(tokenString)
	if err != nil {
		return "", err
	}

	user, err := s.db.GetUserByEmail(ctx, claims.Email)
	if err != nil {
		return "", fmt.Errorf("user not found")
	}

	return s.GenerateJWT(user)
}

// HTTPClient interface for testing
type HTTPClient interface {
	Get(url string) (*http.Response, error)
}


