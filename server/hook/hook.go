package hook

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/alipourhabibi/Hades/utils/log"
)

type Server struct {
	logger *log.LoggerWrapper
}

func NewServer(l *log.LoggerWrapper) *Server {
	return &Server{
		logger: l,
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/v4/internal/check":
		s.check(w, r)
	case "/api/v4/internal/allowed":
		s.allowed(w, r)
	case "/api/v4/internal/pre_receive":
		s.pre(w, r)
	case "/api/v4/internal/post_receive":
		s.post(w, r)
	default:
		http.NotFound(w, r)
	}
}

type HealthCheckResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type allowedResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
}

type postReceiveResponse struct {
	ReferenceCounterDecreased bool                 `json:"reference_counter_decreased"`
	Messages                  []PostReceiveMessage `json:"messages"`
}

type PostReceiveMessage struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

func (s *Server) check(w http.ResponseWriter, _ *http.Request) {

	response := HealthCheckResponse{
		Status:  "running",
		Message: "Service is running",
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8; version=1.0")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (s *Server) allowed(w http.ResponseWriter, _ *http.Request) {
	response := allowedResponse{
		Status:  true,
		Message: "Service is running",
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8; version=1.0")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (s *Server) pre(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close() // Always close the body

	fmt.Println("Received Body:", string(body)) // Print or process the data
	fmt.Println(r.Header)
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write([]byte(`{"reference_counter_increased": true}`))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		s.logger.Error("PreReceive", "error", err)
		return
	}
}

func (s *Server) post(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close() // Always close the body

	fmt.Println("Received Body:", string(body)) // Print or process the data
	fmt.Println(r.Header)
	response := postReceiveResponse{
		ReferenceCounterDecreased: true,
		Messages: []PostReceiveMessage{
			{
				Message: "test",
				Type:    "basic",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&response); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		s.logger.Error("PostReceive", "error", err)
		return
	}
}
