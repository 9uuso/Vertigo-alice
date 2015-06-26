package main

import (
	"bufio"
	"bytes"
	"errors"

	"log"
	"net/http"
	"strings"
	"time"

	"github.com/9uuso/go-jaro-winkler-distance"
	"github.com/gorilla/mux"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gosimple/slug"
	"github.com/jinzhu/gorm"
	"github.com/kennygrant/sanitize"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"github.com/russross/blackfriday"
)

type Post struct {
	ID        int64  `json:"id" gorm:"primary_key:yes"`
	Title     string `json:"title" form:"title" binding:"required"`
	Content   string `json:"content" form:"content" sql:"type:text"`
	Markdown  string `json:"markdown" form:"markdown" sql:"type:text"`
	Tags      string `json:"tags" form:"tags" sql:"type:text"`
	Date      int64  `json:"date"`
	Slug      string `json:"slug"`
	Author    int64  `json:"author"`
	Excerpt   string `json:"excerpt"`
	Viewcount uint   `json:"viewcount"`
	Published bool   `json:"-"`
}

// Search struct is basically just a type check to make sure people don't add anything nasty to
// on-site search queries.
type Search struct {
	Query string `json:"query" form:"query" binding:"required"`
	Score float64
	Posts []Post
}

// Homepage route fetches all posts from database and renders them according to "home.tmpl".
// Normally you'd use this function as your "/" route.
func Homepage(w http.ResponseWriter, r *http.Request) {
	if Settings.Firstrun {
		rend.HTML(w, http.StatusOK, "installation/wizard", nil)
		return
	}
	var post Post
	posts, err := post.GetAll(r)
	if err != nil {
		log.Println("homepage err: ", err)
		rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
		return
	}
	SessionGetValue(r, "id")
	rend.HTML(w, http.StatusOK, "home", posts)
}

// Excerpt generates 15 word excerpt from given input.
// Used to make shorter summaries from blog posts.
func Excerpt(input string) string {
	scanner := bufio.NewScanner(strings.NewReader(input))
	scanner.Split(bufio.ScanWords)
	count := 0
	var excerpt bytes.Buffer
	for scanner.Scan() && count < 15 {
		count++
		excerpt.WriteString(scanner.Text() + " ")
	}
	return sanitize.HTML(strings.TrimSpace(excerpt.String()))
}

// SearchPost is a route which returns all posts and aggregates the ones which contain
// the POSTed search query in either Title or Content field.
func SearchPost(w http.ResponseWriter, r *http.Request) {
	var search Search

	r.ParseForm()
	search.Query = r.PostFormValue("query")

	search, err := search.Get(r)
	if err != nil {
		log.Println("search post: ", err)
		rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
		return
	}
	switch root(r) {
	case "api":
		rend.JSON(w, http.StatusOK, search.Posts)
		return
	case "post":
		rend.HTML(w, http.StatusOK, "search", search.Posts)
		return
	}
}

// Get or search.Get returns all posts which contain parameter search.Query in either
// post.Title or post.Content.
// Returns []Post and error object.
func (search Search) Get(r *http.Request) (Search, error) {
	var post Post

	posts, err := post.GetAll(r)
	if err != nil {
		log.Println("get: ", err)
		return search, err
	}
	for _, post := range posts {
		if post.Published {
			// posts are searched for a match in both content and title, so here
			// we declare two scanners for them
			content := bufio.NewScanner(strings.NewReader(post.Content))
			title := bufio.NewScanner(strings.NewReader(post.Title))
			// Blackfriday makes smartypants corrections some characters, which break the search
			if Settings.Markdown {
				content = bufio.NewScanner(strings.NewReader(post.Markdown))
				title = bufio.NewScanner(strings.NewReader(post.Title))
			}
			content.Split(bufio.ScanWords)
			title.Split(bufio.ScanWords)
			// content is scanned trough Jaro-Winkler distance with
			// quite strict matching score of 0.9/1
			// matching score this high would most likely catch only different
			// capitalization and small typos
			//
			// since we are already in a for loop, we have to break the
			// iteration here by going to label End to avoid showing a
			// duplicate search result
			for content.Scan() {
				if jwd.Calculate(content.Text(), search.Query) >= 0.9 {
					search.Posts = append(search.Posts, post)
					goto End
				}
			}
			for title.Scan() {
				if jwd.Calculate(title.Text(), search.Query) >= 0.9 {
					search.Posts = append(search.Posts, post)
					goto End
				}
			}
		}
	End:
	}
	if len(search.Posts) == 0 {
		search.Posts = make([]Post, 0)
	}
	return search, nil
}

// CreatePost is a route which creates a new post according to the posted data.
// API response contains the created post object and normal request redirects to "/user" page.
// Does not publish the post automatically. See PublishPost for more.
func CreatePost(w http.ResponseWriter, r *http.Request) {
	var post Post

	r.ParseForm()
	post.Title = r.PostFormValue("title")
	post.Markdown = r.PostFormValue("markdown")
	post.Content = r.PostFormValue("content")

	post, err := post.Insert(r)
	if err != nil {
		log.Println("create post: ", err)
		rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
		return
	}

	switch root(r) {
	case "api":
		rend.JSON(w, http.StatusOK, post)
		return
	case "post":
		http.Redirect(w, r, "/user", http.StatusFound)
		return
	}
}

// ReadPosts is a route which returns all posts without merged owner data (although the object does include author field)
// Not available on frontend, so therefore it only returns a JSON response.
func ReadPosts(w http.ResponseWriter, r *http.Request) {
	var post Post
	published := make([]Post, 0)
	posts, err := post.GetAll(r)
	if err != nil {
		log.Println("readposts: ", err)
		rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
		return
	}
	for _, post := range posts {
		if post.Published {
			published = append(published, post)
		}
	}
	rend.JSON(w, http.StatusOK, published)
}

// ReadPost is a route which returns post with given post.Slug.
// Returns post data on JSON call and displays a formatted page on frontend.
func ReadPost(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	slug := vars["slug"]

	var post Post
	if slug == "new" {
		rend.JSON(w, http.StatusBadRequest, map[string]interface{}{"error": "There can't be a post called 'new'."})
		return
	}
	post.Slug = slug
	post, err := post.Get(r)
	if err != nil {
		//log.Println("readpost: ", err)
		if err.Error() == "not found" {
			rend.JSON(w, http.StatusNotFound, NotFound())
			return
		}
		rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
		return
	}
	go post.Increment(r)
	switch root(r) {
	case "api":
		rend.JSON(w, http.StatusOK, post)
		return
	case "post":
		rend.HTML(w, http.StatusOK, "post/display", post)
		return
	}
}

// EditPost is a route which returns a post object to be displayed and edited on frontend.
// Not available for JSON API.
// Analogous to ReadPost. Could be replaced at some point.
func EditPost(w http.ResponseWriter, r *http.Request) {
	var post Post

	vars := mux.Vars(r)
	post.Slug = vars["slug"]

	post, err := post.Get(r)
	if err != nil {
		log.Println("editpost: ", err)
		rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
		return
	}
	rend.HTML(w, http.StatusOK, "post/edit", post)
}

// UpdatePost is a route which updates a post defined by martini parameter "title" with posted data.
// Requires session cookie. JSON request returns the updated post object, frontend call will redirect to "/user".
func UpdatePost(w http.ResponseWriter, r *http.Request) {
	var post Post

	vars := mux.Vars(r)
	post.Slug = vars["slug"]

	post, err := post.Get(r)
	if err != nil {
		log.Println("update post: ", err)
		if err.Error() == "not found" {
			rend.JSON(w, http.StatusNotFound, NotFound())
			return
		}
		rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
		return
	}

	var user User
	user, err = user.Session(r)
	if err != nil {
		log.Println("updatepost session: ", err)
		rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
		return
	}

	if post.Author == user.ID {
		post, err = post.Update(r)
		if err != nil {
			log.Println("updatepost post: ", err)
			rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
			return
		}
	} else {
		rend.JSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "Unauthorized"})
		return
	}

	switch root(r) {
	case "api":
		rend.JSON(w, http.StatusOK, post)
		return
	case "post":
		http.Redirect(w, r, "/user", http.StatusFound)
		return
	}
}

// PublishPost is a route which publishes a post and therefore making it appear on frontpage and search.
// JSON request returns `HTTP 200 {"success": "Post published"}` on success. Frontend call will redirect to
// published page.
// Requires active session cookie.
func PublishPost(w http.ResponseWriter, r *http.Request) {
	var post Post

	vars := mux.Vars(r)
	post.Slug = vars["slug"]

	post, err := post.Get(r)
	if err != nil {
		if err.Error() == "not found" {
			rend.JSON(w, http.StatusNotFound, NotFound())
			return
		}
		rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
		return
	}

	var user User
	user, err = user.Session(r)
	if err != nil {
		log.Println("publishpost user: ", err)
		rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
		return
	}

	if post.Author == user.ID {
		post.Published = true
		post, err = post.Update(r)
		if err != nil {
			log.Println("publishpost post: ", err)
			rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
			return
		}
	} else {
		rend.JSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "Unauthorized"})
		return
	}

	switch root(r) {
	case "api":
		rend.JSON(w, http.StatusOK, map[string]interface{}{"success": "Post published"})
		return
	case "post":
		http.Redirect(w, r, "/post/"+post.Slug, http.StatusFound)
		return
	}
}

// UnpublishPost is a route which unpublishes a post and therefore making it disappear from frontpage and search.
// JSON request returns `HTTP 200 {"success": "Post unpublished"}` on success. Frontend call will redirect to
// user control panel.
// Requires active session cookie.
// The route is anecdotal to route PublishPost().
func UnpublishPost(w http.ResponseWriter, r *http.Request) {
	var post Post

	vars := mux.Vars(r)
	post.Slug = vars["slug"]

	post, err := post.Get(r)
	if err != nil {
		if err.Error() == "not found" {
			rend.JSON(w, http.StatusNotFound, NotFound())
			return
		}
		rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
		return
	}

	var user User
	user, err = user.Session(r)
	if err != nil {
		log.Println("unpublishpost user: ", err)
		rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
		return
	}

	if post.Author == user.ID {
		err = post.Unpublish(r)
		if err != nil {
			log.Println("unpublishpost post: ", err)
			rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
			return
		}
	} else {
		rend.JSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "Unauthorized"})
		return
	}

	switch root(r) {
	case "api":
		rend.JSON(w, http.StatusOK, map[string]interface{}{"success": "Post unpublished"})
		return
	case "post":
		http.Redirect(w, r, "/user", http.StatusFound)
		return
	}
}

// DeletePost is a route which deletes a post according to martini parameter "title".
// JSON request returns `HTTP 200 {"success": "Post deleted"}` on success. Frontend call will redirect to
// "/user" page on successful request.
// Requires active session cookie.
func DeletePost(w http.ResponseWriter, r *http.Request) {
	var post Post

	vars := mux.Vars(r)
	post.Slug = vars["slug"]

	post, err := post.Get(r)
	if err != nil {
		if err.Error() == "not found" {
			rend.JSON(w, http.StatusNotFound, NotFound())
			return
		}
		log.Println("deletepost post: ", err)
		rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
		return
	}

	err = post.Delete(r)
	if err != nil {
		log.Println("deletepost delete: ", err)
		if err.Error() == "unauthorized" {
			rend.JSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "Unauthorized"})
			return
		}
		rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
		return
	}
	switch root(r) {
	case "api":
		rend.JSON(w, http.StatusOK, map[string]interface{}{"success": "Post deleted"})
		return
	case "post":
		http.Redirect(w, r, "/user", http.StatusFound)
		return
	}
}

// Insert or post.Insert inserts Post object into database.
// Requires active session cookie
// Fills post.Author, post.Date, post.Excerpt, post.Slug and post.Published automatically.
// Returns Post and error object.
func (post Post) Insert(r *http.Request) (Post, error) {
	var user User
	user, err := user.Session(r)
	if err != nil {
		return post, err
	}
	// if post.Content is empty, the user has used Markdown editor
	if Settings.Markdown {
		post.Content = string(blackfriday.MarkdownCommon([]byte(cleanup(post.Markdown))))
	} else {
		post.Content = cleanup(post.Content)
	}
	post.Author = user.ID
	post.Date = time.Now().Unix()
	post.Excerpt = Excerpt(post.Content)
	post.Slug = slug.Make(post.Title)
	post.Published = false
	query := db.Create(&post)
	if query.Error != nil {
		return post, query.Error
	}
	//log.Println("query: ", query)
	return post, nil
}

// Get or post.Get returns post according to given post.Slug.
// Requires db session as a parameter.
// Returns Post and error object.
func (post Post) Get(r *http.Request) (Post, error) {
	query := db.Find(&post, Post{Slug: post.Slug})
	if query.Error != nil {
		if query.Error == gorm.RecordNotFound {
			return post, errors.New("not found")
		}
		return post, query.Error
	}
	return post, nil
}

// GetAll or post.GetAll returns all posts in database.
// Returns []Post and error object.
func (post Post) GetAll(r *http.Request) ([]Post, error) {
	var posts []Post
	query := db.Order("date desc").Find(&posts)
	if query.Error != nil {
		if query.Error == gorm.RecordNotFound {
			posts = make([]Post, 0)
			return posts, nil
		}
		return posts, query.Error
	}
	return posts, nil
}

// This function brings sanity to contenteditable. It mainly removes unnecessary <br> lines from the input source.
// Part of the sanitize package, but this one fixes issues with <code> blocks having &nbsp;'s all over.
// https://github.com/kennygrant/sanitize/blob/master/sanitize.go#L106
func cleanup(s string) string {
	// First remove line breaks etc as these have no meaning outside html tags (except pre)
	// this means pre sections will lose formatting... but will result in less uninentional paras.
	s = strings.Replace(s, "\n", "", -1)

	// Then replace line breaks with newlines, to preserve that formatting
	s = strings.Replace(s, "</p>", "\n", -1)
	s = strings.Replace(s, "<br>", "\n", -1)
	s = strings.Replace(s, "</br>", "\n", -1)
	s = strings.Replace(s, "<br/>", "\n", -1)

	return s
}

// Update or post.Update updates parameter "entry" with data given in parameter "post".
// Requires active session cookie.
// Returns updated Post object and an error object.
func (post Post) Update(r *http.Request) (Post, error) {
	// entry is required apparently, only way i can get it to actually update.
	var entry = post
	if Settings.Markdown {
		entry.Markdown = cleanup(post.Markdown)
		entry.Content = string(blackfriday.MarkdownCommon([]byte(post.Markdown)))
	} else {
		entry.Content = cleanup(post.Content)
		// this closure would need a call to convert HTML to Markdown
		// see https://github.com/9uuso/vertigo/issues/7
		// entry.Markdown = Markdown of entry.Content
	}
	entry.Excerpt = Excerpt(post.Content)
	query := db.Where(&Post{Slug: post.Slug}).First(&post).Updates(entry)
	if query.Error != nil {
		if query.Error == gorm.RecordNotFound {
			return post, errors.New("not found")
		}
		return post, query.Error
	}
	return post, nil
}

// Unpublish or post.Unpublish unpublishes a post by updating the Published value to false.
// Gorm specific, declared only because the libaray has a bug.
func (post Post) Unpublish(r *http.Request) error {
	var user User
	user, err := user.Session(r)
	if err != nil {
		return err
	}
	if post.Author == user.ID {
		query := db.Where(&Post{Slug: post.Slug}).Find(&post).Update("published", false)
		if query.Error != nil {
			if query.Error == gorm.RecordNotFound {
				return errors.New("not found")
			}
			return query.Error
		}
	} else {
		return errors.New("unauthorized")
	}
	return nil
}

// Delete or post.Delete deletes a post according to post.Slug.
// Requires session cookie.
// Returns error object.
func (post Post) Delete(r *http.Request) error {
	var user User
	user, err := user.Session(r)
	if err != nil {
		return err
	}
	if post.Author == user.ID {
		query := db.Where(&Post{Slug: post.Slug}).Delete(&post)
		if query.Error != nil {
			if query.Error == gorm.RecordNotFound {
				return errors.New("not found")
			}
			return query.Error
		}
	} else {
		return errors.New("unauthorized")
	}
	return nil
}

// Increment or post.Increment increases viewcount of a post according to its post.ID
// It is supposed to be run as a gouroutine, so therefore it does not return anything.
func (post Post) Increment(r *http.Request) {
	post.Viewcount++
	_, err := post.Update(r)
	if err != nil {
		log.Println("analytics error:", err)
	}
}
