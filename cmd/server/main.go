package main

import (
	"log"
	"net/http"
	"os"

	"roadmapapi/internal/routes"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	r := routes.NewRouter()
	log.Printf("Roadmap API running on :%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
	}
}
