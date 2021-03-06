// Feeds.go contain HTTP routes for rendering RSS and Atom feeds.
package main

import (
	"log"
	"net/http"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/feeds"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

// ReadFeed renders RSS or Atom feed of latest published posts.
// It determines the feed type with strings.Split(r.URL.Path[1:], "/")[1].
func ReadFeed(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "application/xml")

	urlhost := urlHost()

	feed := &feeds.Feed{
		Title:       Settings.Name,
		Link:        &feeds.Link{Href: urlhost},
		Description: Settings.Description,
	}

	//log.Println(urlhost)
	//log.Println(feed)

	var post Post
	posts, err := post.GetAll(r)
	if err != nil {
		log.Println("readfeed posts: ", err)
		rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
		return
	}
	//log.Println(posts)

	for _, post := range posts {

		var user User
		user.ID = post.Author
		user, err := user.Get()
		if err != nil {
			log.Println("readfeed user: ", err)
			rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
			return
		}

		// Don't expose unpublished items to the feeds
		if !post.Published {
			continue
		}

		// The email in &feeds.Author is not actually exported, as it is left out by user.Get().
		// However, the package panics if too few values are exported, so that will do.
		item := &feeds.Item{
			Title:       post.Title,
			Link:        &feeds.Link{Href: urlhost + post.Slug, Rel: "self"},
			Description: post.Excerpt,
			Author:      &feeds.Author{Name: user.Name, Email: user.Email},
			Created:     time.Unix(post.Date, 0),
			Id:          urlhost + post.Slug,
		}
		feed.Add(item)
	}

	// Default to RSS feed.
	result, err := feed.ToRss()
	if err != nil {
		log.Println("readfeed rss: ", err)
		rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
		return
	}

	format := strings.Split(r.URL.Path[1:], "/")[1]
	if format == "atom" {
		result, err = feed.ToAtom()
		if err != nil {
			log.Println("readfeed atom: ", err)
			rend.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Internal server error"})
			return
		}
	}

	w.Write([]byte(result))
}
