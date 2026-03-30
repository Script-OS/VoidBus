package internal

import (
	"testing"
)

func TestChallengeVerifier_CreateExpectedResponse(t *testing.T) {
	secret := []byte("test-secret")
	verifier := NewChallengeVerifier(secret)

	challenge := []byte("challenge-data")
	response := verifier.CreateExpectedResponse(challenge)

	// Response should be SHA256 HMAC (32 bytes)
	if len(response) != 32 {
		t.Errorf("CreateExpectedResponse() length = %d, want 32", len(response))
	}

	// Same challenge + secret should produce same response
	response2 := verifier.CreateExpectedResponse(challenge)
	if string(response) != string(response2) {
		t.Error("CreateExpectedResponse() should be deterministic")
	}

	// Different challenge should produce different response
	challenge2 := []byte("different-challenge")
	response3 := verifier.CreateExpectedResponse(challenge2)
	if string(response) == string(response3) {
		t.Error("CreateExpectedResponse() should produce different responses for different challenges")
	}
}

func TestChallengeVerifier_VerifyChallenge(t *testing.T) {
	secret := []byte("test-secret")
	verifier := NewChallengeVerifier(secret)

	challenge := []byte("challenge-data")
	response := verifier.CreateExpectedResponse(challenge)

	// Correct response should verify
	if !verifier.VerifyChallenge(challenge, response) {
		t.Error("VerifyChallenge() should return true for correct response")
	}

	// Wrong response should fail
	wrongResponse := []byte("wrong-response")
	if verifier.VerifyChallenge(challenge, wrongResponse) {
		t.Error("VerifyChallenge() should return false for wrong response")
	}

	// Different secret should fail
	verifier2 := NewChallengeVerifier([]byte("different-secret"))
	if verifier2.VerifyChallenge(challenge, response) {
		t.Error("VerifyChallenge() should return false with different secret")
	}
}

func TestSimpleChallengeHandler_CreateChallenge(t *testing.T) {
	handler := NewSimpleChallengeHandler()

	challenge, err := handler.CreateChallenge()
	if err != nil {
		t.Errorf("CreateChallenge() error = %v", err)
		return
	}

	// Challenge should be 32 bytes
	if len(challenge) != 32 {
		t.Errorf("CreateChallenge() length = %d, want 32", len(challenge))
	}

	// Challenges should be unique
	challenge2, _ := handler.CreateChallenge()
	if string(challenge) == string(challenge2) {
		t.Error("CreateChallenge() should return unique challenges")
	}
}

func TestSimpleChallengeHandler_CreateResponse(t *testing.T) {
	handler := NewSimpleChallengeHandler()

	challenge := []byte("challenge-data")
	response := handler.CreateResponse(challenge)

	// Response should be SHA256 hash (32 bytes)
	if len(response) != 32 {
		t.Errorf("CreateResponse() length = %d, want 32", len(response))
	}

	// Same challenge should produce same response
	response2 := handler.CreateResponse(challenge)
	if string(response) != string(response2) {
		t.Error("CreateResponse() should be deterministic")
	}

	// Different challenge should produce different response
	challenge2 := []byte("different-challenge")
	response3 := handler.CreateResponse(challenge2)
	if string(response) == string(response3) {
		t.Error("CreateResponse() should produce different responses for different challenges")
	}
}

func TestSimpleChallengeHandler_VerifyResponse(t *testing.T) {
	handler := NewSimpleChallengeHandler()

	challenge := []byte("challenge-data")
	response := handler.CreateResponse(challenge)

	// Correct response should verify
	if !handler.VerifyResponse(challenge, response) {
		t.Error("VerifyResponse() should return true for correct response")
	}

	// Wrong response should fail
	wrongResponse := []byte("wrong-response")
	if handler.VerifyResponse(challenge, wrongResponse) {
		t.Error("VerifyResponse() should return false for wrong response")
	}

	// Different challenge should fail
	challenge2 := []byte("different-challenge")
	if handler.VerifyResponse(challenge2, response) {
		t.Error("VerifyResponse() should return false with different challenge")
	}
}

func TestChallengeVerifier_WithDifferentSecrets(t *testing.T) {
	secrets := [][]byte{
		[]byte("secret1"),
		[]byte("secret2"),
		[]byte("much-longer-secret-key-for-testing"),
		make([]byte, 64), // Empty secret
	}

	challenge := []byte("test-challenge")

	for i, secret := range secrets {
		verifier := NewChallengeVerifier(secret)
		response := verifier.CreateExpectedResponse(challenge)

		if !verifier.VerifyChallenge(challenge, response) {
			t.Errorf("Test %d: verifier should verify its own response", i)
		}

		// Other verifiers should not verify this response
		for j, otherSecret := range secrets {
			if i == j {
				continue
			}
			otherVerifier := NewChallengeVerifier(otherSecret)
			if otherVerifier.VerifyChallenge(challenge, response) {
				t.Errorf("Test %d vs %d: different secrets should produce different responses", i, j)
			}
		}
	}
}
