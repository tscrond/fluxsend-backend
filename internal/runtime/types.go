package runtime

import "net/http"

type NamedHTTPServer struct {
	Name string
	Srv  *http.Server
}
