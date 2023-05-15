package server

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
)

// CharSet groups the possible output of the function RandStr. For the possible values
// see the constants
type CharSet int

const (
	NUM               CharSet = iota // Digits from 0 to 9
	ALPHA                            // Latin letters from A to z (Uppercase and Lowercase)
	ALPHA_LOW                        // Latin letters from a to z (Lowercase)
	ALPHA_NUM                        // Combination of NUM and ALPHA
	ALPHA_LOW_NUM                    // Combination of NUM and ALPHA_LOW
	ALPHA_NUM_SPECIAL                // Combines ALPHA_LOW with this special character: !?+*-_=.&%$€#@
)

const (
	num       = "0123456789"
	alpha     = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	alpha_low = "abcdefghijklmnopqrstuvwxyz"
	special   = "!?+*-_=.&%$€#@"
)

// RandStr generates a random string with the given length. The string can be
// made of differente sets of characters: see CharSet type
func RandStr(length int, randType CharSet) string {
	var dictionary string

	switch randType {
	case NUM:
		dictionary = num
	case ALPHA:
		dictionary = alpha + alpha_low
	case ALPHA_LOW:
		dictionary = alpha_low
	case ALPHA_NUM:
		dictionary = num + alpha + alpha_low
	case ALPHA_LOW_NUM:
		dictionary = num + alpha_low
	case ALPHA_NUM_SPECIAL:
		dictionary = num + alpha + alpha_low + special
	default:
		return ""
	}

	res := make([]byte, length)
	for i := 0; i < length; i++ {
		r, err := rand.Int(rand.Reader, big.NewInt(int64(len(dictionary))))
		if err != nil {
			panic(err)
		}

		if !r.IsInt64() {
			panic(errors.New("random number generated cannot be used as an int64"))
		}

		res[i] = dictionary[r.Int64()]
	}
	return string(res)
}

// GenerateHashString generate a hash with sha256 from data
func GenerateHashString(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func isAbs(path string) bool {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = strings.Replace(path, "~", home, 1)
		}
	}

	return filepath.IsAbs(path)
}
