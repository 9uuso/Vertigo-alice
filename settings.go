// Settings.go includes everything you would think site-wide settings need. It also contains a installation wizard
// route at the bottom of the file. You generally should not need to change anything in here.
package main

import (
	"encoding/json"
	//"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"runtime"
	//"strconv"

	"code.google.com/p/go-uuid/uuid"
	"github.com/mholt/binding"
	//_ "github.com/go-sql-driver/mysql"
	//"github.com/jinzhu/gorm"
	//_ "github.com/lib/pq"
	//_ "github.com/mattn/go-sqlite3"
)

// Vertigo struct is used as a site wide settings structure. Different from posts and person
// it is saved on local disk in JSON format.
// Firstrun and CookieHash are generated and controlled by the application and should not be
// rendered or made editable anywhere on the site.

//go:generate autobindings vertigo
type Vertigo struct {
	Name               string          `json:"name" form:"name" binding:"required"`
	Hostname           string          `json:"hostname" form:"hostname" binding:"required"`
	Firstrun           bool            `json:"firstrun,omitempty"`
	CookieHash         string          `json:"cookiehash,omitempty"`
	AllowRegistrations bool            `json:"allowregistrations" form:"allowregistrations"`
	Markdown           bool            `json:"markdown" form:"markdown"`
	Description        string          `json:"description" form:"description" binding:"required"`
	Mailer             MailgunSettings `json:"mailgun"`
	Disqus             string          `json:"disqus" form:"disqus"`
	GoogleAnalytics    string          `json:"ga" form:"ga"`
}

// MailgunSettings holds the API keys necessary to send account recovery email.
// You can find the necessary values for these structures in https://mailgun.com/cp
type MailgunSettings struct {
	Domain     string `json:"mgdomain" form:"mgdomain" binding:"required"`
	PrivateKey string `json:"mgprikey" form:"mgprikey" binding:"required"`
}

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU()) // defining gomaxprocs is proven to add performance by few percentages
}

// Settings is a global variable which holds settings stored in the settings.json file.
// You can call it globally anywhere by simply using the Settings keyword. For example
// fmt.Println(Settings.Name) will print out your site's name.
// As mentioned in the Vertigo struct, be careful when dealing with the Firstun and CookieHash values.
var Settings = VertigoSettings()

// VertigoSettings populates the global namespace with data from settings.json.
// If the file does not exist, it creates it.
func VertigoSettings() *Vertigo {
	//settingsfile := fmt.Sprint(settingssource)
	settingsfile := "settings.json"
	_, err := os.OpenFile(settingsfile, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		panic(err)
	}

	data, err := ioutil.ReadFile(settingsfile)
	if err != nil {
		panic(err)
	}

	// If settings file is empty, we presume its a first run.
	if len(data) == 0 {
		var settings Vertigo
		settings.CookieHash = uuid.New()
		settings.Firstrun = true
		jsonconfig, err := json.Marshal(settings)
		if err != nil {
			panic(err)
		}
		err = ioutil.WriteFile(settingsfile, jsonconfig, 0600)
		if err != nil {
			panic(err)
		}
		return VertigoSettings()
	}

	var settings *Vertigo
	if err := json.Unmarshal(data, &settings); err != nil {
		panic(err)
	}
	return settings
}

// Save or Settings.Save is a method which replaces the global Settings structure with the structure is is called with.
// It has builtin variable declaration which prevents you from overwriting CookieHash field.
func (settings *Vertigo) Save() error {
	//settingsfile := fmt.Sprint(settingssource)
	settingsfile := "settings.json"
	data, err := ioutil.ReadFile(settingsfile)
	if err != nil {
		return err
	}

	var old Vertigo
	if err := json.Unmarshal(data, &old); err != nil {
		return err
	}

	Settings = settings
	//fmt.Printf("\n================***** SAVE SETTINGS *****============================\nSettings\n%+v", Settings)
	//fmt.Printf("\n================***** SAVE SETTINGS *****============================\nsettings\n%+v", settings)
	settings.CookieHash = old.CookieHash // this to assure that cookiehash cannot be overwritten even if system is hacked
	jsonconfig, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(settingsfile, jsonconfig, 0600)
	if err != nil {
		return err
	}
	return nil
}

// ReadBlogSettings is a route which reads the local settings.json file.
func ReadBlogSettings(w http.ResponseWriter, r *http.Request) {
	var safesettings Vertigo
	safesettings = *Settings
	safesettings.CookieHash = ""
	switch root(r) {
	case "api":
		rend.JSON(w, http.StatusOK, safesettings)
		return
	case "user":
		session, _ := store.Get(r, SESSIONNAME)
		rend.HTML(w, http.StatusOK, "settings", Page{Session: session, Data: safesettings})
		return
	}
}

// UpdateSettings is a route which updates the local .json settings file.
func UpdateBlogSettings(w http.ResponseWriter, r *http.Request) {
	var err error

	/*
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}
	log.Println("updateblog body: ", string(body))

	ct := r.Header.Get("Content-Type")
	log.Println("ct: ", ct)
	*/

	settings := new(Vertigo)
	if errs := binding.Bind(r, settings); errs != nil {
		log.Println(errs)
	}

	if Settings.Firstrun == false {
		var user User
		user, err := user.Session(r)
		if err != nil {
			//log.Println("updateblogsettings not first run: ", err)
			rend.JSON(w, http.StatusNotAcceptable, map[string]interface{}{"error": "You are not allowed to change the settings this time."})
			return
		}
		settings.CookieHash = Settings.CookieHash
		settings.Firstrun = Settings.Firstrun
		err = settings.Save()
		if err != nil {
			//log.Println("updateblogsettings save: ", err)
			rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": http.StatusText(http.StatusInternalServerError)})
			return
		}
		switch root(r) {
		case "api":
			rend.JSON(w, http.StatusOK, map[string]interface{}{"success": "Settings were successfully saved"})
			return
		case "user":
			http.Redirect(w, r, "/user", http.StatusFound)
			return
		}
	}
	settings.Firstrun = false
	settings.AllowRegistrations = true
	err = settings.Save()
	if err != nil {
		log.Println("updateblogsettings first run: ", err)
		rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": http.StatusText(http.StatusInternalServerError)})
		return
	}
	switch root(r) {
	case "api":
		rend.JSON(w, http.StatusOK, map[string]interface{}{"success": "Settings were successfully saved"})
		return
	case "user":
		http.Redirect(w, r, "/user/register", http.StatusFound)
		return
	}
}
