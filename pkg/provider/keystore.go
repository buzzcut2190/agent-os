package provider

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

// KeyStore securely stores API keys on disk (masked on read).
type KeyStore struct {
	mu      sync.RWMutex
	path    string
	keys    map[string]string // provider -> key (plaintext in memory)
	encKey  []byte            // AES-256 key for on-disk encryption (optional)
}

// NewKeyStore creates a key store at the given path. If encKey is non-nil,
// keys are encrypted with AES-GCM before writing to disk.
func NewKeyStore(path string, encKey []byte) *KeyStore {
	ks := &KeyStore{
		path:   path,
		keys:   make(map[string]string),
		encKey: encKey,
	}
	ks.load()
	return ks
}

// Set stores an API key for a provider.
func (k *KeyStore) Set(provider, key string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.keys[provider] = key
	return k.save()
}

// Get returns the plaintext API key for a provider.
func (k *KeyStore) Get(provider string) (string, bool) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	key, ok := k.keys[provider]
	return key, ok
}

// Delete removes an API key.
func (k *KeyStore) Delete(provider string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.keys, provider)
	return k.save()
}

// List returns masked key info for all providers.
func (k *KeyStore) List() []ProviderKeyInfo {
	k.mu.RLock()
	defer k.mu.RUnlock()
	var result []ProviderKeyInfo
	for provider, key := range k.keys {
		result = append(result, ProviderKeyInfo{
			Provider: provider,
			Masked:   Mask(key),
		})
	}
	return result
}

// Mask returns a masked version of a key (first 8 + "..." + last 4 chars).
func Mask(key string) string {
	if len(key) <= 12 {
		return "***"
	}
	return key[:8] + "..." + key[len(key)-4:]
}

// load reads keys from the JSON file (decrypting if needed).
func (k *KeyStore) load() {
	data, err := os.ReadFile(k.path)
	if err != nil {
		return
	}
	if len(k.encKey) > 0 {
		plain, err := decrypt(data, k.encKey)
		if err != nil {
			return
		}
		data = plain
	}
	var keys map[string]string
	if err := json.Unmarshal(data, &keys); err != nil {
		return
	}
	k.keys = keys
}

// save writes keys to the JSON file (encrypting if needed). File mode 0600.
func (k *KeyStore) save() error {
	data, err := json.Marshal(k.keys)
	if err != nil {
		return err
	}
	if len(k.encKey) > 0 {
		data, err = encrypt(data, k.encKey)
		if err != nil {
			return err
		}
	}
	if err := os.MkdirAll(dirOf(k.path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(k.path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	if err != nil {
		return err
	}
	return f.Chmod(0600)
}

// encrypt uses AES-256-GCM.
func encrypt(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes new cipher: %w", err)
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}
	// nonce + ciphertext + tag
	return aesGCM.Seal(nonce, nonce, plaintext, nil), nil
}

// decrypt reverses AES-256-GCM encryption.
func decrypt(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes new cipher: %w", err)
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return aesGCM.Open(nil, nonce, ciphertext, nil)
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}

// DeriveKey derives a 32-byte AES key from a passphrase using a simple hash.
func DeriveKey(passphrase string) []byte {
	key := make([]byte, 32)
	copy(key, []byte(passphrase))
	return key
}

// Raw base64 encode/decode for key transport.
func encodeB64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func decodeB64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}
