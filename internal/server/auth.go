package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// JWTValidator validates JWT tokens.
type JWTValidator struct {
	secret []byte
}

// NewJWTValidator creates a new JWT validator with the given secret.
func NewJWTValidator(secret string) *JWTValidator {
	return &JWTValidator{
		secret: []byte(secret),
	}
}

// UserClaims represents the claims in a JWT token.
type UserClaims struct {
	// UserID is the unique identifier for the user.
	UserID string `json:"sub"`
	// Email is the user's email address.
	Email string `json:"email"`
	// Name is the user's display name.
	Name string `json:"name"`
	// Roles are the user's assigned roles.
	Roles []string `json:"roles"`
	// IssuedAt is when the token was issued.
	IssuedAt time.Time `json:"iat"`
	// ExpiresAt is when the token expires.
	ExpiresAt time.Time `json:"exp"`
	// Issuer is who issued the token.
	Issuer string `json:"iss"`
}

// jwtHeader represents the JWT header.
type jwtHeader struct {
	Algorithm string `json:"alg"`
	Type      string `json:"typ"`
}

// jwtClaims represents the raw JWT claims for parsing.
type jwtClaims struct {
	Subject   string   `json:"sub"`
	Email     string   `json:"email"`
	Name      string   `json:"name"`
	Roles     []string `json:"roles"`
	IssuedAt  int64    `json:"iat"`
	ExpiresAt int64    `json:"exp"`
	Issuer    string   `json:"iss"`
}

// Validate validates a JWT token and returns the claims.
func (v *JWTValidator) Validate(token string) (*UserClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid token format")
	}

	// Verify header
	headerBytes, err := base64URLDecode(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid header encoding: %w", err)
	}

	var header jwtHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("invalid header: %w", err)
	}

	if header.Algorithm != "HS256" {
		return nil, fmt.Errorf("unsupported algorithm: %s", header.Algorithm)
	}

	if header.Type != "JWT" {
		return nil, fmt.Errorf("unsupported type: %s", header.Type)
	}

	// Verify signature
	signingInput := parts[0] + "." + parts[1]
	expectedSig := v.sign([]byte(signingInput))
	actualSig, err := base64URLDecode(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid signature encoding: %w", err)
	}

	if !hmac.Equal(expectedSig, actualSig) {
		return nil, errors.New("invalid signature")
	}

	// Parse claims
	claimsBytes, err := base64URLDecode(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid claims encoding: %w", err)
	}

	var claims jwtClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return nil, fmt.Errorf("invalid claims: %w", err)
	}

	// Check expiration
	expiresAt := time.Unix(claims.ExpiresAt, 0)
	if time.Now().After(expiresAt) {
		return nil, errors.New("token expired")
	}

	return &UserClaims{
		UserID:    claims.Subject,
		Email:     claims.Email,
		Name:      claims.Name,
		Roles:     claims.Roles,
		IssuedAt:  time.Unix(claims.IssuedAt, 0),
		ExpiresAt: expiresAt,
		Issuer:    claims.Issuer,
	}, nil
}

// GenerateToken generates a JWT token for the given claims.
func (v *JWTValidator) GenerateToken(claims *UserClaims) (string, error) {
	header := jwtHeader{
		Algorithm: "HS256",
		Type:      "JWT",
	}

	headerBytes, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("failed to marshal header: %w", err)
	}

	rawClaims := jwtClaims{
		Subject:   claims.UserID,
		Email:     claims.Email,
		Name:      claims.Name,
		Roles:     claims.Roles,
		IssuedAt:  claims.IssuedAt.Unix(),
		ExpiresAt: claims.ExpiresAt.Unix(),
		Issuer:    claims.Issuer,
	}

	claimsBytes, err := json.Marshal(rawClaims)
	if err != nil {
		return "", fmt.Errorf("failed to marshal claims: %w", err)
	}

	headerEncoded := base64URLEncode(headerBytes)
	claimsEncoded := base64URLEncode(claimsBytes)

	signingInput := headerEncoded + "." + claimsEncoded
	signature := v.sign([]byte(signingInput))
	signatureEncoded := base64URLEncode(signature)

	return signingInput + "." + signatureEncoded, nil
}

// sign creates an HMAC-SHA256 signature.
func (v *JWTValidator) sign(data []byte) []byte {
	h := hmac.New(sha256.New, v.secret)
	h.Write(data)
	return h.Sum(nil)
}

// base64URLEncode encodes data using base64 URL encoding without padding.
func base64URLEncode(data []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}

// base64URLDecode decodes base64 URL encoded data.
func base64URLDecode(s string) ([]byte, error) {
	// Add padding if necessary
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}

// HasRole checks if the user has a specific role.
func (c *UserClaims) HasRole(role string) bool {
	for _, r := range c.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// IsAdmin checks if the user has the admin role.
func (c *UserClaims) IsAdmin() bool {
	return c.HasRole("admin")
}
