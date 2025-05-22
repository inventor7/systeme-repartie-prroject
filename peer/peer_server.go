package peer

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/cors"
)

// Configuration
type PeerConfig struct {
	Port              int    `json:"port"`
	SuperPeerAddress  string `json:"super_peer_address"`
	SharedDirectory   string `json:"shared_directory"`
	MaxFileSize       int64  `json:"max_file_size"`
	HeartbeatInterval int    `json:"heartbeat_interval"`
}

// Peer represents this peer instance
type Peer struct {
	ID            string                 `json:"id"`
	Address       string                 `json:"address"`
	Port          int                    `json:"port"`
	SharedFiles   map[string]*SharedFile `json:"shared_files"`
	Config        PeerConfig             `json:"config"`
	IsRegistered  bool                   `json:"is_registered"`
	LastHeartbeat time.Time              `json:"last_heartbeat"`
	DownloadStats DownloadStats          `json:"download_stats"`
	UploadStats   UploadStats            `json:"upload_stats"`
	mutex         sync.RWMutex
}

type SharedFile struct {
	ID          string    `json:"id"`
	Filename    string    `json:"filename"`
	FilePath    string    `json:"file_path"`
	Size        int64     `json:"size"`
	Hash        string    `json:"hash"`
	Category    string    `json:"category"`
	Tags        []string  `json:"tags"`
	SharedAt    time.Time `json:"shared_at"`
	Downloads   int       `json:"downloads"`
	IsAvailable bool      `json:"is_available"`
}

type DownloadStats struct {
	TotalDownloads  int64 `json:"total_downloads"`
	TotalBytes      int64 `json:"total_bytes"`
	ActiveDownloads int   `json:"active_downloads"`
}

type UploadStats struct {
	TotalUploads  int64 `json:"total_uploads"`
	TotalBytes    int64 `json:"total_bytes"`
	ActiveUploads int   `json:"active_uploads"`
}

type DownloadProgress struct {
	FileID   string  `json:"file_id"`
	Filename string  `json:"filename"`
	Progress float64 `json:"progress"`
	Speed    int64   `json:"speed"`
	ETA      int64   `json:"eta"`
	Status   string  `json:"status"`
}

// Global peer instance
var (
	wsConnections = make(map[*websocket.Conn]bool)
	wsMutex       sync.RWMutex
)

func StartPeerServer() {
	// Initialize peer
	p := &Peer{
		SharedFiles: make(map[string]*SharedFile),
		Config: PeerConfig{
			Port:              9001, // Default port
			SuperPeerAddress:  "localhost:8080",
			SharedDirectory:   "./shared_files",
			MaxFileSize:       100 * 1024 * 1024, // 100MB
			HeartbeatInterval: 30,                // seconds
		},
	}

	// Override port from environment variable if set
	if os.Getenv("PEER_PORT") != "" {
		portStr := strings.TrimSpace(os.Getenv("PEER_PORT"))
		if port, err := strconv.Atoi(portStr); err == nil {
			p.Config.Port = port
		} else {
			log.Printf("ERROR: Failed to parse PEER_PORT '%s': %v", portStr, err)
		}
	}
	p.Port = p.Config.Port // Ensure p.Port is updated from config
	log.Printf("DEBUG: Peer will attempt to listen on port: %d", p.Config.Port)

	// Load configuration (this will set p.Address and p.ID)
	p.loadConfig()

	// Create shared directory
	os.MkdirAll(p.Config.SharedDirectory, 0755)

	// Start services
	go p.heartbeatService()
	go p.fileWatcherService()

	// Setup routes
	router := mux.NewRouter()

	// API routes
	api := router.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/info", p.getPeerInfoHandler).Methods("GET")
	api.HandleFunc("/files", p.getFilesHandler).Methods("GET")
	api.HandleFunc("/files/share", p.shareFileHandler).Methods("POST")
	api.HandleFunc("/files/unshare/{fileId}", p.unshareFileHandler).Methods("DELETE")
	api.HandleFunc("/download/{fileId}", p.downloadFileHandler).Methods("GET")
	api.HandleFunc("/upload", p.uploadFileHandler).Methods("POST")
	api.HandleFunc("/stats", p.getStatsHandler).Methods("GET")
	api.HandleFunc("/search", p.searchFilesHandler).Methods("GET")

	// WebSocket endpoint
	router.HandleFunc("/ws", p.websocketHandler)

	// Web interface
	router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("./web/static/"))))
	router.HandleFunc("/", p.serveHomePage).Methods("GET")

	// CORS middleware
	c := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"*"},
	})

	handler := c.Handler(router)

	fmt.Printf("🚀 Professional P2P Peer Server starting on :%d\n", p.Config.Port)
	fmt.Printf("📁 Shared Directory: %s\n", p.Config.SharedDirectory)
	fmt.Printf("🔗 Super-Peer: %s\n", p.Config.SuperPeerAddress)
	fmt.Printf("🌐 Web Interface: http://localhost:%d\n", p.Config.Port)

	// Register with super-peer
	go p.registerWithSuperPeer()

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", p.Config.Port), handler))
}

func (p *Peer) loadConfig() {
	// Load configuration from file or environment variables
	// For now, using defaults
	p.Address = "127.0.0.1"
	p.ID = generatePeerID(p.Address, p.Port)

	// Debugging: Print PEER_PORT environment variable
	log.Printf("DEBUG: PEER_PORT environment variable: %s", os.Getenv("PEER_PORT"))
}

func (p *Peer) initializePeer() {
	// Scan shared directory for existing files
	p.scanSharedDirectory()

	log.Printf("✅ Peer initialized with ID: %s", p.ID)
	log.Printf("📂 Found %d shared files", len(p.SharedFiles))
}

func (p *Peer) scanSharedDirectory() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	filepath.Walk(p.Config.SharedDirectory, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		fileID := generateFileID(info.Name(), p.ID)

		// Calculate file hash
		hash, _ := calculateFileHash(path)

		sharedFile := &SharedFile{
			ID:          fileID,
			Filename:    info.Name(),
			FilePath:    path,
			Size:        info.Size(),
			Hash:        hash,
			Category:    categorizeFile(info.Name()),
			Tags:        extractTags(info.Name()),
			SharedAt:    time.Now(),
			IsAvailable: true,
		}

		p.SharedFiles[fileID] = sharedFile
		return nil
	})
}

func (p *Peer) registerWithSuperPeer() {
	for {
		if p.registerPeer() {
			log.Println("✅ Successfully registered with super-peer")
			return
		}
		log.Println("❌ Failed to register with super-peer, retrying in 10 seconds...")
		time.Sleep(10 * time.Second)
	}
}

func (p *Peer) registerPeer() bool {
	peerData := map[string]interface{}{
		"id":           p.ID,
		"address":      p.Address,
		"port":         p.Port,
		"shared_files": len(p.SharedFiles),
		"region":       "local", // Could be determined by IP geolocation
	}

	jsonData, _ := json.Marshal(peerData)

	resp, err := http.Post(
		fmt.Sprintf("http://%s/api/v1/peers/register", p.Config.SuperPeerAddress),
		"application/json",
		bytes.NewBuffer(jsonData),
	)

	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		p.IsRegistered = true

		// Register all shared files
		go p.registerAllFiles()
		return true
	}

	return false
}

func (p *Peer) registerAllFiles() {
	p.mutex.RLock()
	files := make([]*SharedFile, 0, len(p.SharedFiles))
	for _, file := range p.SharedFiles {
		files = append(files, file)
	}
	p.mutex.RUnlock()

	for _, file := range files {
		p.registerFileWithSuperPeer(file)
		time.Sleep(100 * time.Millisecond) // Rate limiting
	}
}

func (p *Peer) registerFileWithSuperPeer(file *SharedFile) {
	fileData := map[string]interface{}{
		"filename":     file.Filename,
		"size":         file.Size,
		"hash":         file.Hash,
		"category":     file.Category,
		"tags":         file.Tags,
		"owner":        p.ID,
		"peer_address": fmt.Sprintf("%s:%d", p.Address, p.Port),
	}

	jsonData, _ := json.Marshal(fileData)

	resp, err := http.Post(
		fmt.Sprintf("http://%s/api/v1/files/register", p.Config.SuperPeerAddress),
		"application/json",
		bytes.NewBuffer(jsonData),
	)

	if err == nil {
		resp.Body.Close()
	}
}

func (p *Peer) heartbeatService() {
	ticker := time.NewTicker(time.Duration(p.Config.HeartbeatInterval) * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		p.sendHeartbeat()
	}
}

func (p *Peer) sendHeartbeat() {
	if !p.IsRegistered {
		return
	}

	req, _ := http.NewRequest("POST",
		fmt.Sprintf("http://%s/api/v1/peers/heartbeat", p.Config.SuperPeerAddress),
		nil)
	req.Header.Set("X-Peer-ID", p.ID)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err == nil {
		resp.Body.Close()
		p.LastHeartbeat = time.Now()
	}
}

func (p *Peer) fileWatcherService() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		p.scanForNewFiles()
	}
}

func (p *Peer) scanForNewFiles() {
	currentFiles := make(map[string]bool)

	filepath.Walk(p.Config.SharedDirectory, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		filename := info.Name()
		fileID := generateFileID(filename, p.ID)
		currentFiles[fileID] = true

		p.mutex.RLock()
		_, exists := p.SharedFiles[fileID]
		p.mutex.RUnlock()

		if !exists {
			// New file detected
			hash, _ := calculateFileHash(path)

			sharedFile := &SharedFile{
				ID:          fileID,
				Filename:    filename,
				FilePath:    path,
				Size:        info.Size(),
				Hash:        hash,
				Category:    categorizeFile(filename),
				Tags:        extractTags(filename),
				SharedAt:    time.Now(),
				IsAvailable: true,
			}

			p.mutex.Lock()
			p.SharedFiles[fileID] = sharedFile
			p.mutex.Unlock()

			// Register with super-peer
			go p.registerFileWithSuperPeer(sharedFile)

			// Broadcast to WebSocket clients
			p.broadcastUpdate("file_added", sharedFile)

			log.Printf("📁 New file detected: %s", filename)
		}

		return nil
	})

	// Check for removed files
	p.mutex.Lock()
	for fileID, file := range p.SharedFiles {
		if !currentFiles[fileID] {
			delete(p.SharedFiles, fileID)
			p.broadcastUpdate("file_removed", file)
			log.Printf("📁 File removed: %s", file.Filename)
		}
	}
	p.mutex.Unlock()
}

// HTTP Handlers
func (p *Peer) getPeerInfoHandler(w http.ResponseWriter, r *http.Request) {
	p.mutex.RLock()
	info := map[string]interface{}{
		"id":             p.ID,
		"address":        p.Address,
		"port":           p.Port,
		"is_registered":  p.IsRegistered,
		"shared_files":   len(p.SharedFiles),
		"last_heartbeat": p.LastHeartbeat,
		"download_stats": p.DownloadStats,
		"upload_stats":   p.UploadStats,
		"config":         p.Config,
	}
	p.mutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func (p *Peer) getFilesHandler(w http.ResponseWriter, r *http.Request) {
	p.mutex.RLock()
	files := make([]*SharedFile, 0, len(p.SharedFiles))
	for _, file := range p.SharedFiles {
		if file.IsAvailable {
			files = append(files, file)
		}
	}
	p.mutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

func (p *Peer) shareFileHandler(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "File required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Check file size
	if header.Size > p.Config.MaxFileSize {
		http.Error(w, "File too large", http.StatusRequestEntityTooLarge)
		return
	}

	// Save file
	filename := header.Filename
	filePath := filepath.Join(p.Config.SharedDirectory, filename)

	dst, err := os.Create(filePath)
	if err != nil {
		http.Error(w, "Failed to save file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, file)
	if err != nil {
		http.Error(w, "Failed to save file", http.StatusInternalServerError)
		return
	}

	// Calculate hash
	hash, _ := calculateFileHash(filePath)

	// Create shared file entry
	fileID := generateFileID(filename, p.ID)
	sharedFile := &SharedFile{
		ID:          fileID,
		Filename:    filename,
		FilePath:    filePath,
		Size:        written,
		Hash:        hash,
		Category:    categorizeFile(filename),
		Tags:        extractTags(filename),
		SharedAt:    time.Now(),
		IsAvailable: true,
	}

	p.mutex.Lock()
	p.SharedFiles[fileID] = sharedFile
	p.mutex.Unlock()

	// Register with super-peer
	go p.registerFileWithSuperPeer(sharedFile)

	// Broadcast update
	p.broadcastUpdate("file_shared", sharedFile)

	log.Printf("📁 File shared: %s (%d bytes)", filename, written)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"file_id": fileID,
		"message": "File shared successfully",
	})
}

func (p *Peer) downloadFileHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	fileID := vars["fileId"]

	// Also support legacy filename parameter
	if fileID == "" {
		filename := r.URL.Query().Get("filename")
		if filename != "" {
			// Find file by filename
			p.mutex.RLock()
			for id, file := range p.SharedFiles {
				if file.Filename == filename {
					fileID = id
					break
				}
			}
			p.mutex.RUnlock()
		}
	}

	p.mutex.RLock()
	file, exists := p.SharedFiles[fileID]
	p.mutex.RUnlock()

	if !exists || !file.IsAvailable {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// Check if file still exists on disk
	if _, err := os.Stat(file.FilePath); os.IsNotExist(err) {
		http.Error(w, "File not available", http.StatusNotFound)
		return
	}

	// Update download stats
	p.mutex.Lock()
	file.Downloads++
	p.DownloadStats.TotalDownloads++
	p.DownloadStats.TotalBytes += file.Size
	p.mutex.Unlock()

	// Set headers for download
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", file.Filename))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(file.Size, 10))

	// Serve file with progress tracking
	http.ServeFile(w, r, file.FilePath)

	log.Printf("📥 File downloaded: %s by %s", file.Filename, r.RemoteAddr)
}

func (p *Peer) uploadFileHandler(w http.ResponseWriter, r *http.Request) {
	// Handle file upload via multipart form
	p.shareFileHandler(w, r)
}

func (p *Peer) getStatsHandler(w http.ResponseWriter, r *http.Request) {
	p.mutex.RLock()
	stats := map[string]interface{}{
		"peer_id":        p.ID,
		"shared_files":   len(p.SharedFiles),
		"download_stats": p.DownloadStats,
		"upload_stats":   p.UploadStats,
		"is_registered":  p.IsRegistered,
		"last_heartbeat": p.LastHeartbeat,
	}
	p.mutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (p *Peer) searchFilesHandler(w http.ResponseWriter, r *http.Request) {
	query := strings.ToLower(r.URL.Query().Get("q"))
	category := r.URL.Query().Get("category")

	p.mutex.RLock()
	var results []*SharedFile
	for _, file := range p.SharedFiles {
		if !file.IsAvailable {
			continue
		}

		matches := false
		if query == "" {
			matches = true
		} else {
			matches = strings.Contains(strings.ToLower(file.Filename), query) ||
				strings.Contains(strings.ToLower(file.Category), query) ||
				containsTag(file.Tags, query)
		}

		if matches && (category == "" || file.Category == category) {
			results = append(results, file)
		}
	}
	p.mutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"results": results,
		"count":   len(results),
		"query":   query,
	})
}

func (p *Peer) unshareFileHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	fileID := vars["fileId"]

	p.mutex.Lock()
	file, exists := p.SharedFiles[fileID]
	if exists {
		delete(p.SharedFiles, fileID)
	}
	p.mutex.Unlock()

	if !exists {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// Remove file from disk (optional)
	if r.URL.Query().Get("delete") == "true" {
		os.Remove(file.FilePath)
	}

	// Broadcast update
	p.broadcastUpdate("file_unshared", file)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "File unshared successfully",
	})
}

func (p *Peer) websocketHandler(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	wsMutex.Lock()
	wsConnections[conn] = true
	wsMutex.Unlock()

	defer func() {
		wsMutex.Lock()
		delete(wsConnections, conn)
		wsMutex.Unlock()
	}()

	// Send initial data
	p.sendPeerInfo(conn)

	// Keep connection alive
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (p *Peer) broadcastUpdate(eventType string, data interface{}) {
	message := map[string]interface{}{
		"type":      eventType,
		"data":      data,
		"timestamp": time.Now(),
	}

	wsMutex.RLock()
	for conn := range wsConnections {
		conn.WriteJSON(message)
	}
	wsMutex.RUnlock()
}

func (p *Peer) sendPeerInfo(conn *websocket.Conn) {
	p.mutex.RLock()
	info := map[string]interface{}{
		"peer_info":    p,
		"shared_files": p.SharedFiles,
	}
	p.mutex.RUnlock()

	conn.WriteJSON(map[string]interface{}{
		"type": "peer_info",
		"data": info,
	})
}

func (p *Peer) serveHomePage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./web/templates/p2p.html")
}

// Utility functions
func generatePeerID(address string, port int) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d", address, port, time.Now().Unix())))
	return fmt.Sprintf("peer_%x", hash)[:16]
}

func generateFileID(filename, ownerID string) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", filename, ownerID, time.Now().Unix())))
	return fmt.Sprintf("file_%x", hash)[:16]
}

func calculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func categorizeFile(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))

	categories := map[string]string{
		".txt": "document", ".doc": "document", ".docx": "document", ".pdf": "document",
		".xls": "document", ".xlsx": "document", ".ppt": "document", ".pptx": "document",
		".jpg": "image", ".jpeg": "image", ".png": "image", ".gif": "image", ".bmp": "image",
		".mp4": "video", ".avi": "video", ".mkv": "video", ".mov": "video", ".wmv": "video",
		".mp3": "audio", ".wav": "audio", ".flac": "audio", ".aac": "audio", ".ogg": "audio",
		".zip": "archive", ".rar": "archive", ".7z": "archive", ".tar": "archive", ".gz": "archive",
	}

	if category, exists := categories[ext]; exists {
		return category
	}
	return "other"
}

func extractTags(filename string) []string {
	// Simple tag extraction based on filename patterns
	tags := []string{}

	name := strings.ToLower(filename)
	if strings.Contains(name, "music") || strings.Contains(name, "song") {
		tags = append(tags, "music")
	}
	if strings.Contains(name, "video") || strings.Contains(name, "movie") {
		tags = append(tags, "video")
	}
	if strings.Contains(name, "photo") || strings.Contains(name, "image") {
		tags = append(tags, "photo")
	}
	if strings.Contains(name, "document") || strings.Contains(name, "doc") {
		tags = append(tags, "document")
	}

	return tags
}

func containsTag(tags []string, query string) bool {
	for _, tag := range tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	return false
}
