package main

import (
	"errors"
	//"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"time"

	"code.google.com/p/go-uuid/uuid"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	_ "github.com/lib/pq"
	"github.com/mailgun/mailgun-go"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mholt/binding"
)

//go:generate autobindings user
type User struct {
	ID       int64  `json:"id" gorm:"primary_key:yes"`
	Name     string `json:"name" form:"name"`
	Email    string `json:"email,omitempty" form:"email" binding:"required" sql:"unique"`
	Password string `json:"password,omitempty" form:"password" sql:"-"`
	Avatar   string `json:"avatar" form:"avatar"`
	Recovery string `json:"-"`
	Digest   []byte `json:"-"`
	Posts    []Post `json:"posts"`
}

// Session or user.Session returns user.ID from client session cookie.
// The returned object has post data merged.
func (user User) Session(r *http.Request) (User, error) {
	session, _ := store.Get(r, SESSIONNAME)
	idr, exists := session.Values["id"]
	if exists {
		id := idr.(int64)
		var user User
		user.ID = id
		user, err := user.Get()
		if err != nil {
			return user, err
		}
		return user, nil
	}
	return user, errors.New(http.StatusText(http.StatusUnauthorized))
}

// Get or user.Get returns User object according to given .ID
// with post information merged.
func (user User) Get() (User, error) {
	query := db.Where(&User{ID: user.ID}).First(&user)
	if query.Error != nil {
		if query.Error == gorm.RecordNotFound {
			return user, errors.New("not found")
		}
		return user, query.Error
	}
	var posts []Post
	query = db.Order("date desc").Where(&Post{Author: user.ID}).Find(&posts)
	if query.Error != nil {
		if query.Error == gorm.RecordNotFound {
			user.Posts = make([]Post, 0)
			return user, nil
		}
		return user, query.Error
	}
	user.Posts = posts
	return user, nil
}

// CreateUser is a route which creates a new user struct according to posted parameters.
// Requires session cookie.
// Returns created user struct for API requests and redirects to "/user" on frontend ones.
func CreateUser(w http.ResponseWriter, r *http.Request) {
	if Settings.AllowRegistrations == false {
		log.Println("Denied a new registration.")
		switch root(r) {
		case "api":
			rend.JSON(w, http.StatusForbidden, map[string]interface{}{"error": "New registrations are not allowed at this time."})
			return
		case "user":
			rend.HTML(w, http.StatusForbidden, "user/login", "New registrations are not allowed at this time.")
			return
		}
	}

	newuser := new(User)
	if errs := binding.Bind(r, newuser); errs != nil {
		log.Println(errs)
	}

	user, err := newuser.Insert(r)
	if err != nil {
		if err.Error() == "UNIQUE constraint failed: users.email" {
			rend.JSON(w, http.StatusConflict, map[string]interface{}{"error": "Email already in use"})
			return
		}
		rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
		return
	}
	user, err = newuser.Login(r)
	if err != nil {
		rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
		return
	}

	SessionSetValue(w, r, "id", user.ID)

	switch root(r) {
	case "api":
		user.Password = ""
		rend.JSON(w, http.StatusOK, user)
		return
	case "user":
		http.Redirect(w, r, "/user", http.StatusFound)
		return
	}
}

// DeleteUser is a route which deletes a user from database according to session cookie.
// The function calls Login function inside, so it also requires password in POST data.
// Currently unavailable function on both API and frontend side.
// func DeleteUser(w http.ResponseWriter, r *http.Request) {
// 	user, err := user.Login(r)
// 	if err != nil {
// 		log.Println(err)
// 		rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
// 		return
// 	}
// 	err = user.Delete(r)
// 	if err != nil {
// 		log.Println(err)
// 		rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
// 		return
// 	}
// 	switch root(r) {
// 	case "api":
// 		s.Delete("user")
// 		rend.JSON(w, http.StatusOK, map[string]interface{}{"status": "User successfully deleted"})
// 		return
// 	case "user":
// 		s.Delete("user")
// 		rend.HTML(w, http.StatusOK, "User successfully deleted", nil)
// 		return
// 	}
// 	rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
// }

// ReadUser is a route which fetches user according to parameter "id" on API side and according to retrieved
// session cookie on frontend side.
// Returns user struct with all posts merged to object on API call. Frontend call will render user "home" page, "user/index.tmpl".
func ReadUser(w http.ResponseWriter, r *http.Request) {
	var user User
	vars := mux.Vars(r)

	switch root(r) {
	case "api":
		id, err := strconv.Atoi(vars["id"])
		if err != nil {
			//log.Println("readuser id: ", err)
			rend.JSON(w, http.StatusBadRequest, map[string]interface{}{"error": "The user ID could not be parsed from the request URL."})
			return
		}
		user.ID = int64(id)
		user, err := user.Get()
		if err != nil {
			if err.Error() == "not found" {
				rend.JSON(w, http.StatusNotFound, NotFound())
				return
			}
			rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
			return
		}
		rend.JSON(w, http.StatusOK, user)
		return
	case "user":
		user, err := user.Session(r)
		if err != nil {
			log.Println("readuser session: ", err)
			SessionSetValue(w, r, "id", -1)
			rend.HTML(w, http.StatusInternalServerError, "error", err)
			return
		}
		rend.HTML(w, http.StatusOK, "user/index", user)
		return
	}
}

// ReadUsers is a route only available on API side, which fetches all users with post data merged.
// Returns complete list of users on success.
func ReadUsers(w http.ResponseWriter, r *http.Request) {
	var user User
	users, err := user.GetAll(r)
	if err != nil {
		log.Println("readusers: ", err)
		rend.JSON(w, http.StatusInternalServerError, err)
		return
	}
	rend.JSON(w, http.StatusOK, users)
}

// LoginUser is a route which compares plaintext password sent with POST request with
// hash stored in database. On successful request returns session cookie named "user", which contains
// user's ID encrypted, which is the primary key used in database table.
// When called by API it responds with user struct.
// On frontend call it redirects the client to "/user" page.
func LoginUser(w http.ResponseWriter, r *http.Request) {

	newuser := new(User)
	errs := binding.Bind(r, newuser)
	if errs != nil {
		log.Println(errs)
	}

	switch root(r) {
	case "api":
		user, err := newuser.Login(r)
		if err != nil {
			if err.Error() == "wrong username or password" {
				rend.JSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "Wrong username or password."})
				return
			}
			if err.Error() == "not found" {
				rend.JSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "User with that email does not exist."})
				return
			}
			rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
			return
		}
		SessionSetValue(w, r, "id", user.ID)
		user.Password = ""
		rend.JSON(w, http.StatusOK, user)
		return
	case "user":
		user, err := newuser.Login(r)
		if err != nil {
			if err.Error() == "wrong username or password" {
				rend.HTML(w, http.StatusUnauthorized, "user/login", "Wrong username or password.")
				return
			}
			if err.Error() == "not found" {
				rend.HTML(w, http.StatusUnauthorized, "user/login", "User with that email does not exist.")
				return
			}
			rend.HTML(w, http.StatusInternalServerError, "user/login", "Internal server error. Please try again.")
			return
		}
		SessionSetValue(w, r, "id", user.ID)
		http.Redirect(w, r, "/user", http.StatusFound)
		return
	}
}

// RecoverUser is a route of the first step of account recovery, which sends out the recovery
// email etc. associated function calls.
func RecoverUser(w http.ResponseWriter, r *http.Request) {
	var user User
	r.ParseForm()
	user.Email = r.PostFormValue("email")

	user, err := user.Recover(r)
	if err != nil {
		log.Println("recoveruser recover: ", err)
		if err.Error() == "not found" {
			rend.JSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "User with that email does not exist."})
			return
		}
		rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
		return
	}
	switch root(r) {
	case "api":
		rend.JSON(w, http.StatusOK, map[string]interface{}{"success": "We've sent you a link to your email which you may use you reset your password."})
		return
	case "user":
		http.Redirect(w, r, "/user/login", http.StatusFound)
		return
	}
}

// ResetUserPassword is a route which is called when accessing the page generated dispatched with
// account recovery emails.
func ResetUserPassword(w http.ResponseWriter, r *http.Request) {
	var user User
	r.ParseForm()
	user.Password = r.PostFormValue("password")

	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		log.Println("resetuserpassword id: ", err)
		rend.JSON(w, http.StatusBadRequest, map[string]interface{}{"error": "User ID could not be parsed from request URL."})
		return
	}
	user.ID = int64(id)

	// FIXME entry?
	entry, err := user.Get()
	if err != nil {
		log.Println("resetuserpassword get: ", err)
		if err.Error() == "not found" {
			rend.JSON(w, http.StatusBadRequest, map[string]interface{}{"error": "User with that ID does not exist."})
			return
		}
		rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
		return
	}
	// this ensures that accounts won't be compromised by posting recovery string as empty,
	// which would otherwise result in succesful password reset
	UUID := uuid.Parse(vars["recovery"])
	if UUID == nil {
		log.Println("there was a problem trying to verify password reset UUID for", entry.Email)
		rend.JSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Could not parse UUID from the request."})
		return
	}
	if entry.Recovery == vars["recovery"] {
		entry.Password = user.Password
		digest, err := GenerateHash(entry.Password)
		if err != nil {
			log.Println("resertuserpassword genhash: ", err)
			rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
			return
		}
		entry.Digest = digest
		entry.Recovery = " "
		_, err = user.Update(r)
		if err != nil {
			log.Println("resetuserpassword update: ", err)
			rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
			return
		}
		switch root(r) {
		case "api":
			rend.JSON(w, http.StatusOK, map[string]interface{}{"success": "Password was updated successfully."})
			return
		case "user":
			http.Redirect(w, r, "/user/login", http.StatusFound)
			return
		}
	}
}

// LogoutUser is a route which deletes session cookie "user", from the given client.
// On API call responds with HTTP 200 body and on frontend the client is redirected to homepage "/".
func LogoutUser(w http.ResponseWriter, r *http.Request) {
	SessionDelete(w, r, "id")
	switch root(r) {
	case "api":
		rend.JSON(w, http.StatusOK, map[string]interface{}{"success": "You've been logged out."})
		return
	case "user":
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
}

// Login or user.Login is a function which retrieves user according to given .Email field.
// The function then compares the retrieved object's .Digest field with given .Password field.
// If the .Password and .Digest match, the function returns the requested User struct, but with
// the .Password and .Digest omitted.
func (user User) Login(r *http.Request) (User, error) {
	password := user.Password
	user, err := user.GetByEmail()
	if err != nil {
		return user, err
	}

	if !CompareHash(user.Digest, password) {
		return user, errors.New("wrong username or password")
	}

	return user, nil
}

// Update or user.Update updates data of "entry" parameter with the data received from "user".
// Can only used to update Name and Digest fields because of how user.Get works.
// Currently not used elsewhere than in password Recovery, that's why the Digest generation.
func (user User) Update(r *http.Request) (User, error) {
	u := UserFromHTTPPost(r)
	log.Println(u)
	query := db.Where(&User{ID: user.ID}).Find(&user).Updates(user)
	if query.Error != nil {
		if query.Error == gorm.RecordNotFound {
			return user, errors.New("not found")
		}
		return user, query.Error
	}
	return user, nil
}

// Recover or user.Recover is used to recover User's password according to user.Email
// The function will insert user.Recovery field with generated UUID string and dispatch an email
// to the corresponding user.Email address. It will also add TTL to Recovery field.
func (user User) Recover(r *http.Request) (User, error) {
	user, err := user.GetByEmail()
	if err != nil {
		return user, err
	}

	// FIXME entry?
	var entry User
	entry.Recovery = uuid.New()
	user, err = user.Update(r)
	if err != nil {
		return user, err
	}

	err = user.SendRecoverMail()
	if err != nil {
		return user, err
	}

	go user.ExpireRecovery(r, 180*time.Minute)

	return user, nil
}

// ExpireRecovery or user.ExpireRecovery sets a TTL according to t to a recovery hash.
// This function is supposed to be run as goroutine to avoid blocking exection for t.
func (user User) ExpireRecovery(r *http.Request, t time.Duration) {
	time.Sleep(t)

	// FIXME entry?
	user.Recovery = " "
	_, err := user.Update(r)
	if err != nil {
		log.Println("expirerecover: ", err)
	}
	return
}

// GetWithPosts or user.GetWithPosts returns User object according to given .ID
// with post information merged.
func (user User) GetWithPosts(r *http.Request) (User, error) {
	var posts []Post
	query := db.Where(&User{ID: user.ID}).First(&user)
	if query.Error != nil {
		if query.Error == gorm.RecordNotFound {
			return user, errors.New("not found")
		}
		return user, query.Error
	}
	query = db.Order("date desc").Where(&Post{Author: user.ID}).Find(&posts)
	if query.Error != nil {
		if query.Error == gorm.RecordNotFound {
			user.Posts = make([]Post, 0)
			return user, nil
		}
		return user, query.Error
	}
	user.Posts = posts
	return user, nil
}

// GetByEmail or user.GetByEmail returns User object according to given .Email
// with post information merged.
func (user User) GetByEmail() (User, error) {
	query := db.Where(&User{Email: user.Email}).First(&user)
	if query.Error != nil {
		if query.Error == gorm.RecordNotFound {
			return user, errors.New("not found")
		}
		return user, query.Error
	}

	return user, nil
}

// Delete or user.Delete deletes the user with given ID from the database.
// func (user User) Delete(db *gorm.DB, s sessions.Session) error {
// 	user, err := user.Session(r)
// 	if err != nil {
// 		return err
// 	}
// 	query := db.Delete(&user)
// 	if query.Error != nil {
// 		if query.Error == gorm.RecordNotFound {
// 			return errors.New("not found")
// 		}
// 		return query.Error
// 	}
// 	return nil
// }

// Insert or user.Insert inserts a new User struct into the database.
// The function creates .Digest hash from .Password.
func (user User) Insert(r *http.Request) (User, error) {
	digest, err := GenerateHash(user.Password)
	if err != nil {
		return user, err
	}
	user.Digest = digest
	user.Posts = make([]Post, 0)
	query := db.Create(&user)
	if query.Error != nil {
		return user, query.Error
	}
	return user, nil
}

// GetAll or user.GetAll fetches all users with post data merged from the database.
func (user User) GetAll(r *http.Request) ([]User, error) {
	var users []User
	query := db.Find(&users)
	if query.Error != nil {
		if query.Error == gorm.RecordNotFound {
			users = make([]User, 0)
			return users, nil
		}
		return users, query.Error
	}
	for index, user := range users {
		user, err := user.Get()
		if err != nil {
			return users, err
		}
		users[index] = user
	}
	return users, nil
}

// SendRecoverMail or user.SendRecoverMail sends mail with Mailgun with pre-filled email layout.
// See Mailgun example on https://gist.github.com/mbanzon/8179682
func (user User) SendRecoverMail() error {
	gun := mailgun.NewMailgun(Settings.Mailer.Domain, Settings.Mailer.PrivateKey, "")
	id := strconv.Itoa(int(user.ID))
	urlhost := urlHost()

	m := mailgun.NewMessage("Password Reset <postmaster@"+Settings.Mailer.Domain+">", "Password Reset", "Somebody requested password recovery on this email. You may reset your password through this link: "+urlhost+"user/reset/"+id+"/"+user.Recovery, "Recipient <"+user.Email+">")
	if _, _, err := gun.Send(m); err != nil {
		return err
	}
	return nil
}

func UserFromHTTPPost(r *http.Request) (user User) {
	log.Printf("\n----------\nufhp:\n%+v\n", r)
	return user
}

/*
session, _ := store.Get(r, SESSIONNAME)
	// Set some session values.
	token, ok := session.Values["foo"]
	if !ok {
		log.Println(ok)
	}
	log.Println(token.(string))
	log.Printf("%+v", session.Values)
	log.Println("settings hostname: ", Settings.Hostname)
	if token.(string) != "ses" {
		rend.JSON(w, http.StatusUnauthorized, map[string]string{"error": http.StatusText(http.StatusUnauthorized)})
		return
	}
*/
