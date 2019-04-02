// Copyright 2019 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
