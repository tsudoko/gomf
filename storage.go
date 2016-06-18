package main

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"io/ioutil"
	"math/rand"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	MaxIdTries = 64

	DefaultIdCharset = "abcdefghijklmnopqrstuvwxyz"
	DefaultIdLength  = 6
	DefaultMaxSize   = 50 * 1024 * 1024
)

type Storage struct {
	Folder     string
	IdCharset  string
	IdLength   int
	MaxSize    int64
	FilterMime []string
	FilterExt  []string
	Whitelist  bool
}

type ErrForbidden struct{ Type string }

func (e ErrForbidden) Error() string { return "forbidden type: " + e.Type }

type ErrTooLarge struct{ Size int64 }

func (e ErrTooLarge) Error() string {
	return "file exceeds maximum allowed size of " + strconv.FormatInt(e.Size, 10) + " bytes"
}

type ErrNotFound struct{ Name string }

func (e ErrNotFound) Error() string { return "file " + e.Name + " not found" }

func NewStorage(folder string, maxSize int64) *Storage {
	if err := os.MkdirAll(path.Join(folder, "temp"), 0755); err != nil {
		panic(err)
	}
	if err := os.MkdirAll(path.Join(folder, "files"), 0755); err != nil {
		panic(err)
	}
	if err := os.MkdirAll(path.Join(folder, "ids"), 0755); err != nil {
		panic(err)
	}

	return &Storage{
		Folder:    folder,
		IdCharset: DefaultIdCharset,
		IdLength:  DefaultIdLength,
		MaxSize:   maxSize,
	}
}

func (s *Storage) Get(id string) (file *os.File, hash string, size int64, modtime time.Time, err error) {
	ext := path.Ext(id)
	id = id[:len(id)-len(ext)]
	for i := 0; i < len(id); i++ {
		if !strings.ContainsRune(s.IdCharset, rune(id[i])) {
			err = errors.New("invalid ID: " + id)
			return
		}
	}
	folder := s.idToFolder("ids", id)
	files, err := ioutil.ReadDir(folder)
	if err != nil {
		err = ErrNotFound{id + ext}
		return
	}
	if len(files) < 1 {
		err = errors.New("internal storage error")
		return
	}
	fn := files[0].Name()
	fp := path.Join(folder, fn)
	target, err := os.Readlink(fp)
	if err != nil {
		return
	}
	bhash, err := base64.RawURLEncoding.DecodeString(path.Base(path.Dir(target)))
	if err != nil {
		return
	}
	hash = hex.EncodeToString(bhash)
	if path.Ext(fn) != ext {
		err = ErrNotFound{id + ext}
		return
	}
	stat, err := os.Lstat(fp)
	if err != nil {
		return
	}
	modtime = stat.ModTime()
	size = stat.Size()
	file, err = os.Open(fp)
	return
}

var errFileExists = errors.New("file exists")

func (s *Storage) New(r io.Reader, name string) (id, hash string, size int64, err error) {
	temp, err := ioutil.TempFile(path.Join(s.Folder, "temp"), "file")
	if err != nil {
		return
	}
	defer func() {
		if temp != nil {
			temp.Close()
			os.Remove(temp.Name())
		}
	}()

	hash, size, err = s.readInput(temp, r)
	if err != nil {
		return
	}
	_, ext, err := s.getMimeExt(temp.Name(), name)
	if err != nil {
		return
	}
	id, err = s.storeFile(temp, hash, ext)
	if err == nil {
		temp = nil // prevent deletion
	} else if err == errFileExists {
		err = nil
	}

	return
}

func (s *Storage) randomId() string {
	id := make([]byte, s.IdLength)
	for i := 0; i < len(id); i++ {
		id[i] = s.IdCharset[rand.Intn(len(s.IdCharset))]
	}
	return string(id)
}

func (s *Storage) idToFolder(subfolder, id string) string {
	for len(id) < 4 {
		id = "_" + id
	}
	return path.Join(s.Folder, subfolder, id[0:1], id[1:3], id)
}

func (s *Storage) readInput(w io.Writer, r io.Reader) (hash string, size int64, err error) {
	h := sha1.New()
	w = io.MultiWriter(h, w)
	if s.MaxSize > 0 {
		r = io.LimitReader(r, s.MaxSize+1)
	}
	size, err = io.Copy(w, r)
	if err != nil {
		return
	}
	if lr, ok := r.(*io.LimitedReader); ok && lr.N == 0 {
		err = ErrTooLarge{s.MaxSize}
		return
	}
	hash = base64.RawURLEncoding.EncodeToString(h.Sum(nil))
	return
}

func (s *Storage) getMimeExt(fpath string, name string) (mimetype, ext string, err error) {
	mimetype, err = GetMimeType(fpath)
	if err != nil {
		return
	}

	// choose file extension, prefer the user-provided one
	ext = path.Ext(name)
	exts, err := mime.ExtensionsByType(mimetype)
	valid := false
	if err != nil {
		return
	}
	if ext != "" {
		for _, e := range exts {
			if e == ext {
				valid = true
				break
			}
		}
	}
	if !valid {
		ext = ""
		if len(exts) > 0 {
			ext = exts[0]
		}
	}

	filtered, ok := s.findFilter(exts, mimetype)
	if !ok && s.Whitelist { // whitelist: reject if not on filters
		err = ErrForbidden{mimetype}
	} else if ok && !s.Whitelist { // blacklist: reject if filtered
		forbid := true
		// only block application/octet-stream if explicitly requested
		if mimetype == "application/octet-stream" {
			forbid = false
			for _, fm := range s.FilterMime {
				if mimetype == fm {
					forbid = true
					break
				}
			}
		}
		if forbid {
			err = ErrForbidden{filtered}
		}
	}

	return
}

func (s *Storage) findFilter(exts []string, mimetype string) (match string, ok bool) {
	for _, fm := range s.FilterMime {
		if mimetype == fm {
			return mimetype, true
		}
	}
	for _, ext := range exts {
		for _, fe := range s.FilterExt {
			if ext == "."+fe {
				return ext, true
			}
		}
	}
	return "", false
}

func (s *Storage) storeFile(file *os.File, hash, ext string) (id string, err error) {
	hfolder := s.idToFolder("files", hash)
	hpath := path.Join(hfolder, "file")
	fexists := false

	os.MkdirAll(path.Dir(hfolder), 0755)
	err = os.Mkdir(hfolder, 0755)
	if err != nil {
		if _, err = os.Stat(hpath); err != nil {
			err = errors.New("internal storage error")
		}
		fexists = true
	} else {
		err = os.Rename(file.Name(), hpath)
		os.Chmod(hpath, 0644)
	}
	if err != nil {
		return
	}

	fpath := ""
	for i := 0; i < MaxIdTries; i++ {
		id = s.randomId()
		dir := s.idToFolder("ids", id)
		os.MkdirAll(path.Dir(dir), 0755)
		err = os.Mkdir(dir, 0755)
		if err == nil {
			fpath = path.Join(dir, "file"+ext)
			id += ext
			break
		}
	}
	if fpath == "" {
		err = errors.New("internal storage error")
		return
	}
	rhpath, err := filepath.Rel(path.Dir(fpath), hpath)
	if err != nil {
		return
	}
	err = os.Symlink(rhpath, fpath)

	if fexists && err == nil {
		err = errFileExists
	}
	return
}
