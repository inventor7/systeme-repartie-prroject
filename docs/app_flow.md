# P2P File Sharing Application Flow Documentation

This document outlines the functional flow and architecture of the P2P file sharing application, which consists of a central Super-Peer (index server) and multiple Peer nodes (clients that also act as servers).

## 1. Overall Architecture

The system operates on a hybrid P2P model:

*   **Super-Peer (Index Server):** A central server responsible for maintaining an index of all active peers and the files they share. It facilitates discovery but does not directly participate in file transfers.
*   **Peer Nodes (Clients/Servers):** Individual nodes that connect to the Super-Peer. They can:
    *   Register themselves and their shared files with the Super-Peer.
    *   Search for files via the Super-Peer.
    *   Download files directly from other peers.
    *   Serve files directly to other peers.

Communication between peers and the Super-Peer, and between peers themselves, is primarily over HTTP/TCP. WebSockets are used for real-time updates from the Super-Peer to connected clients (e.g., the dashboard).

## 2. Peer Registration Flow

When a Peer node starts, it registers itself with the Super-Peer.

**Flow:**

1.  **Peer Initialization:** A Peer node starts and initializes its configuration (port, Super-Peer address, shared directory).
2.  **Scan Shared Directory:** The Peer scans its local `shared_files` directory to identify files it will share. For each file, it calculates a unique ID, hash, size, category, and tags.
3.  **Attempt Registration (Loop):** The Peer attempts to register with the Super-Peer. If unsuccessful, it retries after a delay.
4.  **Send Registration Request:** The Peer sends an HTTP POST request to the Super-Peer's `/api/v1/peers/register` endpoint, including its ID, IP address, port, and count of shared files.
5.  **Super-Peer Processes Registration:**
    *   Receives the peer data.
    *   Generates a unique Peer ID (if not provided or for verification).
    *   Stores the Peer's information (ID, Address, Port, LastSeen, IsOnline, etc.) in its in-memory `peers` map.
    *   Broadcasts a "peer_registered" update to all connected WebSocket clients.
6.  **Peer Registers Files:** Upon successful peer registration, the Peer iterates through its locally shared files and sends a separate registration request for each file to the Super-Peer's `/api/v1/files/register` endpoint.

**Pseudocode (Peer Registration):**

```
FUNCTION StartPeerServer():
    Initialize Peer_Config (Port, SuperPeer_Address, Shared_Directory)
    Initialize Peer_Instance (ID, Address, Port, Shared_Files_Map, Config)

    Scan_Shared_Directory(Peer_Instance) // Populate Shared_Files_Map

    LOOP FOREVER:
        IF Register_Peer_With_SuperPeer(Peer_Instance) THEN
            Log("Successfully registered with Super-Peer")
            Register_All_Shared_Files(Peer_Instance)
            BREAK LOOP
        ELSE
            Log("Failed to register with Super-Peer, retrying...")
            WAIT 10 seconds
        END IF
    END LOOP

    Start_Heartbeat_Service(Peer_Instance) // Periodically send heartbeats
    Start_File_Watcher_Service(Peer_Instance) // Monitor shared directory for changes

    Start_HTTP_Server(Peer_Instance.Port, Peer_Handlers) // Start peer's own server for file transfers and local UI

FUNCTION Register_Peer_With_SuperPeer(peer):
    peer_data = {ID: peer.ID, Address: peer.Address, Port: peer.Port, SharedFiles: COUNT(peer.SharedFiles)}
    response = HTTP_POST(SuperPeer_Address + "/api/v1/peers/register", peer_data)
    IF response.Status == OK THEN
        peer.IsRegistered = TRUE
        RETURN TRUE
    ELSE
        RETURN FALSE

FUNCTION Register_All_Shared_Files(peer):
    FOR EACH file IN peer.Shared_Files_Map:
        Register_File_With_SuperPeer(peer, file)
        WAIT 100 milliseconds // Rate limit

FUNCTION Register_File_With_SuperPeer(peer, file):
    file_data = {Filename: file.Filename, Size: file.Size, Hash: file.Hash, Category: file.Category, Tags: file.Tags, Owner: peer.ID, PeerAddress: peer.Address + ":" + peer.Port}
    HTTP_POST(SuperPeer_Address + "/api/v1/files/register", file_data)

```

**Pseudocode (Super-Peer File Registration Handler):**

```
FUNCTION registerFileHandler(request, response):
    Parse file_info FROM request_body

    Acquire files_mutex_lock
    found = FALSE
    FOR EACH existing_file IN SuperPeer.files:
        IF existing_file.Hash == file_info.Hash AND existing_file.Owner == file_info.Owner THEN
            // Update existing file information
            existing_file.PeerAddress = file_info.PeerAddress
            existing_file.UploadTime = CURRENT_TIME
            // Update other metadata if necessary
            file_info = existing_file // Use updated info for broadcast
            found = TRUE
            Log("File updated: " + file_info.Filename)
            BREAK
        END IF
    END FOR

    IF NOT found THEN
        file_info.ID = Generate_File_ID(file_info.Filename, file_info.Owner)
        file_info.UploadTime = CURRENT_TIME
        SuperPeer.files[file_info.ID] = file_info
        Log("File registered: " + file_info.Filename)
    END IF
    Release files_mutex_lock

    Broadcast_Update("file_registered", file_info)
    Send_Success_Response(response, file_info.ID)
```

## 3. File Sharing Flow

A Peer shares files by placing them in its designated `shared_files` directory. The Peer's `fileWatcherService` automatically detects these files and registers them with the Super-Peer.

**Flow:**

1.  **File Placement:** A user places a file into the Peer's `shared_files` directory.
2.  **File Watcher Detection:** The Peer's `fileWatcherService` (running as a goroutine) periodically scans the `shared_files` directory.
3.  **New File Processing:** If a new file is detected:
    *   The Peer calculates the file's hash, size, and extracts metadata (category, tags).
    *   It creates a `SharedFile` entry for the file.
    *   It sends an HTTP POST request to the Super-Peer's `/api/v1/files/register` endpoint, providing the file's metadata and its own `PeerAddress`.
    *   The Peer broadcasts a "file_added" update to its local WebSocket clients (for UI updates).
4.  **Removed File Processing:** If a file is removed from the `shared_files` directory, the Peer updates its local `SharedFiles` map and broadcasts a "file_removed" update.

**Pseudocode (Peer File Watcher):**

```
FUNCTION fileWatcherService(peer):
    LOOP FOREVER:
        Scan_For_New_Files(peer)
        WAIT 30 seconds
    END LOOP

FUNCTION Scan_For_New_Files(peer):
    current_files_in_directory = GET_FILES_IN_DIRECTORY(peer.Config.Shared_Directory)
    
    Acquire peer_mutex_lock
    FOR EACH file_path IN current_files_in_directory:
        file_name = GET_FILENAME(file_path)
        file_id = Generate_File_ID(file_name, peer.ID)

        IF file_id NOT IN peer.Shared_Files_Map THEN
            // New file detected
            hash = Calculate_File_Hash(file_path)
            size = GET_FILE_SIZE(file_path)
            category = Categorize_File(file_name)
            tags = Extract_Tags(file_name)

            new_shared_file = CREATE_SHARED_FILE_OBJECT(file_id, file_name, file_path, size, hash, category, tags, CURRENT_TIME, TRUE)
            peer.Shared_Files_Map[file_id] = new_shared_file

            Register_File_With_SuperPeer(peer, new_shared_file) // Asynchronous call
            Broadcast_Update_Local("file_added", new_shared_file)
            Log("New file detected: " + file_name)
        END IF
    END FOR

    FOR EACH file_id, shared_file IN peer.Shared_Files_Map:
        IF shared_file.FilePath NOT IN current_files_in_directory THEN
            // File removed
            DELETE shared_file FROM peer.Shared_Files_Map
            Broadcast_Update_Local("file_removed", shared_file)
            Log("File removed: " + shared_file.Filename)
        END IF
    END FOR
    Release peer_mutex_lock
```

## 4. File Search Flow

A Peer searches for files by querying the Super-Peer.

**Flow:**

1.  **User Initiates Search:** A user on a Peer's local web interface (or via API) enters a search query.
2.  **Peer Sends Search Request:** The Peer sends an HTTP GET request to the Super-Peer's `/api/v1/files/search` endpoint, including query parameters (e.g., `q`, `category`, `sort`, `limit`).
3.  **Super-Peer Processes Search:**
    *   Receives the search query.
    *   Iterates through its `files` index.
    *   Filters files based on the query, category, and tags.
    *   Sorts the results based on criteria (e.g., filename, size, downloads, rating, upload time).
    *   Applies any specified limit to the number of results.
    *   Returns a list of matching `FileInfo` objects to the requesting Peer.
4.  **Peer Displays Results:** The Peer receives the search results and displays them to the user. Each result includes the `PeerAddress` of the file's owner.

**Pseudocode (Peer Search Handler):**

```
FUNCTION searchFilesHandler(request, response):
    query = GET_QUERY_PARAM(request, "q")
    category = GET_QUERY_PARAM(request, "category")
    sort_by = GET_QUERY_PARAM(request, "sort")
    limit = PARSE_INT_QUERY_PARAM(request, "limit", 50)

    search_url = SuperPeer_Address + "/api/v1/files/search?q=" + query + "&category=" + category + "&sort=" + sort_by + "&limit=" + limit
    response_from_super_peer = HTTP_GET(search_url)

    IF response_from_super_peer.Status == OK THEN
        Parse results FROM response_from_super_peer.Body
        Send_JSON_Response(response, {results: results, count: COUNT(results)})
    ELSE
        Send_Error_Response(response, "Failed to search files")
    END IF
```

**Pseudocode (Super-Peer Search Handler):**

```
FUNCTION searchFilesHandler(request, response):
    query = GET_QUERY_PARAM(request, "q")
    category = GET_QUERY_PARAM(request, "category")
    // ... other search parameters

    Acquire files_mutex_read_lock
    filtered_results = []
    FOR EACH file IN SuperPeer.files:
        IF Matches_Search_Criteria(file, query, category) THEN
            ADD file TO filtered_results
        END IF
    END FOR
    Release files_mutex_read_lock

    Sort_Results(filtered_results, sort_by)
    Apply_Limit(filtered_results, limit)

    Send_JSON_Response(response, {results: filtered_results, count: COUNT(filtered_results)})
```

## 5. File Download Flow

Once a file is located, the requesting Peer downloads it directly from the owning Peer.

**Flow:**

1.  **User Initiates Download:** A user on a Peer's local web interface clicks to download a file. The request is sent to the Super-Peer's `/download/{fileId}` endpoint.
2.  **Super-Peer Redirects:**
    *   Receives the download request for `fileId`.
    *   Looks up the `FileInfo` for `fileId` in its `files` index to get the `PeerAddress` of the owner.
    *   Increments the `Downloads` count for the file.
    *   Redirects the requesting client (browser) to the owning Peer's download endpoint (e.g., `http://[PeerAddress]/download?filename=[filename]`).
3.  **Requesting Peer Downloads Directly:** The client (browser) follows the redirect and sends an HTTP GET request directly to the owning Peer's `/download/{fileId}` endpoint (or `/download?filename=`).
4.  **Owning Peer Serves File:**
    *   The owning Peer receives the download request.
    *   Locates the file in its local `shared_files` directory.
    *   Streams the file content back to the requesting client.
    *   Increments its local `UploadStats`.
5.  **Requesting Peer Receives File:** The requesting client receives the file data.

**Pseudocode (Super-Peer Download Handler):**

```
FUNCTION downloadHandler(request, response):
    file_id = GET_PATH_PARAM(request, "fileId")

    Acquire files_mutex_lock
    file = SuperPeer.files[file_id]
    IF file EXISTS THEN
        file.Downloads = file.Downloads + 1
        download_url = "http://" + file.PeerAddress + "/download?filename=" + file.Filename
        Redirect_Client(response, download_url)
    ELSE
        Send_Error_Response(response, "File not found")
    END IF
    Release files_mutex_lock
```

**Pseudocode (Owning Peer Download Handler):**

```
FUNCTION downloadFileHandler(request, response):
    file_id = GET_PATH_PARAM(request, "fileId")
    // OR filename = GET_QUERY_PARAM(request, "filename")

    Acquire peer_mutex_read_lock
    file = peer.Shared_Files_Map[file_id] // Or find by filename
    Release peer_mutex_read_lock

    IF file EXISTS AND file.IsAvailable THEN
        Serve_File_Content(response, file.FilePath, file.Filename)
        Acquire peer_mutex_lock
        peer.UploadStats.TotalUploads = peer.UploadStats.TotalUploads + 1
        peer.UploadStats.TotalBytes = peer.UploadStats.TotalBytes + file.Size
        Release peer_mutex_lock
        Log("File served: " + file.Filename)
    ELSE
        Send_Error_Response(response, "File not found or not available")
    END IF
```

## 6. Heartbeat and Health Check

Peers send periodic heartbeats to the Super-Peer to indicate they are online. The Super-Peer uses this to track peer liveness.

**Flow:**

1.  **Peer Heartbeat Service:** A Peer's `heartbeatService` (running as a goroutine) periodically sends a heartbeat.
2.  **Send Heartbeat Request:** The Peer sends an HTTP POST request to the Super-Peer's `/api/v1/peers/heartbeat` endpoint, including its `X-Peer-ID` header.
3.  **Super-Peer Processes Heartbeat:**
    *   Receives the heartbeat request.
    *   Looks up the Peer by ID in its `peers` map.
    *   Updates the Peer's `LastSeen` timestamp and sets `IsOnline` to `true`.
4.  **Super-Peer Health Check Service:** The Super-Peer's `healthCheckService` (running as a goroutine) periodically checks all registered peers.
5.  **Mark Offline:** If a Peer's `LastSeen` timestamp is older than a defined cutoff (e.g., 2 minutes), the Super-Peer marks that Peer as `IsOnline = false` and logs a warning.
6.  **Update Statistics:** Both heartbeat and health check services trigger updates to the `NetworkStats` and broadcast these statistics via WebSocket.

**Pseudocode (Peer Heartbeat):**

```
FUNCTION heartbeatService(peer):
    LOOP FOREVER:
        IF peer.IsRegistered THEN
            Send_Heartbeat(peer)
        END IF
        WAIT peer.Config.Heartbeat_Interval seconds
    END LOOP

FUNCTION Send_Heartbeat(peer):
    request = CREATE_HTTP_POST_REQUEST(SuperPeer_Address + "/api/v1/peers/heartbeat")
    ADD_HEADER(request, "X-Peer-ID", peer.ID)
    response = EXECUTE_HTTP_REQUEST(request)
    IF response.Status == OK THEN
        peer.LastHeartbeat = CURRENT_TIME
    ELSE
        Log("Failed to send heartbeat to Super-Peer")
    END IF
```

**Pseudocode (Super-Peer Health Check):**

```
FUNCTION healthCheckService(super_peer):
    LOOP FOREVER:
        cutoff_time = CURRENT_TIME - 2 minutes

        Acquire peers_mutex_lock
        FOR EACH peer_id, peer IN super_peer.peers:
            IF peer.LastSeen < cutoff_time THEN
                peer.IsOnline = FALSE
                Log("Peer " + peer_id + " marked offline")
            END IF
        END FOR
        Release peers_mutex_lock

        Update_Network_Stats(super_peer)
        WAIT 30 seconds
    END LOOP
```

## 7. Data Structures

### Super-Peer (`main.go`)

*   **`SuperPeer` struct:**
    ```
    STRUCT SuperPeer:
        peers: MAP[string] -> Peer_Object // Stores active peers by ID
        files: MAP[string] -> FileInfo_Object // Stores registered files by ID
        peersMutex: RWMutex // For concurrent access to 'peers'
        filesMutex: RWMutex // For concurrent access to 'files'
        wsConnections: MAP[*websocket.Conn] -> bool // Active WebSocket connections
        wsMutex: RWMutex // For concurrent access to 'wsConnections'
        stats: NetworkStats_Object // Aggregated network statistics
    ```
*   **`Peer` struct (Super-Peer's view):**
    ```
    STRUCT Peer:
        ID: string
        Address: string
        Port: int
        LastSeen: time.Time
        IsOnline: bool
        Reputation: int
        SharedFiles: int
        Region: string
    ```
*   **`FileInfo` struct (Super-Peer's view):**
    ```
    STRUCT FileInfo:
        ID: string
        Filename: string
        Size: int64
        Hash: string
        Category: string
        Tags: []string
        Owner: string // Peer ID
        PeerAddress: string // "IP:Port" of the owning peer
        UploadTime: time.Time
        Downloads: int
        Rating: float64
    ```
*   **`NetworkStats` struct:**
    ```
    STRUCT NetworkStats:
        TotalPeers: int
        OnlinePeers: int
        TotalFiles: int
        TotalDownloads: int
        NetworkHealth: float64
        LastUpdated: time.Time
    ```

### Peer Node (`peer/peer_server.go`)

*   **`Peer` struct (Peer's own view):**
    ```
    STRUCT Peer:
        ID: string
        Address: string
        Port: int
        SharedFiles: MAP[string] -> SharedFile_Object // Files locally shared by this peer
        Config: PeerConfig_Object
        IsRegistered: bool
        LastHeartbeat: time.Time
        DownloadStats: DownloadStats_Object
        UploadStats: UploadStats_Object
        mutex: RWMutex // For concurrent access to peer's own state
    ```
*   **`PeerConfig` struct:**
    ```
    STRUCT PeerConfig:
        Port: int
        SuperPeerAddress: string
        SharedDirectory: string
        MaxFileSize: int64
        HeartbeatInterval: int
    ```
*   **`SharedFile` struct (Peer's own view):**
    ```
    STRUCT SharedFile:
        ID: string
        Filename: string
        FilePath: string // Local path to the file
        Size: int64
        Hash: string
        Category: string
        Tags: []string
        SharedAt: time.Time
        Downloads: int
        IsAvailable: bool
    ```
*   **`DownloadStats` / `UploadStats` structs:**
    ```
    STRUCT DownloadStats:
        TotalDownloads: int64
        TotalBytes: int64
        ActiveDownloads: int

    STRUCT UploadStats:
        TotalUploads: int64
        TotalBytes: int64
        ActiveUploads: int