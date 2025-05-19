package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

type FileInfo struct {
	Filename string `json:"filename"`
	Address  string `json:"address"`
}

func main() {
	// Étape 1 : S'enregistrer avec un fichier
	register()

	// Étape 2 : Rechercher un fichier
	var searchFilename string
	fmt.Print("Quel fichier voulez-vous chercher ? ")
	fmt.Scanln(&searchFilename)

	peerAddr := searchFile(searchFilename)
	if peerAddr != "" {
		downloadFile(searchFilename, peerAddr)
	}
}

func register() {
	// Remplace ce chemin par un fichier réel que tu veux partager
	fileToShare := "file1.txt"
	address := "127.0.0.1:9001"

	fileInfo := FileInfo{
		Filename: fileToShare,
		Address:  address,
	}

	data, _ := json.Marshal(fileInfo)
	resp, err := http.Post("http://localhost:8080/register", "application/json", bytes.NewBuffer(data))
	if err != nil {
		fmt.Println("Erreur d'enregistrement:", err)
		return
	}
	defer resp.Body.Close()
	fmt.Println("Fichier enregistré avec succès !")
}

func searchFile(name string) string {
	resp, err := http.Get("http://localhost:8080/search?filename=" + name)
	if err != nil {
		fmt.Println("Erreur de recherche:", err)
		return ""
	}
	defer resp.Body.Close()

	var result FileInfo
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Println("Erreur de décodage de la réponse:", err)
		return ""
	}

	if result.Address == "" {
		fmt.Println("Fichier non trouvé.")
		return ""
	}

	fmt.Println("Résultats trouvés :")
	fmt.Printf("Fichier: %s, disponible chez: %s\n", result.Filename, result.Address)
	return result.Address
}

func downloadFile(name string, peerAddr string) {
	url := fmt.Sprintf("http://%s/download?filename=%s", peerAddr, name)
	resp, err := http.Get(url)
	if err != nil {
		fmt.Println("Erreur lors du téléchargement :", err)
		return
	}
	defer resp.Body.Close()

	out, err := os.Create("downloaded_" + name)
	if err != nil {
		fmt.Println("Erreur de création du fichier local :", err)
		return
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		fmt.Println("Erreur de copie du contenu :", err)
		return
	}

	fmt.Println("Fichier téléchargé avec succès !")
}
