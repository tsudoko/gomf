package main

import (
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"mime"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

func handleGrill(w http.ResponseWriter, r *http.Request) {
	grills, err := ioutil.ReadDir("static/grill/")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/static/grill/"+grills[rand.Intn(len(grills))].Name(), http.StatusFound)
}

func handleFile(w http.ResponseWriter, r *http.Request) {
	f, hash, size, modtime, err := storage.Get(strings.TrimRight(r.URL.Path, "/"))
	if err != nil {
		if _, ok := err.(ErrNotFound); ok {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	defer f.Close()
	mtype := mime.TypeByExtension(path.Ext(f.Name()))
	if strings.Index(mtype, "text/html") == 0 || strings.Index(mtype, "application/xhtml+xml") == 0 {
		mtype = "text/plain"
	}
	w.Header().Set("Content-Type", mtype)
	_ = size
	//w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.Header().Set("Content-Security-Policy", "default-src 'none'; media-src 'self'")
	w.Header().Set("Last-Modified", modtime.UTC().Format(http.TimeFormat))
	w.Header().Set("Expires", modtime.UTC().Add(time.Hour*24*30).Format(http.TimeFormat))
	w.Header().Set("Cache-Control", "max-age=2592000")
	w.Header().Set("ETag", "\"sha1:"+hash+"\"")
	//io.Copy(w, f)
	http.ServeContent(w, r, "", modtime, f)
}

type result struct {
	Url  string `json:"url"`
	Name string `json:"name"`
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

type response struct {
	Success     bool     `json:"success"`
	ErrorCode   int      `json:"errorcode,omitempty"`
	Description string   `json:"description,omitempty"`
	Files       []result `json:"files,omitempty"`
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	output := r.FormValue("output")
	resp := response{Files: []result{}}

	if r.Method == "GET" && output == "html" {
		respond(w, output, resp)
		return
	}

	mr, err := r.MultipartReader()
	if err != nil {
		resp.ErrorCode = http.StatusInternalServerError
		resp.Description = err.Error()
		respond(w, output, resp)
		return
	}

	for {
		part, err := mr.NextPart()
		if err != nil {
			if err != io.EOF {
				resp.ErrorCode = http.StatusInternalServerError
				resp.Description = err.Error()
			}
			break
		}

		if part.FormName() != "files[]" {
			continue
		}

		id, hash, size, err := storage.New(part, part.FileName())
		if err != nil {
			resp.ErrorCode = http.StatusInternalServerError
			resp.Description = err.Error()
			if _, ok := err.(ErrTooLarge); ok {
				resp.ErrorCode = http.StatusRequestEntityTooLarge
			} else if _, ok := err.(ErrForbidden); ok {
				resp.ErrorCode = http.StatusForbidden
			}
			break
		}

		bhash, _ := base64.RawURLEncoding.DecodeString(hash)
		res := result{
			Name: part.FileName(),
			Url:  strings.TrimRight(uploadUrl, "/") + "/" + id,
			Hash: hex.EncodeToString(bhash),
			Size: size,
		}
		resp.Files = append(resp.Files, res)

		part.Close()
	}

	respond(w, output, resp)
}

func respond(w http.ResponseWriter, mode string, resp response) {
	if resp.ErrorCode != 0 {
		resp.Files = []result{}
		resp.Success = false
	} else {
		resp.Success = true
	}

	code := http.StatusOK
	if resp.ErrorCode != 0 {
		code = resp.ErrorCode
	}
	w.WriteHeader(code)

	switch mode {
	case "json":
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)

	case "text", "gyazo":
		w.Header().Set("Content-Type", "text/plain")
		sep := ""
		for _, file := range resp.Files {
			io.WriteString(w, sep+file.Url)
			sep = "\n"
		}
		if mode != "gyazo" {
			io.WriteString(w, "\n")
		}

	case "csv":
		w.Header().Set("Content-Type", "text/csv")
		wr := csv.NewWriter(w)
		wr.Write([]string{"name", "url", "hash", "size"})
		for _, file := range resp.Files {
			wr.Write([]string{file.Name, file.Url, file.Hash, strconv.FormatInt(file.Size, 10)})
		}
		wr.Flush()

	case "", "html":
		w.Header().Set("Content-Type", "text/html")
		context := newContext()
		context.Result = resp
		if err := templates.ExecuteTemplate(w, "index.html", context); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}

	default:
		respond(w, "", response{ErrorCode: http.StatusNotFound, Description: "invalid output mode " + mode})
		return
	}
}