package main

// root returns HTTP request "root".

import (
	"flag"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jinzhu/gorm"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

// FIXME: are these even used?
//var driver = flag.String("driver", "sqlite3", "name of the database driver used, by default sqlite3")
//var dbsource = flag.String("dbsource", "./vertigo.db", "connection string or path to database file")
var settingssource = flag.String("settings", "./settings.json", "path to settings file")

// For example, calling it with http.Request which has URL of /api/user/5348482a2142dfb84ca41085
// would return "api". This function is used to route both JSON API and frontend requests in the same function.
func root(r *http.Request) string {
	return strings.Split(strings.TrimPrefix(r.URL.String(), "/"), "/")[0]
}

// NotFound is a shorthand JSON response for HTTP 404 errors.
func NotFound() map[string]interface{} {
	return map[string]interface{}{"error": http.StatusText(http.StatusNotFound)}
}

func urlHost() string {
	return Settings.Hostname
}

func initDB() *gorm.DB {

	if os.Getenv("DATABASE_URL") != "" {
		os.Setenv("driver", "postgres")
		os.Setenv("dbsource", os.Getenv("DATABASE_URL"))
		log.Println("Using PostgreSQL")
	} else {
		os.Setenv("driver", "sqlite3")
		os.Setenv("dbsource", "./vertigo.db")
		log.Println("Using SQLite3")
	}

	db, err := gorm.Open(os.Getenv("driver"), os.Getenv("dbsource"))

	if err != nil {
		panic(err)
	}

	db.LogMode(false)

	// Here database and tables are created in case they do not exist yet.
	// If database or tables do exist, nothing will happen to the original ones.
	db.CreateTable(&User{})
	db.CreateTable(&Post{})
	db.AutoMigrate(&User{}, &Post{})

	return &db
}

type LogWriter struct{ io.Writer }
func (w *LogWriter) Enable()  { w.Writer = os.Stdout }
func (w *LogWriter) Disable() { w.Writer = ioutil.Discard }