// Package internal provides crypto utilities for VoidBus.
package internal

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
)

// Crypto errors
var (
	ErrChallengeFailed = errors.New("crypto: challenge verification failed")
	ErrInvalidResponse = errors.New("crypto: invalid response")
)

// ChallengeVerifier handles handshake challenge verification.
// Used to verify that a client possesses the expected codec capability.
type ChallengeVerifier struct {
	secret []byte
}

// NewChallengeVerifier creates a new ChallengeVerifier.
func NewChallengeVerifier(secret []byte) *ChallengeVerifier {
	return &ChallengeVerifier{secret: secret}
}

// CreateExpectedResponse creates expected challenge response.
// The response is HMAC-SHA256 of the challenge using the secret.
func (v *ChallengeVerifier) CreateExpectedResponse(challenge []byte) []byte {
	h := hmac.New(sha256.New, v.secret)
	h.Write(challenge)
	return h.Sum(nil)
}

// VerifyChallenge verifies the challenge response.
func (v *ChallengeVerifier) VerifyChallenge(challenge, response []byte) bool {
	expected := v.CreateExpectedResponse(challenge)
	return hmac.Equal(expected, response)
}

// SimpleChallengeHandler provides simple challenge handling.
type SimpleChallengeHandler struct{}

// NewSimpleChallengeHandler creates a new handler.
func NewSimpleChallengeHandler() *SimpleChallengeHandler {
	return &SimpleChallengeHandler{}
}

// CreateChallenge creates a random challenge.
func (h *SimpleChallengeHandler) CreateChallenge() ([]byte, error) {
	return GenerateRandomBytes(32)
}

// CreateResponse creates a simple response (SHA256 of challenge).
// This is a basic implementation. Real implementations should use proper codec encoding.
func (h *SimpleChallengeHandler) CreateResponse(challenge []byte) []byte {
	hash := sha256.Sum256(challenge)
	return hash[:]
}

// VerifyResponse verifies the response.
func (h *SimpleChallengeHandler) VerifyResponse(challenge, response []byte) bool {
	expected := h.CreateResponse(challenge)
	return hmac.Equal(expected, response)
}
