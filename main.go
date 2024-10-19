package main

import (
	"errors"
	"fmt"
	loadBalancerError "github.com/ayushs-2k4/go-load-balancer/error"
	"github.com/ayushs-2k4/go-load-balancer/loadbalancer"
	"io"
	"kontest-api-gateway/Auth"
	"kontest-api-gateway/utils"
	"log"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
)

// Route defines a mapping from a path prefix to a backend service
type Route struct {
	Path    string
	Backend string
}

// APIGateway holds the routing information and the Consul configuration.
type APIGateway struct {
	Routes     []Route // List of routes
	ConsulHost string  // Consul host used for load balancing
	ConsulPort int     // Consul port used for load balancing
}

// NewAPIGateway initializes a new API gateway with predefined routes
func NewAPIGateway(consulHost string, consulPort int) *APIGateway {
	return &APIGateway{
		Routes:     []Route{},
		ConsulHost: consulHost,
		ConsulPort: consulPort,
	}
}

func (g *APIGateway) registerRoute(route Route) {
	g.Routes = append(g.Routes, route)
	log.Printf("Route registered: Path=%s, Backend=%s\n", route.Path, route.Backend)
}

func (g *APIGateway) registerRoutes(routes []Route) {
	for _, route := range routes {
		g.registerRoute(route)
	}
}

// PathMatches ensures that the request path starts with the route prefix and is followed by '/' or nothing
func PathMatches(routePath, requestPath string) bool {
	if requestPath == routePath {
		return true
	}
	if strings.HasPrefix(requestPath, routePath) && (len(requestPath) == len(routePath) || requestPath[len(routePath)] == '/') {
		return true
	}
	return false
}

func doesContainInappropriateHeaders(r *http.Request) (bool, error) {
	inappropriateHeaders := [...]string{"user-id"}
	inappropriateValues := [...]string{"hack"}

	// Convert inappropriateHeaders to lowercase
	for i := range inappropriateHeaders {
		inappropriateHeaders[i] = strings.ToLower(inappropriateHeaders[i])
	}

	// Check headers
	for header, values := range r.Header {
		// Convert header to lowercase for case-insensitive comparison
		lowerHeader := strings.ToLower(header)

		if slices.Contains(inappropriateHeaders[:], lowerHeader) {
			return true, errors.New("inappropriate headers found")
		}

		// Check values for each header
		for _, value := range values {
			// Convert value to lowercase for case-insensitive comparison
			if slices.Contains(inappropriateValues[:], strings.ToLower(value)) {
				return true, errors.New("inappropriate values found")
			}
		}
	}

	return false, nil
}

func isRequestInvalid(r *http.Request) (bool, error) {
	return doesContainInappropriateHeaders(r)
}

// FindBackend finds the backend service for a given request path
func (g *APIGateway) FindBackend(path string) (string, string, error) {
	log.Printf("Finding backend for path: %s\n", path)
	for _, route := range g.Routes {
		if PathMatches(route.Path, path) {
			if strings.HasPrefix(route.Backend, "lb://") {
				// Load balance the request to the backend service
				serviceName := strings.TrimPrefix(route.Backend, "lb://")
				log.Printf("Using load balancer for service: %s\n", serviceName)

				// Get the load balancer for the service
				lb, err := loadbalancer.GetLoadBalancer(serviceName, g.ConsulHost, g.ConsulPort)
				if err != nil {
					log.Printf("Error getting load balancer for service: %s, Error: %s\n", serviceName, err)
					return "", "", err
				}

				// Get the healthy instances of the service
				instance, err := lb.ChooseInstance()
				if err != nil {
					log.Printf("Error choosing instance for service: %s, Error: %s\n", serviceName, err)
					return "", "", err
				}

				log.Printf("Forwarding to instance: %s:%d\n", instance.Address, instance.Port)
				return "http://" + instance.Address + ":" + strconv.Itoa(instance.Port), strings.TrimPrefix(path, route.Path), nil
			} else {
				// Direct backend found
				log.Printf("Forwarding to backend: %s\n", route.Backend)
				return route.Backend, strings.TrimPrefix(path, route.Path), nil
			}
		}
	}
	return "", "", errors.New("no matching backend found")
}

// ForwardRequest forwards the incoming HTTP request to the appropriate backend
func (g *APIGateway) ForwardRequest(w http.ResponseWriter, r *http.Request) {
	log.Printf("Incoming request: %s %s\n", r.Method, r.URL.Path)

	// Check if it contains inappropriate headers

	isRequestInvalid, err := isRequestInvalid(r)

	if err != nil {
		http.Error(w, fmt.Sprintf("%s", err), http.StatusBadRequest)
		return
	}

	if isRequestInvalid {
		http.Error(w, "Request is invalid", http.StatusBadRequest)
		return
	}

	// Find the appropriate backend based on the request path
	backend, newPath, err := g.FindBackend(r.URL.Path)
	if err != nil {
		var noHealthyInstanceAvailableError *loadBalancerError.NoHealthyInstanceAvailableError
		switch {
		case errors.As(err, &noHealthyInstanceAvailableError):
			// Handle the specific error here
			http.Error(w, "Currently service not available", http.StatusInternalServerError)
			// Optionally, you can access e.ServiceName() if needed
		default:
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}

		log.Printf("Error finding backend: %s\n", err)
		return
	}

	// Create the new URL for the backend service
	backendURL, err := url.Parse(backend)
	if err != nil {
		log.Printf("Invalid backend URL: %s, Error: %s\n", backend, err)
		http.Error(w, "Invalid backend URL", http.StatusInternalServerError)
		return
	}

	// Construct the full URL for the backend request
	backendURL.Path = newPath
	backendURL.RawQuery = r.URL.RawQuery
	log.Printf("Forwarding request to: %s\n", backendURL.String())

	// Create a new request to the backend service
	req, err := http.NewRequest(r.Method, backendURL.String(), r.Body)
	if err != nil {
		log.Printf("Error creating new request to backend: %s\n", err)
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}

	// get user-id from jwt (if present)
	tokenString := r.Header.Get("Authorization")
	if tokenString == "" {
		// no auth header present
	} else {
		// Validate the JWT
		claims, err := Auth.ValidateJWT(tokenString, []byte(Auth.JWTSecret))
		if err != nil || claims == nil {
			http.Error(w, "Invalid JWT", http.StatusUnauthorized)
			return
		}

		// add it to the request header
		req.Header.Add(utils.UserIdRequestHeader, claims.Subject)
	}

	// Copy the headers from the original request except Authorization
	for header, values := range r.Header {
		if header == "Authorization" {
			continue
		}

		for _, value := range values {
			req.Header.Add(header, value)
		}
	}

	// Forward the request to the backend service
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error forwarding request to backend: %s\n", err)
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
	log.Printf("Request forwarded with status: %d\n", resp.StatusCode)
}

func main() {
	//testingSecurity()

	var consulHost = "localhost"
	var consulPort = 5150

	// get the consul host and port from the environment variables
	if host := os.Getenv("CONSUL_HOST"); host != "" {
		consulHost = host
	}

	if port := os.Getenv("CONSUL_PORT"); port != "" {
		consulPort, _ = strconv.Atoi(port)
	}

	// Initialize the API Gateway
	gateway := NewAPIGateway(consulHost, consulPort)

	// Register routes for the API Gateway
	gateway.registerRoutes(
		[]Route{
			{Path: "/kontest", Backend: "lb://KONTEST-API"},
			{Path: "/user-stats", Backend: "lb://KONTEST-USER-STATS-SERVICE"},
			{Path: "/auth", Backend: "lb://KONTEST-AUTHENTICATION-SERVICE"},
			{Path: "/user", Backend: "lb://KONTEST-USER-SERVICE"},
		},
	)

	router := http.NewServeMux()

	// Define the main route to forward all requests through the API Gateway
	router.HandleFunc("/", gateway.ForwardRequest)

	// Start the API Gateway server on port 5153
	log.Println("API Gateway listening on port 5153...")
	err := http.ListenAndServe(":5153", router)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
