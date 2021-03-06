package main

import (
	//"log"
	"net/http"
)

/*
This is an autogenerated file by autobindings
*/

import (
	"github.com/mholt/binding"
)

func (v *Vertigo) FieldMap() binding.FieldMap {
	return binding.FieldMap{
		&v.AllowRegistrations: "allowregistrations",
		&v.CookieHash:         "cookiehash,omitempty",
		&v.Description:        "description",
		&v.Disqus:             "disqus",
		&v.Firstrun:           "firstrun,omitempty",
		&v.GoogleAnalytics:    "ga",
		&v.Hostname:           "hostname",
		&v.Mailer:             "mailgun",
		&v.Markdown:           "markdown",
		&v.Name:               "name",
	}
}

func (v *Vertigo) Validate(req *http.Request, errs binding.Errors) binding.Errors {
	//log.Println("bindingvalidate: ", v)
    return errs
}