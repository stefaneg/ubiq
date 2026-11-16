// Package person tells who a Person is: the human ubiq acts for, with a name, an email and a Role.
package person

import (
	"net/http"
)

// Role is what a Person may do: read, or also make Changes, or, in stdio, everything a holder of the working tree
// may.
type Role string

const (
	Reader     Role = "Reader"
	Writer     Role = "Writer"
	Maintainer Role = "Maintainer"
)

// Person is a human ubiq acts for. Name and Email are empty when not known.
type Person struct {
	Subject string
	Name    string
	Email   string
	Role    Role
}

// Resolver learns the Person behind a request from its headers, which are nil in stdio. It returns an error, one
// sentence, when it cannot tell, and never guesses.
type Resolver interface {
	Resolve(header http.Header) (Person, error)
}
