package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"time"
)

// Hardcoded keys from application.js
var EncKeys = []struct {
	Name string
	Key  string
}{
	{"malfunctioning-unapproachability", "Em9k8X2SsEDHbC6mF9jwBug8BGfLYC2TR97hzKzCaAY="},
	{"tegular-peripatopsidae", "eOSPDQfRMp+RwOKE4v7TQc5yGgeg2ABQ23pjWg8kWAg="},
	{"elective-experience", "Wh7toVpICwu53zFH7+1PagoveuCK6uquyVfr8TSIwQw="},
	{"heraldic-epacris", "QnyTODU7KLY9taRt7V2sNyRflu97U3LYmnx4uhCsLDM="},
}

func GetRandomKey() (string, []byte, error) {
	// Pick first for simplicity, or random in full impl
	k := EncKeys[0]
	keyBytes, err := base64.StdEncoding.DecodeString(k.Key)
	return k.Name, keyBytes, err
}

func GetKeyByName(name string) ([]byte, error) {
	for _, k := range EncKeys {
		if k.Name == name {
			return base64.StdEncoding.DecodeString(k.Key)
		}
	}
	return nil, errors.New("key not found")
}

// Encrypt encrypts plainText using AES-GCM
func EncryptMessage(plainText string) (EncryptedData, error) {
	start := time.Now()
	keyName, keyBytes, err := GetRandomKey()
	if err != nil {
		return EncryptedData{}, err
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return EncryptedData{}, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return EncryptedData{}, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return EncryptedData{}, err
	}

	cipherText := gcm.Seal(nil, nonce, []byte(plainText), nil)
	fmt.Println("Encryption took:", time.Since(start))
	return EncryptedData{
		KeyName: keyName,
		Data:    base64.StdEncoding.EncodeToString(cipherText),
		IV:      base64.StdEncoding.EncodeToString(nonce),
	}, nil
}

// Decrypt decrypts base64 ciphertext using the named key and IV
func DecryptMessage(cipherBase64, keyName, ivBase64 string) (string, error) {
	keyBytes, err := GetKeyByName(keyName)
	if err != nil {
		return "", err
	}

	cipherBytes, err := base64.StdEncoding.DecodeString(cipherBase64)
	if err != nil {
		return "", err
	}

	nonce, err := base64.StdEncoding.DecodeString(ivBase64)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	plainBytes, err := gcm.Open(nil, nonce, cipherBytes, nil)
	if err != nil {
		return "", err
	}

	return string(plainBytes), nil
}
