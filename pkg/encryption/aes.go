package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
)

// See https://bruinsslot.jp/post/golang-crypto/

func Encrypt(data, password []byte) ([]byte, error) {
	key := deriveKey(password)

	blockCipher, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(blockCipher)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}

	return gcm.Seal(nonce, nonce, data, nil), nil
}

func Decrypt(data, password []byte) ([]byte, error) {
	key := deriveKey(password)

	blockCipher, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(blockCipher)
	if err != nil {
		return nil, err
	}

	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

func deriveKey(password []byte) []byte {
	buf := make([]byte, 32) // Will use AES-256

	copy(buf, sha256.New224().Sum(password)) // Fill the rest of the hash with zeros (SHA-224 leads to a 28 byte long hash)

	return buf
}
