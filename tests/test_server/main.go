package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	fileName := "1gb.bin"
	
	// Create sparse file
	f, err := os.Create(fileName)
	if err != nil {
		log.Fatal(err)
	}
	
	// 1 GB file
	if err := f.Truncate(1073741824); err != nil {
		log.Fatal(err)
	}
	f.Close()
	
	log.Println("Test server ready. Serving 1GB sparse file on http://localhost:9999/1gb.bin")
	log.Println("Serving chunked stream on http://localhost:9999/stream")
	
	http.HandleFunc("/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		for i := 0; i < 10; i++ {
			w.Write([]byte("chunked data chunked data chunked data\n"))
			w.(http.Flusher).Flush()
		}
	})

	fs := http.FileServer(http.Dir("."))
	http.Handle("/", fs)
	
	log.Fatal(http.ListenAndServe(":9999", nil))
}
