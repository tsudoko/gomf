package main

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path"
	"sync"
	"time"
)

type Logger struct {
	LogDir        string
	LogIP         bool
	LogUserAgent  bool
	LogReferer    bool
	HashIP        bool
	HashUserAgent bool
	HashReferer   bool
	HashSalt      string
	logFile       *os.File
	encoder       *json.Encoder
	lastDate      string
	lock          sync.Mutex
}

type LogEntry map[string]interface{}

func InitLogger(logdir string) *Logger {
	return &Logger{
		LogDir: logdir,
	}
}

func (l *Logger) Log(entry LogEntry) {
	l.lock.Lock()
	defer l.lock.Unlock()
	_, err := l.getLogFile()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening log file: %s\n", err)
		return
	}
	err = l.encoder.Encode(entry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error writing to log: %s\n", err)
	}
}

func (l *Logger) LogUpload(req *http.Request, res result) {
	host, _, _ := net.SplitHostPort(req.RemoteAddr)
	l.logUpload(
		host,               // ip
		req.UserAgent(),    // userAgent
		req.Referer(),      // referer
		res.Name,           // origName
		path.Base(res.Url), // idext
		res.Hash,           // hash
		res.Size,           // size
	)
}

func (l *Logger) logUpload(ip, userAgent, referer, origName, idext, hash string, size int64) {
	if !l.LogIP {
		ip = ""
	} else if l.HashIP {
		ip = l.hash(ip)
	}
	if !l.LogUserAgent {
		userAgent = ""
	} else if l.HashUserAgent {
		userAgent = l.hash(userAgent)
	}
	if !l.LogReferer {
		referer = ""
	} else if l.HashReferer {
		referer = l.hash(referer)
	}
	l.Log(LogEntry{
		"type":       "upload",
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"ip":         ip,
		"user_agent": userAgent,
		"referer":    referer,
		"orig_name":  origName,
		"id":         idext,
		"hash":       hash,
		"size":       size,
	})
}

func (l *Logger) hash(s string) string {
	h := sha1.New()
	h.Write([]byte(l.HashSalt))
	h.Write([]byte(s))
	h.Write([]byte(l.HashSalt))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

func (l *Logger) getLogFile() (*os.File, error) {
	if l.lastDate == "" {
		if err := os.MkdirAll(l.LogDir, 0755); err != nil {
			return nil, err
		}
	}
	currentDate := time.Now().UTC().Format("2006-01-02")
	if l.lastDate == currentDate {
		return l.logFile, nil
	}
	f, err := os.OpenFile(path.Join(l.LogDir, currentDate+".log.json"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return f, err
	}
	if l.logFile != nil {
		l.logFile.Close()
	}
	l.lastDate = currentDate
	l.logFile = f
	l.encoder = json.NewEncoder(f)
	return f, nil
}

var DefaultLogger = InitLogger("log")

func Log(entry LogEntry) {
	if DefaultLogger != nil {
		DefaultLogger.Log(entry)
	}
}

func LogUpload(req *http.Request, res result) {
	if DefaultLogger != nil {
		DefaultLogger.LogUpload(req, res)
	}
}
