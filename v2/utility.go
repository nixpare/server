package server

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"
	"runtime/debug"
	"strings"
)

// ParseCommandArgs gets a list of strings and parses their content
// splitting them into separated strings. The characters used to parse
// the commands are, in the relevant order, <'>, <"> and < >
func ParseCommandArgs(args ...string) []string {
	a := make([]string, 0)
	for _, s := range args {
		for i, s1 := range strings.Split(s, "'") {
			if i%2 == 1 {
				a = append(a, s1)
				continue
			}

			for j, s2 := range strings.Split(s1, "\"") {
				if j%2 == 1 {
					a = append(a, s2)
					continue
				}

				for _, s3 := range strings.Split(s2, " ") {
					if s3 != "" {
						a = append(a, s3)
					}
				}
			}
		}
	}

	return a
}

// CharSet groups the possible output of the function RandStr. For the possible values
// see the constants
type CharSet int

const (
	NUM CharSet = iota 	// Digits from 0 to 9
	ALPHA 				// Latin letters from A to z (Uppercase and Lowercase)
	ALPHA_LOW 			// Latin letters from a to z (Lowercase)
	ALPHA_NUM 			// Combination of NUM and ALPHA
	ALPHA_LOW_NUM 		// Combination of NUM and ALPHA_LOW
	ALPHA_NUM_SPECIAL 	// Combines ALPHA_LOW with this special character: !?+*-_=.&%$€#@
)

const (
	num = "0123456789"
	alpha = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	alpha_low = "abcdefghijklmnopqrstuvwxyz"
	special = "!?+*-_=.&%$€#@"
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

// ChanIsOpened tells wether the channel is opened or closed
func ChanIsOpened(c chan any) bool {
	if c == nil {
		return false
	}

	open := true

	select {
	case _, open = <- c:
	default:
	}

	return open
}

// PanicError is the data used to capture any function returning an error
// and any generated panic. If Err is set, means that the function returned
// an error without panicking, instead if PanicErr is set, this means that
// the function returned prematurely due to a panic: the panic error is set
// and the stack trace is provided in the relative field
type PanicError struct {
	Err 	 error
	PanicErr error
	Stack 	 string
}

// PanicToErr runs any function that returns an error in a panic-controlled
// environment and converts any panic (if any) into an error. Expecially, the
// error generated by a panic can be unwrapped to get rid of the stack trace (
// or manipulated to separate the error from the stack trace)
func PanicToErr(f func() error) *PanicError {
	errChan := make(chan *PanicError)

	go func() {
		defer func() {
			err := recover()
			
			if err == nil {
				errChan <- nil
			} else {
				switch err := err.(type) {
				case error:
					errChan <- &PanicError {
						PanicErr: fmt.Errorf("%w", err),
						Stack: Stack(),
					}
				default:
					errChan <- &PanicError {
						PanicErr: fmt.Errorf("%v", err),
						Stack: Stack(),
					}
				}
			}

			close(errChan)
		}()
		
		if err := f(); err == nil {
			errChan <- nil
		} else {
			errChan <- &PanicError { Err: err }
		}
	}()

	res := <- errChan
	return res
}

// GenerateHashString generate an hash with sha256 from data
func GenerateHashString(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

// Stack returns the execution stack. This must be called after
// a panic (during a recovery) because it strips the first lines
// where are reported also the recovery functions, returning only
// the panic-reletate stuff. If this is not desired just use the
// standard debug.Stack
func Stack() string {
	var out string

	split := strings.Split(string(debug.Stack()), "\n")
	cont := true

	for _, s := range split {
		if strings.HasPrefix(s, "panic(") {
			cont = false
		}

		if cont {
			continue
		}

		out += s + "\n"
	}

	return strings.TrimRight(out, "\n")
}

// IndentString takes a string and indents every line with
// the provided number of single spaces
func IndentString(s string, n int) string {
	split := strings.Split(s, "\n")
	var res string

	for _, line := range split {
		for i := 0; i < n; i++ {
			res += " "
		}
		res += line + "\n"
	}

	return strings.TrimRight(res, " \n")
}
