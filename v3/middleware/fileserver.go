package middleware

import (
	"io/fs"
	"net/http"
)

func FileServerHandler(root fs.FS) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        
    }
}