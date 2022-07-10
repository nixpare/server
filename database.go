package server

import (
	//"database/sql"
	//"fmt"

	_ "github.com/go-sql-driver/mysql"
)

const (
	dbUserName = "user"
	dbPassword = "password"
	dbSchema = "schema"
)

/* func (srv *Server) StartDB() (err error) {
	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(127.0.0.1:3306)/%s", dbUserName, dbPassword, dbSchema))
    if err != nil {
        return
    }

	srv.DB = db
	return
}

func (srv *Server) CloseDB() (err error) {
	err = srv.DB.Close()
	if err != nil {
		return
	}

	return
} */
