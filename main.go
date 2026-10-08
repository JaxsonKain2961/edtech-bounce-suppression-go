package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

func main() {
	client, err := NewInfraiClientFromEnv()
	if err != nil {
		panic(err)
	}
	service := NewDeliveryService(client)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /deliveries", func(w http.ResponseWriter, r *http.Request) {
		var input LearnerDelivery
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		delivery, err := service.Deliver(r.Context(), input)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, delivery)
	})
	mux.HandleFunc("POST /bounces", func(w http.ResponseWriter, r *http.Request) {
		var input BounceNotice
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		delivery, err := service.RecordBounce(r.Context(), input)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, delivery)
	})
	mux.HandleFunc("GET /educator-report", func(w http.ResponseWriter, r *http.Request) {
		course := r.URL.Query().Get("course")
		writeJSON(w, http.StatusOK, service.Report(course, time.Now().UTC()))
	})
	if err := http.ListenAndServe(":8080", mux); err != nil {
		panic(err)
	}
}

func writeServiceError(w http.ResponseWriter, err error) {
	var upstream *InfraiError
	if errors.As(err, &upstream) && upstream.Status >= 400 && upstream.Status < 500 {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeError(w, http.StatusBadGateway, err)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}
