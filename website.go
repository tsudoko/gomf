package main

import (
	"fmt"
	"html/template"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
)

var templates *template.Template

func initWebsite() {
	pages, err := ioutil.ReadDir("pages")
	if err != nil {
		panic(err)
	}

	templates = template.Must(template.ParseGlob("pages/*.html"))

	for _, page := range pages {
		if path.Ext(page.Name()) == ".html" {
			http.HandleFunc("/"+page.Name(), handlePage)
			if page.Name() == "index.html" {
				http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/" {
						http.NotFound(w, r)
						return
					}
					handlePage(w, r)
				})
			}
		}
	}

	http.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir("static"))))
	http.HandleFunc("/favicon.ico", handleFavicon)
}

func humanize(bytes int64) string {
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB", "ZiB", "YiB"}
	i := 0
	n := float64(bytes)
	for n >= 1024 && i < len(units)-1 {
		n /= 1024
		i += 1
	}
	return strconv.FormatFloat(n, 'f', -1, 64) + " " + units[i]
}

func handleFavicon(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/favicon.ico")
}

type pageContext struct {
	SiteName     string
	Abuse        string
	Contact      string
	MaxSizeBytes int64
	MaxSize      string
	Pages        map[string]string
	Result       response
}

func newContext() pageContext {
	pages := make(map[string]string)
	for _, t := range templates.Templates() {
		n := t.Name()
		title := n[:len(n)-len(path.Ext(n))]
		title = strings.ToUpper(title[0:1]) + title[1:]
		pages[title] = n
	}
	return pageContext{
		SiteName:     siteName,
		Abuse:        abuseMail,
		Contact:      contactMail,
		MaxSizeBytes: storage.MaxSize,
		MaxSize:      humanize(storage.MaxSize),
		Pages:        pages,
	}
}

func handlePage(w http.ResponseWriter, r *http.Request) {
	page := strings.TrimLeft(r.URL.Path, "/")
	if page == "" {
		page = "index.html"
	}
	if err := templates.ExecuteTemplate(w, page, newContext()); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

func handleGrill(w http.ResponseWriter, r *http.Request) {
	grills, err := ioutil.ReadDir("static/grill/")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/static/grill/"+grills[rand.Intn(len(grills))].Name(), http.StatusFound)
}
