package main

import (
	"html"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/throttled"
	"github.com/justinas/alice"
	//"github.com/justinas/nosurf"
	"github.com/gorilla/mux"
	"github.com/unrolled/render"
)

func timeoutHandler(h http.Handler) http.Handler {
	return http.TimeoutHandler(h, 1*time.Second, "timed out")
}

var helpers = template.FuncMap{
	// Unescape unescapes and parses HTML from database objects.
	// Used in templates such as "/post/display.tmpl"
	"unescape": func(s string) template.HTML {
		return template.HTML(html.UnescapeString(s))
	},
	// Title renders post name as a page title.
	"title": func(t interface{}) string {
		post, exists := t.(Post)
		if exists {
			return post.Title
		}
		return Settings.Name
	},
	// Page Title renders page title.
	"pagetitle": func(t interface{}) string {
		if Settings.Name == "" {
			return "Vertigo"
		}
		return Settings.Name
	},
	// Description renders page description.
	"description": func(t interface{}) string {
		if Settings.Description == "" {
			return "Blog in Go"
		}
		return Settings.Description
	},
	// Hostname renders page hostname.
	"hostname": func(t interface{}) string {
		return urlHost()
	},
	// Date helper returns unix date as more readable one in string format. Format of YYYY-MM-DD
	// https://html.spec.whatwg.org/multipage/semantics.html#datetime-value
	"date": func(d int64) string {
		return time.Unix(d, 0).Format("2006-01-02")
	},
	// Env helper returns environment variable of s.
	"env": func(s string) string {
		if s == "MAILGUN_SMTP_LOGIN" {
			return strings.TrimLeft(os.Getenv(s), "postmaster@")
		}
		return os.Getenv(s)
	},
	// Markdown returns whether user has Markdown enabled from settings.
	"Markdown": func() bool {
		if Settings.Markdown {
			return true
		}
		return false
	},
	// ReadOnly checks whether a post is safe to edit with current settings.
	"ReadOnly": func(p Post) bool {
		if Settings.Markdown && p.Markdown == "" {
			return true
		}
		return false
	},
}

var rend = render.New(render.Options{
	Directory:  "templates",
	Layout:     "layout",
	Extensions: []string{".tmpl", ".html"},
	Delims:     render.Delims{"{[", "]}"},
	Funcs:      []template.FuncMap{helpers},
})

// Context
type key int

const MyKey key = 0

var db = initDB()

var logit = new(LogWriter)

func init() {
	logit.Enable()
}

func NewServer() http.Handler {
	th := throttled.Interval(throttled.PerSec(10), 1, &throttled.VaryBy{Path: true}, 50)

	r := mux.NewRouter()

	// Handle Static files
	r.Handle("/css/{rest}", http.StripPrefix("/css/", http.FileServer(http.Dir("./public/css/")))).Methods("GET")
	r.Handle("/js/{rest}", http.StripPrefix("/js/", http.FileServer(http.Dir("./public/js/")))).Methods("GET")
	r.Handle("/uploads/{rest}", http.StripPrefix("/uploads/", http.FileServer(http.Dir("./public/uploads/")))).Methods("GET")

	// Handle Root
	r.Handle("/", alice.New(th.Throttle, timeoutHandler).Then(http.HandlerFunc(Homepage))).Methods("GET")

	// Handle feeds
	r.HandleFunc("/feeds", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/feeds/rss", http.StatusFound)
	})
	r.HandleFunc("/feeds/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/feeds/rss", http.StatusFound)
	})
	r.HandleFunc("/feeds/atom", ReadFeed).Methods("GET")
	r.HandleFunc("/feeds/rss", ReadFeed).Methods("GET")

	// route: /post

	// Please note that `/new` route has to be before the `/:slug` route. Otherwise the program will try
	// to fetch for Post named "new".
	// For now I'll keep it this way to streamline route naming.
	r.Handle("/post/new", alice.New(th.Throttle, timeoutHandler, ProtectedPage).Then(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rend.HTML(w, http.StatusOK, "post/new", nil)
	}))).Methods("GET")
	r.Handle("/post/new", alice.New(th.Throttle, timeoutHandler, ProtectedPage, StrictWWWFormUrlEncoded).Then(http.HandlerFunc(CreatePost))).Methods("POST")
	r.Handle("/post/search", alice.New(th.Throttle, timeoutHandler, StrictWWWFormUrlEncoded).Then(http.HandlerFunc(SearchPost))).Methods("POST")
	r.HandleFunc("/post/{slug}", ReadPost).Methods("GET")
	r.Handle("/post/{slug}/edit", alice.New(th.Throttle, timeoutHandler, ProtectedPage).Then(http.HandlerFunc(EditPost))).Methods("GET")
	r.Handle("/post/{slug}/edit", alice.New(th.Throttle, timeoutHandler, ProtectedPage, StrictWWWFormUrlEncoded).Then(http.HandlerFunc(UpdatePost))).Methods("POST")
	r.Handle("/post/{slug}/delete", alice.New(th.Throttle, timeoutHandler, ProtectedPage).Then(http.HandlerFunc(DeletePost))).Methods("GET")
	r.Handle("/post/{slug}/publish", alice.New(th.Throttle, timeoutHandler, ProtectedPage).Then(http.HandlerFunc(PublishPost))).Methods("GET")
	r.Handle("/post/{slug}/unpublish", alice.New(th.Throttle, timeoutHandler, ProtectedPage).Then(http.HandlerFunc(UnpublishPost))).Methods("GET")

	// route: /user
	r.Handle("/user", alice.New(th.Throttle, timeoutHandler, ProtectedPage).Then(http.HandlerFunc(ReadUser))).Methods("GET")
	r.Handle("/user/login", alice.New(th.Throttle, timeoutHandler, SessionRedirect).Then(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rend.HTML(w, http.StatusOK, "user/login", nil)
	}))).Methods("GET")
	r.Handle("/user/login", alice.New(th.Throttle, timeoutHandler, SessionRedirect, StrictWWWFormUrlEncoded).Then(http.HandlerFunc(LoginUser))).Methods("POST")
	//r.Handle("/user/login", alice.New(th.Throttle, timeoutHandler).Then(http.HandlerFunc(LoginUser))).Methods("POST")
	r.HandleFunc("/user/logout", LogoutUser).Methods("GET")
	r.Handle("/user/settings", alice.New(th.Throttle, timeoutHandler, ProtectedPage).Then(http.HandlerFunc(ReadBlogSettings))).Methods("GET")
	r.Handle("/user/settings", alice.New(th.Throttle, timeoutHandler, ProtectedPage, StrictWWWFormUrlEncoded).Then(http.HandlerFunc(UpdateBlogSettings))).Methods("POST")
	r.Handle("/user/installation", alice.New(th.Throttle, timeoutHandler, StrictWWWFormUrlEncoded).Then(http.HandlerFunc(UpdateBlogSettings))).Methods("POST")
	r.Handle("/user/register", alice.New(th.Throttle, timeoutHandler, SessionRedirect).Then(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rend.HTML(w, http.StatusOK, "user/register", nil)
	}))).Methods("GET")
	r.Handle("/user/register", alice.New(th.Throttle, timeoutHandler, ProtectedPage, StrictWWWFormUrlEncoded).Then(http.HandlerFunc(CreateUser))).Methods("POST")
	r.Handle("/user/recover", alice.New(th.Throttle, timeoutHandler, SessionRedirect).Then(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rend.HTML(w, http.StatusOK, "user/recover", nil)
	}))).Methods("GET")
	r.Handle("/user/recover", alice.New(th.Throttle, timeoutHandler, ProtectedPage, StrictWWWFormUrlEncoded).Then(http.HandlerFunc(RecoverUser))).Methods("POST")
	r.Handle("/user/reset/{id}/{recovery}", alice.New(th.Throttle, timeoutHandler, SessionRedirect).Then(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rend.HTML(w, http.StatusOK, "user/reset", nil)
	}))).Methods("GET")
	r.Handle("/user/reset/{id}/{recovery}", alice.New(th.Throttle, timeoutHandler, ProtectedPage, StrictWWWFormUrlEncoded).Then(http.HandlerFunc(ResetUserPassword))).Methods("POST")
	//r.Post("/delete", strict.ContentType("application/x-www-form-urlencoded"), ProtectedPage, binding.Form(User{}), DeleteUser)

	// route: /api
	r.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		rend.HTML(w, http.StatusOK, "api/index", nil)
	})
	r.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		rend.HTML(w, http.StatusOK, "api/index", nil)
	})
	r.Handle("/api/settings", alice.New(th.Throttle, timeoutHandler, ProtectedPage).Then(http.HandlerFunc(ReadBlogSettings))).Methods("GET")
	r.Handle("/api/settings", alice.New(th.Throttle, timeoutHandler, ProtectedPage, StrictJSON).Then(http.HandlerFunc(UpdateBlogSettings))).Methods("POST")
	r.Handle("/api/installation", alice.New(th.Throttle, timeoutHandler, StrictJSON).Then(http.HandlerFunc(UpdateBlogSettings))).Methods("POST")
	r.HandleFunc("/api/users", ReadUsers).Methods("GET")

	r.Handle("/api/user/login", alice.New(th.Throttle, timeoutHandler, SessionRedirect, StrictJSON).Then(http.HandlerFunc(LoginUser))).Methods("POST")
	//r.Handle("/api/user/login", alice.New(th.Throttle, timeoutHandler).Then(http.HandlerFunc(LoginUser))).Methods("POST")
	r.HandleFunc("/api/user/logout", LogoutUser).Methods("GET")
	r.Handle("/api/user/recover", alice.New(th.Throttle, timeoutHandler, StrictJSON).Then(http.HandlerFunc(RecoverUser))).Methods("POST")
	r.Handle("/api/user/reset/{id}/{recovery}", alice.New(th.Throttle, timeoutHandler, StrictJSON).Then(http.HandlerFunc(ResetUserPassword))).Methods("POST")

	r.HandleFunc("/api/user/{id}", ReadUser).Methods("GET")
	//r.Delete("/user", DeleteUser)
	r.Handle("/api/user", alice.New(th.Throttle, timeoutHandler, StrictJSON).Then(http.HandlerFunc(CreateUser))).Methods("POST")
	r.HandleFunc("/api/posts", ReadPosts).Methods("GET")
	r.HandleFunc("/api/post/{slug}", ReadPost).Methods("GET")
	r.Handle("/api/post/{slug}/edit", alice.New(th.Throttle, timeoutHandler, ProtectedPage, StrictJSON).Then(http.HandlerFunc(UpdatePost))).Methods("POST")
	r.Handle("/api/post/{slug}/publish", alice.New(th.Throttle, timeoutHandler, ProtectedPage).Then(http.HandlerFunc(PublishPost))).Methods("GET")
	r.Handle("/api/post/{slug}/unpublish", alice.New(th.Throttle, timeoutHandler, ProtectedPage).Then(http.HandlerFunc(UnpublishPost))).Methods("GET")
	r.Handle("/api/post/{slug}/delete", alice.New(th.Throttle, timeoutHandler, ProtectedPage).Then(http.HandlerFunc(DeletePost))).Methods("GET")
	r.Handle("/api/post", alice.New(th.Throttle, timeoutHandler, ProtectedPage, StrictJSON).Then(http.HandlerFunc(CreatePost))).Methods("POST")
	r.Handle("/api/post/search", alice.New(th.Throttle, timeoutHandler, StrictJSON).Then(http.HandlerFunc(SearchPost))).Methods("POST")

	return alice.New(Logger).Then(r)
}

func main() {
	server := NewServer()
	log.Println("listening port 8000")
	http.ListenAndServe(":8000", server)
}

func Logger(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//startTime := time.Now()
		h.ServeHTTP(w, r)
		//log.Printf("[vertigo] %s %s (%v)\n", r.Method, r.URL.Path, time.Since(startTime))
	})
}

