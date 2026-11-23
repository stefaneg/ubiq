package person

import "net/http"

// Fixed resolves every request to one Person, a Writer, without looking at the request: the Server's dev direct mode,
// offered on loopback only.
func Fixed(name, email string) Resolver {
	return fixed(Person{Name: name, Email: email, Role: Writer})
}

type fixed Person

func (p fixed) Resolve(http.Header) (Person, error) { return Person(p), nil }
