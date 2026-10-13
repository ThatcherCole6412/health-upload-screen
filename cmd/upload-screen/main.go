package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strings"

	"github.com/example/health-upload-screen/internal/screening"
)

const maxUpload = 12 << 20

type server struct {
	workflow *screening.Workflow
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	client := screening.NewInfraiClient(key, nil)
	service := &server{workflow: screening.NewWorkflow(client, client)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /screen", service.screen)
	addr := envOr("ADDR", ":8080")
	log.Printf("upload screen listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func (s *server) screen(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart request"})
		return
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "image is required"})
		return
	}
	defer file.Close()
	image, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot read image"})
		return
	}

	decision, err := s.workflow.Screen(r.Context(), screening.Submission{
		ID:          strings.TrimSpace(r.FormValue("submission_id")),
		Appointment: strings.TrimSpace(r.FormValue("appointment_id")),
		Filename:    safeFilename(header),
		Caption:     strings.TrimSpace(r.FormValue("caption")),
		Image:       image,
	})
	if err != nil {
		var apiErr *screening.InfraiError
		if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
			writeJSON(w, apiErr.HTTPStatus, map[string]string{"error": apiErr.Error()})
			return
		}
		log.Printf("screen upload: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "screening service unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, decision)
}

func safeFilename(header *multipart.FileHeader) string {
	name := strings.ReplaceAll(header.Filename, "\\", "/")
	parts := strings.Split(name, "/")
	return parts[len(parts)-1]
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write response: %v", err)
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
