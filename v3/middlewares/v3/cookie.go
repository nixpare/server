package middlewares

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/securecookie"
	"github.com/nixpare/server/v3"
)

type CookieCtxKeyT string

const COOKIE_CTX_KEY CookieCtxKeyT = "nix-cookie-middleware"

var (
	HashKeyString  = "NixPare Server"
	BlockKeyString = "github.com/nixpare/server"
)

func RegisterCookieManager() server.MiddlewareFunc {
	cm, err := NewCookieManager()
	if err != nil {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			*r = *r.WithContext(context.WithValue(r.Context(), COOKIE_CTX_KEY, cm))
			next.ServeHTTP(w, r)
		})
	}
}

func GetCookieManager(r *http.Request) *CookieManager {
	return r.Context().Value(COOKIE_CTX_KEY).(*CookieManager)
}

type CookieManager struct {
	secureCookie     *securecookie.SecureCookie
	secureCookiePerm *securecookie.SecureCookie
}

func NewCookieManager() (*CookieManager, error) {
	cm := new(CookieManager)

	hashKey := securecookie.GenerateRandomKey(64)
	if hashKey == nil {
		return nil, fmt.Errorf("error creating hashKey")
	}
	blockKey := securecookie.GenerateRandomKey(32)
	if blockKey == nil {
		return nil, fmt.Errorf("error creating blockKey")
	}
	cm.secureCookie = securecookie.New(hashKey, blockKey).MaxAge(0)

	hashKeyPerm := make([]byte, 0, 32)
	for _, b := range sha256.Sum256([]byte(HashKeyString)) {
		hashKeyPerm = append(hashKeyPerm, b)
	}
	blockKeyPerm := make([]byte, 0, 32)
	for _, b := range sha256.Sum256([]byte(BlockKeyString)) {
		blockKeyPerm = append(blockKeyPerm, b)
	}
	cm.secureCookiePerm = securecookie.New(hashKeyPerm, blockKeyPerm).MaxAge(0)

	return cm, nil
}

type CookieOption func(cookie *http.Cookie)

func CookiePathOpt(value string) CookieOption {
	return func(cookie *http.Cookie) {
		cookie.Path = value
	}
}

func CookieDomainOpt(value string) CookieOption {
	return func(cookie *http.Cookie) {
		cookie.Domain = value
	}
}

func CookieExpiresOpt(value time.Time) CookieOption {
	return func(cookie *http.Cookie) {
		cookie.Expires = value
	}
}

func CookieSecureOpt(value bool) CookieOption {
	return func(cookie *http.Cookie) {
		cookie.Secure = value
	}
}

func CookieHTTPOnlyOpt(value bool) CookieOption {
	return func(cookie *http.Cookie) {
		cookie.HttpOnly = value
	}
}

func CookieSameSiteOpt(value http.SameSite) CookieOption {
	return func(cookie *http.Cookie) {
		cookie.SameSite = value
	}
}

// SetCookie creates a new cookie with the given name and value, maxAge can be used
// to sex the expiration date:
//   - maxAge = 0 means no expiration specified
//   - maxAge > 0 sets the expiration date from the current date adding the given time in seconds
//     (- maxAge < 0 will remove the cookie instantly, like route.DeleteCookie)
//
// The cookie value is encoded and encrypted using a pair of keys created randomly at server creation,
// so if the same cookie is later decoded between server restart, it can't be decoded. To have such a
// behaviour see SetCookiePerm.
//
// The encoding of the value is managed by the package encoding/gob. If you are just encoding and decoding
// plain structs and each field type is a primary type or a struct (with the same rules), nothing should be
// done, but if you are dealing with interfaces, you must first register every concrete structure or type
// implementing that interface before encoding or decoding
func (cm *CookieManager) SetCookie(w http.ResponseWriter, name string, value any, maxAge int, opts ...CookieOption) error {
	encValue, err := cm.secureCookie.Encode(name, value)
	if err != nil {
		return err
	}

	cookie := &http.Cookie{
		Name:     server.GenerateHashString([]byte(name)),
		Value:    encValue,
		MaxAge:   maxAge,
		HttpOnly: true,
	}

	for _, opt := range opts {
		opt(cookie)
	}

	http.SetCookie(w, cookie)
	return nil
}

// DeleteCookie instantly removes a cookie with the given name before set with route.SetCookie
// or route.SetCookiePerm
func (cm *CookieManager) DeleteCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:   server.GenerateHashString([]byte(name)),
		MaxAge: -1,
	})
}

// DecodeCookie decodes a previously set cookie with the given name
// using the method route.SetCookie.
//
// If the cookie was not found, it will return false and the relative error
// (probably an http.ErrNoCookie), otherwise it will return true and, possibly,
// the decode error. It happends when:
//   - the server was restarted, so the keys used for decoding are different
//   - you provided the wrong value type
//   - the cookie was not set by the server
//
// The argument value must be a pointer, otherwise the value will not
// be returned. A workaround might be using the type parametric
// function server.DecodeCookie
func (cm *CookieManager) Cookie(r *http.Request, name string, value any) error {
	cookie, err := r.Cookie(server.GenerateHashString([]byte(name)))
	if err != nil {
		return err
	}

	return cm.secureCookie.Decode(name, cookie.Value, value)
}

// SetCookiePerm creates a new cookie with the given name and value, maxAge can be used
// to sex the expiration date:
//   - maxAge = 0 means no expiration specified
//   - maxAge > 0 sets the expiration date from the current date adding the given time in seconds
//     (- maxAge < 0 will remove the cookie instantly, like route.DeleteCookie)
//
// The cookie value is encoded and encrypted using a pair of keys at package level that MUST be set at
// program startup. This differs for the method route.SetCookie to ensure that even after server restart
// these cookies can still be decoded.
func (cm *CookieManager) SetCookiePerm(w http.ResponseWriter, name string, value any, maxAge int, opts ...CookieOption) error {
	encValue, err := cm.secureCookiePerm.Encode(name, value)
	if err != nil {
		return err
	}

	cookie := &http.Cookie{
		Name:     server.GenerateHashString([]byte(name)),
		Value:    encValue,
		MaxAge:   maxAge,
		HttpOnly: true,
	}

	for _, opt := range opts {
		opt(cookie)
	}

	http.SetCookie(w, cookie)
	return nil
}

// DecodeCookiePerm decodes a previously set cookie with the given name
// using the method route.SetCookiePerm.
//
// If the cookie was not found, it will return false and the relative error
// (probably an http.ErrNoCookie), otherwise it will return true and, possibly,
// the decode error. It happends when:
//   - you provided the wrong value type
//   - the cookie was not set by the server
//
// The argument value must be a pointer, otherwise the value will not
// be returned. A workaround might be using the type parametric
// function server.DecodeCookiePerm
func (cm *CookieManager) CookiePerm(r *http.Request, name string, value any) error {
	cookie, err := r.Cookie(server.GenerateHashString([]byte(name)))
	if err != nil {
		return err
	}

	return cm.secureCookiePerm.Decode(name, cookie.Value, value)
}
