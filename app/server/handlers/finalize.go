package handlers

import (
	"io"
	"log"
	"net/http"
)

func FinalizeProjectHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Received request for FinalizeProjectHandler")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading finalize request body: %v\n", err)
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	log.Printf("Finalized Project Data: %s", string(body))

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Project finalized and received successfully"))
}
