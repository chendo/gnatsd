package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// Routez represents detail information on current routes
type Routez struct {
	NumRoutes int          `json:"num_routes"`
	Routes    []*RouteInfo `json:"routes"`
}

// RouteInfo has detailed information on a per connection basis.
type RouteInfo struct {
	Cid       uint64 `json:"cid"`
	URL       string `json:"url"`
	IP        string `json:"ip"`
	Port      int    `json:"port"`
	Solicited bool   `json:"solicited"`
	Subs      uint32 `json:"subscriptions"`
	Pending   int    `json:"pending_size"`
	InMsgs    int64  `json:"in_msgs"`
	OutMsgs   int64  `json:"out_msgs"`
	InBytes   int64  `json:"in_bytes"`
	OutBytes  int64  `json:"out_bytes"`
}

// HandleConnz process HTTP requests for connection information.
func (s *Server) HandleRoutez(w http.ResponseWriter, req *http.Request) {

	if req.Method == "GET" {
		r := Routez{Routes: []*RouteInfo{}}

		// Walk the list
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, route := range s.routes {
			ri := &RouteInfo{
				Cid:       route.cid,
				Subs:      route.subs.Count(),
				Solicited: route.route.didSolicit,
				InMsgs:    route.inMsgs,
				OutMsgs:   route.outMsgs,
				InBytes:   route.inBytes,
				OutBytes:  route.outBytes,
			}

			if route.route.url != nil {
				ri.URL = route.route.url.String()
			}

			if ip, ok := route.nc.(*net.TCPConn); ok {
				addr := ip.RemoteAddr().(*net.TCPAddr)
				ri.Port = addr.Port
				ri.IP = addr.IP.String()
			}
			r.Routes = append(r.Routes, ri)
		}

		r.NumRoutes = len(r.Routes)
		b, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			Errorf("Error marshalling response to /routez request: %v", err)
		}
		w.Write(b)
	} else if req.Method == "PUT" {
		body := make([]byte, 1024)
		req.Body.Read(body)
		routeURL, err := url.Parse(strings.Trim(string(body), "\x00"))
		if err != nil {
			w.WriteHeader(400)
			w.Write([]byte(fmt.Sprintf(`{"error": "could not parse URL: %v"}`, err)))
			return
		}

		err = s.connectToRouteOnce(routeURL)
		if err == nil {
			w.Write([]byte(`{"status": "ok"}`))
		} else {
			w.Write([]byte(fmt.Sprintf(`{"error": "could not connect: %v"}`, err)))
		}
	} else if req.Method == "DELETE" {
		body := make([]byte, 1024)
		req.Body.Read(body)
		url := strings.Trim(string(body), "\x00")

		s.mu.Lock()
		var routeToDelete *client
		for _, route := range s.routes {
			if route.route.url != nil && route.route.url.String() == url {
				routeToDelete = route
				break
			}
		}
		s.mu.Unlock()

		if routeToDelete != nil {
			routeToDelete.mu.Lock()
			routeToDelete.route.didSolicit = false // don't reconnect
			routeToDelete.mu.Unlock()
			routeToDelete.closeConnection()
			w.WriteHeader(200)
			w.Write([]byte(`{"status": "ok"}`))
		} else {
			w.WriteHeader(404)
			w.Write([]byte(`{"error": "could not find matching route"}`))
		}
	}
}
