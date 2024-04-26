package middleware

import (
	"crypto/sha256"
	"fmt"
	"net/http"

	"github.com/gorilla/securecookie"
)

type CookieManager struct {
	secureCookie     *securecookie.SecureCookie
	secureCookiePerm *securecookie.SecureCookie
}

func NewCookieManager(hashKey []byte, blockKey []byte) (*CookieManager, error) {
	cm := new(CookieManager)

	hashKeyRand := securecookie.GenerateRandomKey(64)
	if hashKeyRand == nil {
		return nil, fmt.Errorf("error creating random hashKey")
	}
	blockKeyRand := securecookie.GenerateRandomKey(32)
	if blockKeyRand == nil {
		return nil, fmt.Errorf("error creating random blockKey")
	}
	cm.secureCookie = securecookie.New(hashKeyRand, blockKeyRand).MaxAge(0)

	hashKeyPerm := make([]byte, 0, 32)
	for _, b := range sha256.Sum256(hashKey) {
		hashKeyPerm = append(hashKeyPerm, b)
	}
	blockKeyPerm := make([]byte, 0, 32)
	for _, b := range sha256.Sum256(blockKey) {
		blockKeyPerm = append(blockKeyPerm, b)
	}
	cm.secureCookiePerm = securecookie.New(hashKeyPerm, blockKeyPerm).MaxAge(0)

	return cm, nil
}

type cookie_ctx_key_t string

const cookie_ctx_key cookie_ctx_key_t = "github.com/nixpare/server/v3/middlewares/cookies.CookieManager"

func GetCookieManager(r *http.Request) *CookieManager {
	a := r.Context().Value(cookie_ctx_key)
	if a == nil {
		return nil
	}

	cm, ok := a.(*CookieManager)
	if ok {
		return nil
	}
	return cm
}
