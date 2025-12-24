package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
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

// KeyPair matches the JSON structure from the nomenclator tool.
type KeyPair struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

// LoadKeys reads a JSON file and appends the keys to the EncKeys list.
func LoadKeys(reader io.Reader) error {
	var loadedKeys []KeyPair
	// The file contains a JSON array of pairs
	if err := json.NewDecoder(reader).Decode(&loadedKeys); err != nil {
		return err
	}

	count := 0
	for _, lk := range loadedKeys {
		if lk.Name != "" && lk.Key != "" {
			EncKeys = append(EncKeys, struct {
				Name string
				Key  string
			}{Name: lk.Name, Key: lk.Key})
			count++
		}
	}
	fmt.Printf("Loaded %d additional keys\n", count)
	return nil
}

// LoadKeysFromFile opens the file and calls LoadKeys
func LoadKeysFromFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return LoadKeys(f)
}

func GetRandomKey() (string, []byte, error) {
	if len(EncKeys) == 0 {
		return "", nil, errors.New("no keys available")
	}

	// Generate a cryptographically secure random index
	max := big.NewInt(int64(len(EncKeys)))
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", nil, err
	}

	k := EncKeys[n.Int64()]
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
