package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/cors"
)

// Data structures
type Peer struct {
	ID          string    `json:"id"`
	Address     string    `json:"address"`
	Port        int       `json:"port"`
	LastSeen    time.Time `json:"last_seen"`
	IsOnline    bool      `json:"is_online"`
	Reputation  int       `json:"reputation"`
	SharedFiles int       `json:"shared_files"`
	Region      string    `json:"region"`
}

type FileInfo struct {
	ID          string    `json:"id"`
	Filename    string    `json:"filename"`
	Size        int64     `json:"size"`
	Hash        string    `json:"hash"`
	Category    string    `json:"category"`
	Tags        []string  `json:"tags"`
	Owner       string    `json:"owner"`
	PeerAddress string    `json:"peer_address"`
	UploadTime  time.Time `json:"upload_time"`
	Downloads   int       `json:"downloads"`
	Rating      float64   `json:"rating"`
}

type SearchQuery struct {
	Query    string   `json:"query"`
	Category string   `json:"category"`
	Tags     []string `json:"tags"`
	SortBy   string   `json:"sort_by"`
	Limit    int      `json:"limit"`
}

type NetworkStats struct {
	TotalPeers     int       `json:"total_peers"`
	OnlinePeers    int       `json:"online_peers"`
	TotalFiles     int       `json:"total_files"`
	TotalDownloads int       `json:"total_downloads"`
	NetworkHealth  float64   `json:"network_health"`
	LastUpdated    time.Time `json:"last_updated"`
}

// Global state
type SuperPeer struct {
	peers         map[string]*Peer
	files         map[string]*FileInfo
	peersMutex    sync.RWMutex
	filesMutex    sync.RWMutex
	wsConnections map[*websocket.Conn]bool
	wsMutex       sync.RWMutex
	stats         NetworkStats
}

var (
	superPeer = &SuperPeer{
		peers:         make(map[string]*Peer),
		files:         make(map[string]*FileInfo),
		wsConnections: make(map[*websocket.Conn]bool),
	}
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for development
		},
	}
)

func main() {
	// Initialize logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Create directories
	os.MkdirAll("web/static", 0755)
	os.MkdirAll("shared_files", 0755)
	os.MkdirAll("logs", 0755)

	// Start background services
	go superPeer.healthCheckService()
	go superPeer.statisticsService()

	// Setup routes
	router := mux.NewRouter()

	// API routes
	api := router.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/peers/register", superPeer.registerPeerHandler).Methods("POST")
	api.HandleFunc("/peers/heartbeat", superPeer.heartbeatHandler).Methods("POST")
	api.HandleFunc("/peers", superPeer.getPeersHandler).Methods("GET")
	api.HandleFunc("/files/register", superPeer.registerFileHandler).Methods("POST")
	api.HandleFunc("/files/search", superPeer.searchFilesHandler).Methods("GET")
	api.HandleFunc("/files", superPeer.getFilesHandler).Methods("GET")
	api.HandleFunc("/stats", superPeer.getStatsHandler).Methods("GET")
	api.HandleFunc("/download/{fileId}", superPeer.downloadHandler).Methods("GET")

	// WebSocket endpoint
	router.HandleFunc("/ws", superPeer.websocketHandler)

	// Static files and web interface
	router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("./web/static/"))))
	router.HandleFunc("/", superPeer.serveHomePage).Methods("GET")

	// CORS middleware
	c := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"*"},
	})

	handler := c.Handler(router)

	fmt.Println("🚀 Professional P2P Super-Peer Server starting on :8080")
	fmt.Println("📊 Dashboard: http://localhost:8080")
	fmt.Println("🔌 WebSocket: ws://localhost:8080/ws")
	fmt.Println("📡 API Base: http://localhost:8080/api/v1")

	go func() {
		log.Fatal(http.ListenAndServe(":8080", handler))
	}()

	// Block main goroutine to keep servers running
	select {}
}

// Peer registration handler
func (sp *SuperPeer) registerPeerHandler(w http.ResponseWriter, r *http.Request) {
	var peer Peer
	if err := json.NewDecoder(r.Body).Decode(&peer); err != nil {
		http.Error(w, "Invalid peer data", http.StatusBadRequest)
		return
	}

	peer.LastSeen = time.Now()
	peer.IsOnline = true
	peer.ID = generatePeerID(peer.Address, peer.Port)

	sp.peersMutex.Lock()
	sp.peers[peer.ID] = &peer
	sp.peersMutex.Unlock()

	sp.broadcastUpdate("peer_registered", peer)

	log.Printf("✅ Peer registered: %s (%s:%d)", peer.ID, peer.Address, peer.Port)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"peer_id": peer.ID,
		"message": "Peer registered successfully",
	})
}

// Heartbeat handler to keep peers alive
func (sp *SuperPeer) heartbeatHandler(w http.ResponseWriter, r *http.Request) {
	peerID := r.Header.Get("X-Peer-ID")
	if peerID == "" {
		http.Error(w, "Peer ID required", http.StatusBadRequest)
		return
	}

	sp.peersMutex.Lock()
	if peer, exists := sp.peers[peerID]; exists {
		peer.LastSeen = time.Now()
		peer.IsOnline = true
	}
	sp.peersMutex.Unlock()

	w.WriteHeader(http.StatusOK)
}

// File registration handler
func (sp *SuperPeer) registerFileHandler(w http.ResponseWriter, r *http.Request) {
	var fileInfo FileInfo
	if err := json.NewDecoder(r.Body).Decode(&fileInfo); err != nil {
		http.Error(w, "Invalid file data", http.StatusBadRequest)
		return
	}

	fileInfo.ID = generateFileID(fileInfo.Filename, fileInfo.Owner)
	fileInfo.UploadTime = time.Now()

	sp.filesMutex.Lock()
	found := false
	for _, existingFile := range sp.files {
		if existingFile.Hash == fileInfo.Hash && existingFile.Owner == fileInfo.Owner {
			// Update existing file info
			existingFile.PeerAddress = fileInfo.PeerAddress
			existingFile.UploadTime = time.Now()
			existingFile.Filename = fileInfo.Filename // Update filename in case it changed
			existingFile.Size = fileInfo.Size         // Update size in case it changed
			existingFile.Category = fileInfo.Category // Update category
			existingFile.Tags = fileInfo.Tags         // Update tags
			fileInfo = *existingFile                  // Use the updated existing fileInfo for broadcast
			found = true
			log.Printf("🔄 File updated: %s by %s", fileInfo.Filename, fileInfo.Owner)
			break
		}
	}

	if !found {
		sp.files[fileInfo.ID] = &fileInfo
		log.Printf("📁 File registered: %s by %s", fileInfo.Filename, fileInfo.Owner)
	}
	sp.filesMutex.Unlock()

	sp.broadcastUpdate("file_registered", fileInfo)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"file_id": fileInfo.ID,
		"message": "File registered successfully",
	})
}

// Advanced search handler
func (sp *SuperPeer) searchFilesHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	category := r.URL.Query().Get("category")
	sortBy := r.URL.Query().Get("sort")
	limitStr := r.URL.Query().Get("limit")

	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	sp.filesMutex.RLock()
	var results []*FileInfo

	for _, file := range sp.files {
		if matchesSearch(file, query, category) {
			results = append(results, file)
		}
	}
	sp.filesMutex.RUnlock()

	// Sort results
	sortResults(results, sortBy)

	// Apply limit
	if len(results) > limit {
		results = results[:limit]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"results": results,
		"count":   len(results),
		"query":   query,
	})
}

// WebSocket handler for real-time updates
func (sp *SuperPeer) websocketHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	sp.wsMutex.Lock()
	sp.wsConnections[conn] = true
	sp.wsMutex.Unlock()

	defer func() {
		sp.wsMutex.Lock()
		delete(sp.wsConnections, conn)
		sp.wsMutex.Unlock()
	}()

	// Send initial data
	sp.sendNetworkStats(conn)

	// Keep connection alive
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// Broadcast updates to all WebSocket connections
func (sp *SuperPeer) broadcastUpdate(eventType string, data interface{}) {
	message := map[string]interface{}{
		"type":      eventType,
		"data":      data,
		"timestamp": time.Now(),
	}

	sp.wsMutex.Lock() // Acquire write lock
	for conn := range sp.wsConnections {
		if err := conn.WriteJSON(message); err != nil {
			log.Printf("Error broadcasting to WebSocket: %v", err)
			// Optionally, remove broken connection
			// delete(sp.wsConnections, conn)
		}
	}
	sp.wsMutex.Unlock() // Release write lock
}

// Health check service
func (sp *SuperPeer) healthCheckService() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		cutoff := time.Now().Add(-5 * time.Minute)

		sp.peersMutex.Lock()
		for id, peer := range sp.peers {
			if peer.LastSeen.Before(cutoff) {
				peer.IsOnline = false
				log.Printf("⚠️ Peer %s marked offline", id)
			}
		}
		sp.peersMutex.Unlock()

		sp.updateStats()
	}
}

// Statistics service
func (sp *SuperPeer) statisticsService() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		sp.updateStats()
		sp.broadcastUpdate("stats_update", sp.stats)
	}
}

// Update network statistics
func (sp *SuperPeer) updateStats() {
	sp.peersMutex.RLock()
	onlinePeers := 0
	for _, peer := range sp.peers {
		if peer.IsOnline {
			onlinePeers++
		}
	}
	totalPeers := len(sp.peers)
	sp.peersMutex.RUnlock()

	sp.filesMutex.RLock()
	totalFiles := len(sp.files)
	totalDownloads := 0
	for _, file := range sp.files {
		totalDownloads += file.Downloads
	}
	sp.filesMutex.RUnlock()

	healthScore := 0.0
	if totalPeers > 0 {
		healthScore = float64(onlinePeers) / float64(totalPeers) * 100
	}

	sp.stats = NetworkStats{
		TotalPeers:     totalPeers,
		OnlinePeers:    onlinePeers,
		TotalFiles:     totalFiles,
		TotalDownloads: totalDownloads,
		NetworkHealth:  healthScore,
		LastUpdated:    time.Now(),
	}
}

// Helper functions
func generatePeerID(address string, port int) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d", address, port, time.Now().Unix())))
	return fmt.Sprintf("%x", hash)[:16]
}

func generateFileID(filename, owner string) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", filename, owner, time.Now().Unix())))
	return fmt.Sprintf("%x", hash)[:16]
}

func matchesSearch(file *FileInfo, query, category string) bool {
	if category != "" && file.Category != category {
		return false
	}

	if query == "" {
		return true
	}

	queryLower := strings.ToLower(query)
	return strings.Contains(strings.ToLower(file.Filename), queryLower) ||
		strings.Contains(strings.ToLower(file.Category), queryLower) ||
		containsTag(file.Tags, queryLower)
}

func containsTag(tags []string, query string) bool {
	for _, tag := range tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	return false
}

func sortResults(results []*FileInfo, sortBy string) {
	switch sortBy {
	case "name":
		sort.Slice(results, func(i, j int) bool {
			return results[i].Filename < results[j].Filename
		})
	case "size":
		sort.Slice(results, func(i, j int) bool {
			return results[i].Size > results[j].Size
		})
	case "downloads":
		sort.Slice(results, func(i, j int) bool {
			return results[i].Downloads > results[j].Downloads
		})
	case "rating":
		sort.Slice(results, func(i, j int) bool {
			return results[i].Rating > results[j].Rating
		})
	default: // date
		sort.Slice(results, func(i, j int) bool {
			return results[i].UploadTime.After(results[j].UploadTime)
		})
	}
}

// API handlers
func (sp *SuperPeer) getPeersHandler(w http.ResponseWriter, r *http.Request) {
	sp.peersMutex.RLock()
	peers := make([]*Peer, 0, len(sp.peers))
	for _, peer := range sp.peers {
		peers = append(peers, peer)
	}
	sp.peersMutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(peers)
}

func (sp *SuperPeer) getFilesHandler(w http.ResponseWriter, r *http.Request) {
	sp.filesMutex.RLock()
	files := make([]*FileInfo, 0, len(sp.files))
	for _, file := range sp.files {
		files = append(files, file)
	}
	sp.filesMutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

func (sp *SuperPeer) getStatsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sp.stats)
}

func (sp *SuperPeer) downloadHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	fileID := vars["fileId"]

	sp.filesMutex.Lock()
	file, exists := sp.files[fileID]
	if exists {
		file.Downloads++
	}
	sp.filesMutex.Unlock()

	if !exists {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// Redirect to peer for actual download
	downloadURL := fmt.Sprintf("http://%s/download?filename=%s", file.PeerAddress, file.Filename)
	http.Redirect(w, r, downloadURL, http.StatusFound)
}

func (sp *SuperPeer) serveHomePage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./web/templates/index.html")
}

func (sp *SuperPeer) sendNetworkStats(conn *websocket.Conn) {
	conn.WriteJSON(map[string]interface{}{
		"type": "stats_update",
		"data": sp.stats,
	})
}
