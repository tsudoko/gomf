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
	uploadUrl     string
	uploadHost    string
	siteName      string
	contactMail   string
	abuseMail     string
	csp           string
	hsts          bool
	allowHtml     bool
	cors          bool
	redirectHttps bool
)

func handle(w http.ResponseWriter, r *http.Request) {
	if cors {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}
	if hsts {
		w.Header().Set("Strict-Transport-Security", "max-age=15552000")
	}
	if redirectHttps && r.TLS == nil && r.Host != "" {
		targ := &*r.URL
		targ.Host = r.Host
		targ.Scheme = "https"
		http.Redirect(w, r, targ.String(), http.StatusFound)
		return
	}
	if r.Method == http.MethodGet || r.Method == http.MethodPost || r.Method == http.MethodHead {
		for _, host := range strings.Split(uploadHost, ",") {
			if r.Host == host {
				handleFile(w, r)
				return
			}
		}
		http.DefaultServeMux.ServeHTTP(w, r)
	} else {
		w.Header().Set("Allow", "POST, HEAD, OPTIONS, GET")
		if r.Method != http.MethodOptions {
			http.Error(w, "The method is not allowed for the requested URL.", http.StatusMethodNotAllowed)
		}
	}
}

func main() {
	flag.StringVar(&uploadUrl, "upload-url", "", "URL to serve uploads from")
	flag.StringVar(&uploadHost, "upload-host", "", "comma-separated list of hosts to serve uploads on")
	flag.StringVar(&siteName, "name", "Gomf", "website name")
	flag.StringVar(&contactMail, "contact", "contact@example.com", "contact email address")
	flag.StringVar(&abuseMail, "abuse", "abuse@example.com", "abuse email address")
	flag.StringVar(&csp, "csp", "default-src 'none'; media-src 'self'", "the Content-Security-Policy header for files; blank to disable")
	flag.BoolVar(&hsts, "hsts", false, "enable HSTS")
	flag.BoolVar(&allowHtml, "allow-html", false, "serve (X)HTML uploads with (X)HTML filetypes")
	flag.BoolVar(&cors, "cors", false, "enable CORS and allow all origins")
	flag.BoolVar(&redirectHttps, "redirect-https", false, "redirect HTTP traffic to HTTPS")
	listenHttp := flag.String("http", "localhost:8080", "address to listen on for HTTP")
	listenHttps := flag.String("https", "", "address to listen on for HTTPS")
	cert := flag.String("cert", "", "path to TLS certificate (for HTTPS)")
	key := flag.String("key", "", "path to TLS key (for HTTPS)")
	maxSize := flag.Int64("max-size", DefaultMaxSize, "max filesize in bytes")
	filterMime := flag.String("filter-mime", "application/x-dosexec,application/x-msdos-program", "comma-separated list of filtered MIME types")
	filterExt := flag.String("filter-ext", "exe,dll,msi,scr,com,pif", "comma-separated list of filtered file extensions")
	whitelist := flag.Bool("whitelist", false, "use filter as a whitelist instead of blacklist")
	grill := flag.Bool("grill", false, "enable grills")
	idLength := flag.Int("id-length", DefaultIdLength, "length of uploaded file IDs")
	idCharset := flag.String("id-charset", "", "charset for uploaded file IDs (default lowercase letters a-z)")
	enableLog := flag.Bool("log", false, "enable logging")
	logIP := flag.Bool("log-ip", false, "log IP addresses")
	logIPHash := flag.Bool("log-ip-hash", false, "log hashed IP addresses")
	logUA := flag.Bool("log-ua", false, "log User-Agent headers")
	logUAHash := flag.Bool("log-ua-hash", false, "log hashed User-Agent headers")
	logReferer := flag.Bool("log-referer", false, "log Referer headers")
	logRefererHash := flag.Bool("log-referer-hash", false, "log hashed Referer headers")
	logHashSalt := flag.String("log-hash-salt", "", "salt to use for hashed log entries")

	flag.Parse()

	rand.Seed(time.Now().UnixNano())

	initWebsite()

	storage = NewStorage("upload")
	storage.FilterExt = strings.Split(*filterExt, ",")
	for i := range storage.FilterExt {
		storage.FilterExt[i] = "." + storage.FilterExt[i]
	}
	storage.FilterMime = strings.Split(*filterMime, ",")
	storage.Whitelist = *whitelist
	storage.IdLength = *idLength
	storage.MaxSize = *maxSize
	if *idCharset != "" {
		storage.IdCharset = *idCharset
	}

	if !*enableLog {
		DefaultLogger = nil
	} else {
		DefaultLogger.LogIP = *logIP || *logIPHash
		DefaultLogger.LogUserAgent = *logUA || *logUAHash
		DefaultLogger.LogReferer = *logReferer || *logRefererHash
		DefaultLogger.HashIP = *logIPHash
		DefaultLogger.HashUserAgent = *logUAHash
		DefaultLogger.HashReferer = *logRefererHash
		DefaultLogger.HashSalt = *logHashSalt
	}

	http.HandleFunc("/upload.php", handleUpload)
	http.Handle("/u/", http.StripPrefix("/u/", http.HandlerFunc(handleFile)))
	if *grill {
		http.HandleFunc("/grill.php", handleGrill)
	}

	if uploadUrl == "" {
		if uploadHost != "" {
			host := strings.Split(uploadHost, ",")[0]
			if *listenHttps != "" {
				uploadUrl = "https://" + host + "/"
			} else if *listenHttp != "" {
				uploadUrl = "http://" + host + "/"
			}
		} else {
			if *listenHttps != "" {
				uploadUrl = "https://" + *listenHttps + "/u/"
			} else if *listenHttp != "" {
				uploadUrl = "http://" + *listenHttp + "/u/"
			}
		}
		fmt.Printf("using %q as uploaded file URL\n", uploadUrl)
	}

	exit := true
	if *listenHttp != "" {
		exit = false
		fmt.Printf("listening on http://%s/\n", *listenHttp)
		go func() {
			panic(http.ListenAndServe(*listenHttp, http.HandlerFunc(handle)))
		}()
	}
	if *listenHttps != "" {
		exit = false
		fmt.Printf("listening on https://%s/\n", *listenHttps)
		go func() {
			panic(http.ListenAndServeTLS(*listenHttps, *cert, *key, http.HandlerFunc(handle)))
		}()
	}

	if !exit {
		select {}
	}
}
