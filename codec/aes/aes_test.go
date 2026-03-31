package aes

import (
	"crypto/rand"
	"testing"

	"github.com/Script-OS/VoidBus/codec"
	"github.com/Script-OS/VoidBus/keyprovider"
	"github.com/Script-OS/VoidBus/keyprovider/embedded"
)

// Helper to create valid key provider
func createKeyProvider(keySize int) keyprovider.KeyProvider {
	key := make([]byte, keySize)
	rand.Read(key)
	kp, _ := embedded.New(key, "", "")
	return kp
}

func TestAESCodec_EncodeDecode_AES128(t *testing.T) {
	c := NewAES128Codec()
	kp := createKeyProvider(KeySize128)

	err := c.SetKeyProvider(kp)
	if err != nil {
		t.Fatalf("SetKeyProvider() error = %v", err)
	}

	testData := [][]byte{
		[]byte("Hello, World!"),
		[]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05},
		make([]byte, 256),
		make([]byte, 1024),
	}

	for i, data := range testData {
		encoded, err := c.Encode(data)
		if err != nil {
			t.Errorf("Test %d: Encode() error = %v", i, err)
			continue
		}

		// Verify output length: nonce(12) + ciphertext + tag(16)
		expectedMinLen := NonceSize + len(data) + TagSize
		if len(encoded) < expectedMinLen {
			t.Errorf("Test %d: encoded length = %d, want >= %d", i, len(encoded), expectedMinLen)
		}

		decoded, err := c.Decode(encoded)
		if err != nil {
			t.Errorf("Test %d: Decode() error = %v", i, err)
			continue
		}

		if string(decoded) != string(data) {
			t.Errorf("Test %d: roundtrip failed", i)
		}
	}
}

func TestAESCodec_EncodeDecode_AES256(t *testing.T) {
	c := NewAES256Codec()
	kp := createKeyProvider(KeySize256)

	err := c.SetKeyProvider(kp)
	if err != nil {
		t.Fatalf("SetKeyProvider() error = %v", err)
	}

	testData := [][]byte{
		[]byte("Hello, World!"),
		[]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05},
		make([]byte, 256),
		make([]byte, 1024),
	}

	for i, data := range testData {
		encoded, err := c.Encode(data)
		if err != nil {
			t.Errorf("Test %d: Encode() error = %v", i, err)
			continue
		}

		decoded, err := c.Decode(encoded)
		if err != nil {
			t.Errorf("Test %d: Decode() error = %v", i, err)
			continue
		}

		if string(decoded) != string(data) {
			t.Errorf("Test %d: roundtrip failed", i)
		}
	}
}

func TestAESCodec_Encode_EmptyData(t *testing.T) {
	c := NewAES128Codec()
	kp := createKeyProvider(KeySize128)
	c.SetKeyProvider(kp)

	_, err := c.Encode([]byte{})
	if err == nil {
		t.Error("Encode() should return error for empty data")
	}
}

func TestAESCodec_Encode_NoKeyProvider(t *testing.T) {
	c := NewAES128Codec()

	_, err := c.Encode([]byte("test"))
	if err != ErrKeyNotSet {
		t.Errorf("Encode() should return ErrKeyNotSet, got %v", err)
	}
}

func TestAESCodec_Decode_CiphertextTooShort(t *testing.T) {
	c := NewAES128Codec()
	kp := createKeyProvider(KeySize128)
	c.SetKeyProvider(kp)

	shortData := []byte{0x00, 0x01, 0x02} // Less than nonce + tag size
	_, err := c.Decode(shortData)
	if err != ErrCiphertextTooShort {
		t.Errorf("Decode() should return ErrCiphertextTooShort, got %v", err)
	}
}

func TestAESCodec_SetKeyProvider_InvalidKeySize(t *testing.T) {
	c := NewAES128Codec() // Requires 16-byte key

	// 15-byte key (wrong size)
	wrongKey := make([]byte, 15)
	kp, _ := embedded.New(wrongKey, "", "")

	err := c.SetKeyProvider(kp)
	if err != ErrInvalidKeySize {
		t.Errorf("SetKeyProvider() should return ErrInvalidKeySize, got %v", err)
	}

	// 32-byte key (wrong size for AES-128)
	key32 := make([]byte, 32)
	kp32, _ := embedded.New(key32, "", "")

	err = c.SetKeyProvider(kp32)
	if err != ErrInvalidKeySize {
		t.Errorf("SetKeyProvider() should return ErrInvalidKeySize for 32-byte key on AES-128, got %v", err)
	}
}

func TestAESCodec_SetKeyProvider_Nil(t *testing.T) {
	c := NewAES128Codec()

	err := c.SetKeyProvider(nil)
	if err != codec.ErrInvalidKeyProvider {
		t.Errorf("SetKeyProvider(nil) should return ErrInvalidKeyProvider, got %v", err)
	}
}

func TestAESCodec_DifferentKeys(t *testing.T) {
	c := NewAES128Codec()

	key1 := make([]byte, KeySize128)
	key1[0] = 0x01
	kp1, _ := embedded.New(key1, "", "")

	key2 := make([]byte, KeySize128)
	key2[0] = 0x02
	kp2, _ := embedded.New(key2, "", "")

	// Encode with key1
	c.SetKeyProvider(kp1)
	data := []byte("test message")
	encoded, err := c.Encode(data)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	// Try to decode with key2 (should fail)
	c.SetKeyProvider(kp2)
	_, err = c.Decode(encoded)
	if err == nil {
		t.Error("Decode() should fail with different key")
	}
}

func TestAESCodec_NonceUniqueness(t *testing.T) {
	c := NewAES128Codec()
	kp := createKeyProvider(KeySize128)
	c.SetKeyProvider(kp)

	data := []byte("test")

	// Encode same data twice
	encoded1, _ := c.Encode(data)
	encoded2, _ := c.Encode(data)

	// Extract nonces (first 12 bytes)
	nonce1 := encoded1[:NonceSize]
	nonce2 := encoded2[:NonceSize]

	// Nonces should be different
	if string(nonce1) == string(nonce2) {
		t.Error("Nonces should be unique for each encryption")
	}

	// Ciphertexts should be different (due to different nonce)
	ciphertext1 := encoded1[NonceSize:]
	ciphertext2 := encoded2[NonceSize:]
	if string(ciphertext1) == string(ciphertext2) {
		t.Error("Ciphertexts should differ due to unique nonce")
	}
}

func TestAESCodec_InternalID(t *testing.T) {
	c128 := NewAES128Codec()
	if c128.InternalID() != InternalID128 {
		t.Errorf("AES-128 InternalID() = %s, want %s", c128.InternalID(), InternalID128)
	}

	c256 := NewAES256Codec()
	if c256.InternalID() != InternalID256 {
		t.Errorf("AES-256 InternalID() = %s, want %s", c256.InternalID(), InternalID256)
	}
}

func TestAESCodec_SecurityLevel(t *testing.T) {
	c128 := NewAES128Codec()
	if c128.SecurityLevel() != codec.SecurityLevelMedium {
		t.Errorf("AES-128 SecurityLevel() = %d, want Medium(%d)", c128.SecurityLevel(), codec.SecurityLevelMedium)
	}

	c256 := NewAES256Codec()
	if c256.SecurityLevel() != codec.SecurityLevelHigh {
		t.Errorf("AES-256 SecurityLevel() = %d, want High(%d)", c256.SecurityLevel(), codec.SecurityLevelHigh)
	}
}

func TestAESCodec_RequiresKey(t *testing.T) {
	c := NewAES128Codec()
	if !c.RequiresKey() {
		t.Error("RequiresKey() should return true for AES codec")
	}
}

func TestAESCodec_KeyAlgorithm(t *testing.T) {
	c128 := NewAES128Codec()
	if c128.KeyAlgorithm() != Algorithm128 {
		t.Errorf("AES-128 KeyAlgorithm() = %s, want %s", c128.KeyAlgorithm(), Algorithm128)
	}

	c256 := NewAES256Codec()
	if c256.KeyAlgorithm() != Algorithm256 {
		t.Errorf("AES-256 KeyAlgorithm() = %s, want %s", c256.KeyAlgorithm(), Algorithm256)
	}
}

func TestModule_Create(t *testing.T) {
	m128 := NewAES128Module()
	codec128, err := m128.Create(nil)
	if err != nil {
		t.Errorf("AES128Module.Create() error = %v", err)
	}
	if codec128.InternalID() != InternalID128 {
		t.Errorf("Created codec InternalID() = %s, want %s", codec128.InternalID(), InternalID128)
	}

	m256 := NewAES256Module()
	codec256, err := m256.Create(nil)
	if err != nil {
		t.Errorf("AES256Module.Create() error = %v", err)
	}
	if codec256.InternalID() != InternalID256 {
		t.Errorf("Created codec InternalID() = %s, want %s", codec256.InternalID(), InternalID256)
	}
}

func TestModule_SecurityLevel(t *testing.T) {
	m128 := NewAES128Module()
	if m128.SecurityLevel() != codec.SecurityLevelMedium {
		t.Errorf("AES128Module SecurityLevel() = %d, want Medium", m128.SecurityLevel())
	}

	m256 := NewAES256Module()
	if m256.SecurityLevel() != codec.SecurityLevelHigh {
		t.Errorf("AES256Module SecurityLevel() = %d, want High", m256.SecurityLevel())
	}
}
