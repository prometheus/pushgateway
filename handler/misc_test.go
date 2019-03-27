package handler

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

type fakeFileSystem struct {
	files map[string]struct{}
}

// Open implements the http.FileSystem interface
//
// If a file is present, no error will be returned.
// This implementation always returns a nil File.
func (f *fakeFileSystem) Open(name string) (http.File, error) {
	log.Println("requesting" + name)

	if _, ok := f.files[name]; !ok {
		return nil, os.ErrNotExist
	}
	return os.Open("misc_test.go")
	//	return nil, nil
}

func TestRoutePrefixForStatic(t *testing.T) {
	fs := &fakeFileSystem{map[string]struct{}{
		"/index.js": struct{}{},
	}}

	for _, test := range []struct {
		prefix string
		path   string
		code   int
	}{
		{"/", "/index.js", 200},
		{"/", "/missing.js", 404},
		{"/route-prefix", "/index.js", 200},
		{"/route-prefix", "/missing.js", 404},
	} {
		test := test
		t.Run(fmt.Sprintf("%v", test), func(t *testing.T) {
			t.Parallel()
			req, err := http.NewRequest(
				http.MethodGet, "http://example.com"+test.prefix+test.path, nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			static := Static(fs, test.prefix)
			static.ServeHTTP(w, req)
			if test.code != w.Code {
				t.Errorf("Wanted %d, got %d.", test.code, w.Code)
			}
		})
	}
}
