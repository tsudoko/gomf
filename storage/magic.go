package storage

// #cgo LDFLAGS: -lmagic
// #include <stdlib.h>
// #include <magic.h>
import "C"
import "errors"
import "unsafe"

var magic C.magic_t

func init() {
	magic = C.magic_open(C.MAGIC_MIME_TYPE | C.MAGIC_SYMLINK | C.MAGIC_ERROR)
	if magic == nil {
		panic("unable to initialize libmagic")
	}
	if C.magic_load(magic, nil) != 0 {
		C.magic_close(magic)
		panic("unable to load libmagic database: " + C.GoString(C.magic_error(magic)))
	}
}

func GetMimeType(fname string) (string, error) {
	cfname := C.CString(fname)
	defer C.free(unsafe.Pointer(cfname))
	mime := C.magic_file(magic, cfname)
	if mime == nil {
		return "", errors.New(C.GoString(C.magic_error(magic)))
	}
	return C.GoString(mime), nil
}
