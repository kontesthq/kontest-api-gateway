package main

import (
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

// Route defines a mapping from a path prefix to a backend service
type Route struct {
	Path    string
	Backend string
}

// APIGateway holds the routing information
type APIGateway struct {
	Routes []Route
}

// NewAPIGateway initializes a new API gateway with predefined routes
func NewAPIGateway() *APIGateway {
	return &APIGateway{
		Routes: []Route{
			{Path: "/kontest", Backend: "http://localhost:5151"},
			// Add more services here
		},
	}
}

// FindBackend finds the backend service for a given request path
func (g *APIGateway) FindBackend(path string) (string, string) {
	for _, route := range g.Routes {
		if strings.HasPrefix(path, route.Path) {
			// Return the backend URL and the new path after the prefix
			return route.Backend, strings.TrimPrefix(path, route.Path)
		}
	}
	return "", ""
}

// ForwardRequest forwards the incoming HTTP request to the appropriate backend
func (g *APIGateway) ForwardRequest(w http.ResponseWriter, r *http.Request) {
	// Find the appropriate backend based on the request path
	backend, newPath := g.FindBackend(r.URL.Path)
	if backend == "" {
		http.Error(w, "No matching backend found", http.StatusNotFound)
		return
	}

	// Create the new URL for the backend service
	backendURL, err := url.Parse(backend)
	if err != nil {
		http.Error(w, "Invalid backend URL", http.StatusInternalServerError)
		return
	}

	// Construct the full URL for the backend request
	backendURL.Path = newPath
	backendURL.RawQuery = r.URL.RawQuery

	// Create a new request to the backend service
	req, err := http.NewRequest(r.Method, backendURL.String(), r.Body)
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}

	// Copy the headers from the original request
	for header, values := range r.Header {
		for _, value := range values {
			req.Header.Add(header, value)
		}
	}

	// Forward the request to the backend service
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Failed to forward request to backend", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy the response headers from the backend to the client
	for header, values := range resp.Header {
		for _, value := range values {
			w.Header().Set(header, value)
		}
	}

	// Write the response status and body
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func main() {
	// Initialize the API Gateway
	gateway := NewAPIGateway()

	// Define the main route to forward all requests through the API Gateway
	http.HandleFunc("/", gateway.ForwardRequest)

	// Start the API Gateway server on port 8080
	log.Println("API Gateway listening on port 5153...")
	err := http.ListenAndServe(":5153", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
