package main

import (
	"net/http"
	"reflect"

	autoneg "bitbucket.org/ww/goautoneg"
)

type negotiator struct {
	contentType string
	http.Handler
}

// match compares two parsed media types for compatibility.
func match(a, b autoneg.Accept) bool {
	type contentType struct{ a, b string }

	if (contentType{a.Type, a.SubType}) == (contentType{b.Type, b.SubType}) {
		return reflect.DeepEqual(a.Params, b.Params)
	}

	switch (contentType{b.Type, b.SubType}) {
	case contentType{a.Type, "*"}:
		return true
	case contentType{"*", "*"}:
		return true
	}

	return false
}

// negotiate accepts a slice of negotiators and returns http.Handler which
// negotiates based on the Accept header.
func negotiate(handlers []negotiator) http.Handler {
	alternatives := make([]autoneg.Accept, 0, len(handlers))

	for _, neg := range handlers {
		accepts := autoneg.ParseAccept(neg.contentType)

		alternatives = append(alternatives, accepts[0])
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, accept := range autoneg.ParseAccept(r.Header.Get("Accept")) {
			for i, alternative := range alternatives {
				if match(alternative, accept) {
					handlers[i].ServeHTTP(w, r)
					return
				}
			}
		}

		http.Error(w, "", http.StatusNotAcceptable)
	})
}
