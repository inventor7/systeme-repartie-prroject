package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

type FileInfo struct {
	Filename string `json:"filename"`
	FilePath string `json:"file_path"` // Chemin local du fichier
}

var filesIndex []FileInfo

func main() {
	// Configuration du serveur web
	fs := http.FileServer(http.Dir("./web/static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	// Gestion des routes
	http.HandleFunc("/", serveHomePage)
	http.HandleFunc("/register", registerHandler)
	http.HandleFunc("/search", searchHandler)
	http.HandleFunc("/download", downloadHandler)

	fmt.Println("Serveur P2P en écoute sur :8080...")
	http.ListenAndServe(":8080", nil)
}

func serveHomePage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./web/templates/index.html")
}

func registerHandler(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Fichier requis", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Enregistre le fichier localement
	filePath := filepath.Join(".", "shared_files", header.Filename)
	os.MkdirAll("shared_files", 0755)

	dst, err := os.Create(filePath)
	if err != nil {
		http.Error(w, "Erreur de création", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		http.Error(w, "Erreur d'enregistrement", http.StatusInternalServerError)
		return
	}

	// Ajoute à l'index
	filesIndex = append(filesIndex, FileInfo{
		Filename: header.Filename,
		FilePath: filePath,
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "success",
		"file":   header.Filename,
	})
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	filename := r.URL.Query().Get("filename")
	w.Header().Set("Content-Type", "application/json")

	var results []FileInfo
	for _, file := range filesIndex {
		if file.Filename == filename {
			results = append(results, file)
		}
	}

	if len(results) == 0 {
		http.Error(w, `{"error":"Fichier non trouvé"}`, http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(results[0]) // Retourne le premier résultat
}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
	filename := r.URL.Query().Get("filename")
	for _, file := range filesIndex {
		if file.Filename == filename {
			http.ServeFile(w, r, file.FilePath)
			return
		}
	}
	http.Error(w, "Fichier non trouvé", http.StatusNotFound)
}
