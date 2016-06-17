package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

var storage *Storage

var (
	uploadUrl   string
	uploadHost  string
	siteName    string
	contactMail string
	abuseMail   string
	csp         string
	hsts        bool
	allowHtml   bool
	cors        bool
)

func handle(w http.ResponseWriter, r *http.Request) {
	if hsts {
		w.Header().Set("Strict-Transport-Security", "max-age=15552000")
	}
	if uploadHost != "" && r.URL.Host == uploadHost {
		handleFile(w, r)
	} else {
		http.DefaultServeMux.ServeHTTP(w, r)
	}
}

func globalHandler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cors {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		if r.Method == http.MethodGet || r.Method == http.MethodPost || r.Method == http.MethodHead {
			handler.ServeHTTP(w, r)
		} else {
			w.Header().Set("Allow", "POST, HEAD, OPTIONS, GET")
			if r.Method != http.MethodOptions {
				http.Error(w, "The method is not allowed for the requested URL.", http.StatusMethodNotAllowed)
			}
		}
	})
}

func main() {
	flag.StringVar(&uploadUrl, "upload-url", "", "URL to serve uploads from")
	flag.StringVar(&uploadHost, "upload-host", "", "host to serve uploads on")
	flag.StringVar(&siteName, "name", "Gomf", "website name")
	flag.StringVar(&contactMail, "contact", "contact@example.com", "contact email address")
	flag.StringVar(&abuseMail, "abuse", "abuse@example.com", "abuse email address")
	flag.StringVar(&csp, "csp", "default-src 'none'; media-src 'self'", "the Content-Security-Policy header for files; blank to disable")
	flag.BoolVar(&hsts, "hsts", false, "enable HSTS")
	flag.BoolVar(&allowHtml, "allow-html", false, "serve (X)HTML uploads with (X)HTML filetypes")
	flag.BoolVar(&cors, "cors", false, "enable CORS and allow all origins")
	listenHttp := flag.String("http", "localhost:8080", "address to listen on for HTTP")
	listenHttps := flag.String("https", "", "address to listen on for HTTPS")
	cert := flag.String("cert", "", "path to TLS certificate (for HTTPS)")
	key := flag.String("key", "", "path to TLS key (for HTTPS)")
	maxSize := flag.Int64("max-size", 50*1024*1024, "max filesize in bytes")
	forbidMime := flag.String("forbid-mime", "application/x-dosexec,application/x-msdos-program", "comma-separated list of forbidden MIME types")
	forbidExt := flag.String("forbid-ext", "exe,dll,msi,scr,com,pif", "comma-separated list of forbidden file extensions")
	grill := flag.Bool("grill", false, "enable grills")
	idLength := flag.Int("id-length", 6, "length of uploaded file IDs")
	idCharset := flag.String("id-charset", "", "charset for uploaded file IDs (default lowercase letters a-z)")

	flag.Parse()

	rand.Seed(time.Now().UnixNano())

	initWebsite()

	storage = NewStorage("upload", *maxSize)
	storage.ForbiddenExt = strings.Split(*forbidExt, ",")
	storage.ForbiddenMime = strings.Split(*forbidMime, ",")
	storage.IdLength = *idLength
	if *idCharset != "" {
		storage.IdCharset = *idCharset
	}

	http.HandleFunc("/upload.php", handleUpload)
	http.Handle("/u/", http.StripPrefix("/u/", http.HandlerFunc(handleFile)))
	if *grill {
		http.HandleFunc("/grill.php", handleGrill)
	}

	if uploadUrl == "" {
		if *listenHttps != "" {
			if uploadHost != "" {
				uploadUrl = "https://" + uploadHost + "/"
			} else {
				uploadUrl = "https://" + *listenHttps + "/u/"
			}
		} else if *listenHttp != "" {
			if uploadHost != "" {
				uploadUrl = "http://" + uploadHost + "/"
			} else {
				uploadUrl = "http://" + *listenHttp + "/u/"
			}
		}
	}

	exit := true
	if *listenHttp != "" {
		exit = false
		fmt.Printf("listening on http://%s/\n", *listenHttp)
		go panic(http.ListenAndServe(*listenHttp, globalHandler(http.HandlerFunc(handle))))
	}
	if *listenHttps != "" {
		exit = false
		fmt.Printf("listening on https://%s/\n", *listenHttps)
		go panic(http.ListenAndServeTLS(*listenHttps, *cert, *key, globalHandler(http.HandlerFunc(handle))))
	}

	if !exit {
		switch {
		}
	}
}
