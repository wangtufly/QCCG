package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"strings"
	"time"
)

const cosyVersion = "0.2.17"

const serverPubKeyPEM = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDA8iMH5c02LilrsERw9t6Pv5Nc
4k6Pz1EaDicBMpdpxKduSZu5OANqUq8er4GM95omAGIOPOh+Nx0spthYA2BqGz+l
6HRkPJ7S236FZz73In/KVuLnwI8JJ2CbuJap8kvheCCZpmAWpb/cPx/3Vr/J6I17
XcW+ML9FoCI6AOvOzwIDAQAB
-----END PUBLIC KEY-----`

type authIdentity struct {
	Name               string
	Aid                string
	Uid                string
	YxUid              string
	OrganizationId     string
	OrganizationName   string
	UserType           string
	SecurityOauthToken string
	RefreshToken       string
}

type sessionContext struct {
	TempKey      []byte
	CosyKey      string
	Info         string
	Identity     authIdentity
	MachineId    string
	MachineToken string
	MachineType  string
}

func newSession(id authIdentity, machineId, machineToken, machineType string) (*sessionContext, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	// use hex-like 16 ASCII bytes as temp key (matches Java uuid-hex substr logic)
	tempKey := []byte(fmt.Sprintf("%x", raw)[:16])

	cosyKeyBytes, err := rsaEncrypt(tempKey)
	if err != nil {
		return nil, err
	}
	cosyKey := base64.StdEncoding.EncodeToString(cosyKeyBytes)

	payloadBytes, err := authPayloadJSON(id)
	if err != nil {
		return nil, err
	}
	encPayload, err := aesEncrypt(payloadBytes, tempKey)
	if err != nil {
		return nil, err
	}
	info := base64.StdEncoding.EncodeToString(encPayload)

	return &sessionContext{
		TempKey:      tempKey,
		CosyKey:      cosyKey,
		Info:         info,
		Identity:     id,
		MachineId:    machineId,
		MachineToken: machineToken,
		MachineType:  machineType,
	}, nil
}

func signRequest(payloadB64, cosyKey, cosyDate, body, pathWithoutAlgo string) string {
	s := payloadB64 + "\n" + cosyKey + "\n" + cosyDate + "\n" + body + "\n" + pathWithoutAlgo
	return md5Hex(s)
}

func buildPayloadB64(info string) (string, error) {
	m := map[string]string{
		"cosyVersion": cosyVersion,
		"ideVersion":  "",
		"info":        info,
		"requestId":   newUUID(),
		"version":     "v1",
	}
	data, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func composeBearer(payloadB64, sig string) string {
	return "Bearer COSY." + payloadB64 + "." + sig
}

func authPayloadJSON(id authIdentity) ([]byte, error) {
	m := map[string]string{
		"name":                 id.Name,
		"aid":                  id.Aid,
		"uid":                  id.Uid,
		"yx_uid":               id.YxUid,
		"organization_id":      id.OrganizationId,
		"organization_name":    id.OrganizationName,
		"user_type":            id.UserType,
		"security_oauth_token": id.SecurityOauthToken,
		"refresh_token":        id.RefreshToken,
	}
	return json.Marshal(m)
}

func rsaEncrypt(plaintext []byte) ([]byte, error) {
	pemData := strings.ReplaceAll(serverPubKeyPEM, "\r", "")
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not RSA public key")
	}
	return rsa.EncryptPKCS1v15(rand.Reader, rsaPub, plaintext)
}

func aesEncrypt(plain, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	// PKCS5/PKCS7 padding
	bs := block.BlockSize()
	pad := bs - len(plain)%bs
	padded := make([]byte, len(plain)+pad)
	copy(padded, plain)
	for i := len(plain); i < len(padded); i++ {
		padded[i] = byte(pad)
	}
	out := make([]byte, len(padded))
	iv := key[:bs] // same as Java: IvParameterSpec(key)
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(out, padded)
	return out, nil
}

func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func newRequestId() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)[:24]
}

// newBase64Token mimics Java: base64url(uuid+uuid substring of 50 chars)
func newBase64Token() string {
	u1 := newUUID()
	u2 := newUUID()
	raw := (u1 + u2)[:50]
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// newHexToken returns a random hex string of given length
func newHexToken(n int) string {
	b := make([]byte, (n+1)/2)
	rand.Read(b)
	return fmt.Sprintf("%x", b)[:n]
}

func unixSec() int64 {
	return time.Now().Unix()
}

func unixMs() int64 {
	return time.Now().UnixMilli()
}
