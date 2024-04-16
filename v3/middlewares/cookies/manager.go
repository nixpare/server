package cookie

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"

	"github.com/gorilla/securecookie"
	"github.com/nixpare/server/v3"
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

func (cm *CookieManager) Register() server.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			*r = *r.WithContext(context.WithValue(r.Context(), cookie_ctx_key, cm))
			next.ServeHTTP(w, r)
		})
	}
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
