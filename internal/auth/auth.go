package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Manager handles OTP generation, token creation, and token validation.
type Manager struct {
	secret  string
	otps    map[string]otpEntry
	smtpCfg SMTPConfig
}

// SMTPConfig holds SMTP settings for sending OTP emails.
type SMTPConfig struct {
	Host string
	Port string
	User string
	Pass string
	From string
}

type otpEntry struct {
	code      string
	expiresAt time.Time
}

// NewManager creates a new auth manager.
func NewManager(secret string, smtp SMTPConfig) *Manager {
	return &Manager{
		secret:  secret,
		otps:    make(map[string]otpEntry),
		smtpCfg: smtp,
	}
}

// GenerateOTP creates a 6-digit OTP for the given email and stores it.
// If no SMTP is configured, the OTP is returned for logging to stderr.
func (m *Manager) GenerateOTP(email string) (string, error) {
	code := generateOTPCode()
	m.otps[email] = otpEntry{
		code:      code,
		expiresAt: time.Now().Add(10 * time.Minute),
	}

	// If SMTP is configured, send email. Otherwise return code for logging.
	if m.smtpCfg.Host != "" {
		if err := sendEmail(m.smtpCfg, email, "stashkit OTP", fmt.Sprintf("Your OTP code is: %s", code)); err != nil {
			return "", fmt.Errorf("failed to send OTP email: %w", err)
		}
	}

	return code, nil
}

// VerifyOTP checks if the provided OTP code is valid for the given email.
func (m *Manager) VerifyOTP(email, code string) bool {
	entry, ok := m.otps[email]
	if !ok {
		return false
	}
	if time.Now().After(entry.expiresAt) {
		delete(m.otps, email)
		return false
	}
	if entry.code != code {
		return false
	}
	delete(m.otps, email)
	return true
}

// GenerateToken creates a signed bearer token for the given email.
func (m *Manager) GenerateToken(email string) (string, error) {
	payload := fmt.Sprintf("%s|%d", email, time.Now().Unix())
	mac := hmac.New(sha256.New, []byte(m.secret))
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	token := base64.URLEncoding.EncodeToString([]byte(payload + "|" + sig))
	return token, nil
}

// ValidateToken checks if a bearer token is valid and returns the email.
func (m *Manager) ValidateToken(token string) (string, error) {
	decoded, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return "", fmt.Errorf("invalid token format")
	}

	parts := strings.SplitN(string(decoded), "|", 3)
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid token structure")
	}

	email := parts[0]
	payload := parts[0] + "|" + parts[1]
	sig := parts[2]

	mac := hmac.New(sha256.New, []byte(m.secret))
	mac.Write([]byte(payload))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return "", fmt.Errorf("invalid token signature")
	}

	return email, nil
}

// IsSMTPConfigured returns true if SMTP settings are set.
func (m *Manager) IsSMTPConfigured() bool {
	return m.smtpCfg.Host != ""
}

// generateOTPCode creates a random 6-digit code.
func generateOTPCode() string {
	b := make([]byte, 4)
	rand.Read(b)
	code := int(b[0])<<24 | int(b[1])<<16 | int(b[2])<<8 | int(b[3])
	if code < 0 {
		code = -code
	}
	return fmt.Sprintf("%06d", code%1000000)
}

// sendEmail sends an email via SMTP. This is a stub that returns nil
// when SMTP is not configured. In production, use net/smtp.
func sendEmail(cfg SMTPConfig, to, subject, body string) error {
	// Real SMTP sending would go here using net/smtp.
	// For now, this is a placeholder that will be filled in when SMTP is configured.
	_ = cfg
	_ = to
	_ = subject
	_ = body
	return nil
}
