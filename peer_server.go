package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

func main() {
	http.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		filename := r.URL.Query().Get("filename")
		fmt.Println("Download request for:", filename) // Debug log

		file, err := os.Open(filename)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, "File not found: %s", filename)
			return
		}
		defer file.Close()

		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
		io.Copy(w, file)
	})
	fmt.Println("Serveur du pair en écoute sur :9001...")
	http.ListenAndServe(":9001", nil)
}
