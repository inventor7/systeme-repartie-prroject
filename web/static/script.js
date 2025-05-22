document.addEventListener('DOMContentLoaded', () => {
    const peerIdElement = document.getElementById('peer-id');
    const peerAddressElement = document.getElementById('peer-address');
    const connectionStatusElement = document.getElementById('connection-status');
    const totalSharedFilesElement = document.getElementById('total-shared-files');
    const totalDownloadsElement = document.getElementById('total-downloads');
    const totalUploadsElement = document.getElementById('total-uploads');
    const filesGrid = document.getElementById('files-grid');
    const searchResultsGrid = document.getElementById('search-results-grid');
    const searchInput = document.getElementById('search-input');
    const searchButton = document.getElementById('search-btn');
    const filesFilter = document.getElementById('files-filter');
    const networkFilter = document.getElementById('network-filter');
    const uploadArea = document.getElementById('upload-area');
    const fileInput = document.getElementById('file-input');
    const uploadProgressContainer = document.getElementById('upload-progress');
    const uploadProgressBar = document.getElementById('upload-progress-bar');
    const uploadProgressText = document.getElementById('upload-progress-text');

    let peerInfo = {};
    let sharedFiles = [];
    let searchResults = [];

    // Tab switching logic
    document.querySelectorAll('.nav-tab').forEach(tab => {
        tab.addEventListener('click', () => {
            document.querySelectorAll('.nav-tab').forEach(t => t.classList.remove('active'));
            tab.classList.add('active');
            document.querySelectorAll('.content-section').forEach(section => section.classList.remove('active'));
            document.getElementById(`${tab.dataset.tab}-section`).classList.add('active');
        });
    });

    // Fetch peer info
    async function fetchPeerInfo() {
        try {
            const response = await fetch('/api/v1/info');
            peerInfo = await response.json();
            peerIdElement.textContent = peerInfo.id;
            peerAddressElement.textContent = `${peerInfo.address}:${peerInfo.port}`;
            connectionStatusElement.textContent = peerInfo.is_registered ? 'Online' : 'Offline';
            connectionStatusElement.classList.toggle('online', peerInfo.is_registered);
            connectionStatusElement.classList.toggle('offline', !peerInfo.is_registered);
            totalSharedFilesElement.textContent = peerInfo.shared_files;
            totalDownloadsElement.textContent = peerInfo.download_stats.TotalDownloads;
            totalUploadsElement.textContent = peerInfo.upload_stats.TotalUploads;
        } catch (error) {
            console.error('Error fetching peer info:', error);
            connectionStatusElement.textContent = 'Offline';
            connectionStatusElement.classList.remove('online');
            connectionStatusElement.classList.add('offline');
        }
    }

    // Fetch shared files
    async function fetchSharedFiles() {
        try {
            const response = await fetch('/api/v1/files');
            sharedFiles = await response.json();
            renderFiles(filesGrid, sharedFiles, true); // true for local files (show unshare)
        } catch (error) {
            console.error('Error fetching shared files:', error);
        }
    }

    // Render files to a grid
    function renderFiles(gridElement, files, isLocal = false) {
        gridElement.innerHTML = '';
        if (files.length === 0) {
            const emptyStateId = gridElement.id === 'files-grid' ? 'files-empty' : 'search-empty';
            document.getElementById(emptyStateId).style.display = 'block';
            return;
        } else {
            const emptyStateId = gridElement.id === 'files-grid' ? 'files-empty' : 'search-empty';
            document.getElementById(emptyStateId).style.display = 'none';
        }

        files.forEach(file => {
            const fileCard = document.createElement('div');
            fileCard.className = 'file-card';
            fileCard.innerHTML = `
                <div class="file-header">
                    <div class="file-icon ${getFileCategoryClass(file.category)}">
                        <i class="${getFileIcon(file.category)}"></i>
                    </div>
                    <div class="file-details">
                        <h4>${file.filename}</h4>
                        <p>${formatBytes(file.size)}</p>
                    </div>
                </div>
                <div class="file-meta">
                    <span>Owner: ${file.owner ? file.owner.substring(0, 8) + '...' : 'N/A'}</span>
                    <span>Downloads: ${file.downloads || 0}</span>
                </div>
                <div class="file-actions">
                    <button class="btn btn-secondary view-info-btn" data-file-id="${file.id}" data-is-local="${isLocal}">
                        <i class="fas fa-info-circle"></i> Info
                    </button>
                    <button class="btn btn-primary download-btn" data-file-id="${file.id}" data-peer-address="${file.peer_address}" data-filename="${file.filename}">
                        <i class="fas fa-download"></i> Download
                    </button>
                    ${isLocal ? `
                        <button class="btn btn-secondary share-link-btn" data-file-id="${file.id}">
                            <i class="fas fa-share-alt"></i> Share Link
                        </button>
                        <button class="btn btn-danger unshare-btn" data-file-id="${file.id}">
                            <i class="fas fa-trash"></i> Unshare
                        </button>
                    ` : ''}
                </div>
            `;
            gridElement.appendChild(fileCard);
        });

        // Add event listeners for the new "View Info" buttons
        document.querySelectorAll('.view-info-btn').forEach(button => {
            button.addEventListener('click', (event) => {
                const fileId = event.target.dataset.fileId;
                const isLocal = event.target.dataset.isLocal === 'true';
                viewFileInfo(fileId, isLocal);
            });
        });

        // Add event listeners for Download buttons (now on both local and search results)
        document.querySelectorAll('.download-btn').forEach(button => {
            button.addEventListener('click', (event) => downloadFile(event.target.dataset.fileId, event.target.dataset.peerAddress, event.target.dataset.filename));
        });

        // Add event listeners for Share Link buttons
        document.querySelectorAll('.share-link-btn').forEach(button => {
            button.addEventListener('click', (event) => {
                const fileId = event.target.dataset.fileId;
                const shareLink = `http://localhost:8080/download/${fileId}`; // Construct the share link
                navigator.clipboard.writeText(shareLink).then(() => {
                    alert('Share link copied to clipboard!'); // Provide feedback
                }).catch(err => {
                    console.error('Failed to copy share link:', err);
                    alert('Failed to copy share link.');
                });
            });
        });

        if (isLocal) {
            document.querySelectorAll('.unshare-btn').forEach(button => {
                button.addEventListener('click', (event) => unshareFile(event.target.dataset.fileId));
            });
        }
    }

    // Helper functions for file icons and categories
    function getFileCategoryClass(category) {
        return category ? category.toLowerCase().replace(/\s/g, '-') : 'other';
    }

    function getFileIcon(category) {
        switch (category ? category.toLowerCase() : '') {
            case 'document': return 'fas fa-file-alt';
            case 'image': return 'fas fa-image';
            case 'video': return 'fas fa-video';
            case 'audio': return 'fas fa-music';
            case 'archive': return 'fas fa-archive';
            default: return 'fas fa-file';
        }
    }

    function formatBytes(bytes, decimals = 2) {
        if (bytes === 0) return '0 Bytes';
        const k = 1024;
        const dm = decimals < 0 ? 0 : decimals;
        const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
    }

    // File upload logic
    uploadArea.addEventListener('dragover', (event) => {
        event.preventDefault();
        uploadArea.classList.add('dragover');
    });

    uploadArea.addEventListener('dragleave', () => {
        uploadArea.classList.remove('dragover');
    });

    uploadArea.addEventListener('drop', (event) => {
        event.preventDefault();
        uploadArea.classList.remove('dragover');
        const files = event.dataTransfer.files;
        if (files.length > 0) {
            uploadFile(files[0]);
        }
    });

    fileInput.addEventListener('change', (event) => {
        if (event.target.files.length > 0) {
            uploadFile(event.target.files[0]);
        }
    });

    async function uploadFile(file) {
        const formData = new FormData();
        formData.append('file', file);

        uploadProgressContainer.style.display = 'block';
        uploadProgressBar.style.width = '0%';
        uploadProgressText.textContent = 'Uploading... 0%';

        try {
            const xhr = new XMLHttpRequest();
            xhr.open('POST', '/api/v1/files/share', true);

            xhr.upload.addEventListener('progress', (event) => {
                if (event.lengthComputable) {
                    const percent = (event.loaded / event.total) * 100;
                    uploadProgressBar.style.width = `${percent}%`;
                    uploadProgressText.textContent = `Uploading... ${percent.toFixed(2)}%`;
                }
            });

            xhr.addEventListener('load', () => {
                if (xhr.status === 200) {
                    uploadProgressText.textContent = 'Upload Complete!';
                    setTimeout(() => {
                        uploadProgressContainer.style.display = 'none';
                        fetchSharedFiles(); // Refresh shared files list
                    }, 1000);
                } else {
                    uploadProgressText.textContent = `Upload Failed: ${xhr.statusText}`;
                    console.error('Upload failed:', xhr.status, xhr.statusText);
                }
            });

            xhr.addEventListener('error', () => {
                uploadProgressText.textContent = 'Upload Failed: Network Error';
                console.error('Upload network error');
            });

            xhr.send(formData);

        } catch (error) {
            console.error('Error uploading file:', error);
            uploadProgressText.textContent = 'Upload Failed';
        }
    }

    // Unshare file logic
    async function unshareFile(fileId) {
        if (!confirm('Are you sure you want to unshare this file?')) {
            return;
        }
        try {
            const response = await fetch(`/api/v1/files/unshare/${fileId}`, {
                method: 'DELETE',
            });
            if (response.ok) {
                console.log(`File ${fileId} unshared successfully.`);
                fetchSharedFiles(); // Refresh list
            } else {
                console.error('Failed to unshare file:', response.statusText);
            }
        } catch (error) {
            console.error('Error unsharing file:', error);
        }
    }

    // Search files logic
    searchButton.addEventListener('click', performSearch);
    searchInput.addEventListener('keypress', (event) => {
        if (event.key === 'Enter') {
            performSearch();
        }
    });

    async function performSearch() {
        const query = searchInput.value;
        const category = networkFilter.value;
        try {
            const response = await fetch(`/api/v1/search?q=${encodeURIComponent(query)}&category=${encodeURIComponent(category)}`);
            const data = await response.json();
            searchResults = data.results;
            renderFiles(searchResultsGrid, searchResults, false); // false for remote files (show download)
        } catch (error) {
            console.error('Error performing search:', error);
        }
    }

    // Download file logic (client-side initiation)
    function downloadFile(fileId, peerAddress, filename) {
        // The Super-Peer will redirect the browser to the actual peer's download endpoint.
        // So, we just navigate the browser to the Super-Peer's download endpoint.
        const downloadUrl = `http://localhost:8080/download/${fileId}`;
        window.open(downloadUrl, '_blank');
        console.log(`Attempting to download ${filename} from ${peerAddress} via Super-Peer.`);
    }

    // View file information in modal
    function viewFileInfo(fileId, isLocal) {
        const file = isLocal
            ? sharedFiles.find(f => f.id === fileId)
            : searchResults.find(f => f.id === fileId);

        if (!file) {
            console.error('File not found for viewing info:', fileId);
            return;
        }

        document.getElementById('modal-file-title').textContent = file.filename;
        document.getElementById('modal-filename').textContent = file.filename;
        document.getElementById('modal-filesize').textContent = formatBytes(file.size);
        document.getElementById('modal-filecategory').textContent = file.category || 'Other';
        document.getElementById('modal-filehash').textContent = file.hash || 'N/A';
        document.getElementById('modal-fileowner').textContent = file.owner || 'N/A';
        document.getElementById('modal-peeraddress').textContent = file.peer_address || 'N/A';
        document.getElementById('modal-uploadtime').textContent = file.upload_time ? new Date(file.upload_time).toLocaleString() : 'N/A';
        document.getElementById('modal-downloads').textContent = file.downloads || 0;
        document.getElementById('modal-islocal').textContent = isLocal ? 'Yes' : 'No';

        document.getElementById('file-info-modal').style.display = 'block';
    }

    // Close the file information modal
    function closeModal() {
        document.getElementById('file-info-modal').style.display = 'none';
    }

    // Close modal when clicking outside of it
    window.onclick = function (event) {
        const modal = document.getElementById('file-info-modal');
        if (event.target === modal) {
            modal.style.display = "none";
        }
    }


    // View file information in modal
    function viewFileInfo(fileId, isLocal) {
        const file = isLocal
            ? sharedFiles.find(f => f.id === fileId)
            : searchResults.find(f => f.id === fileId);

        if (!file) {
            console.error('File not found for viewing info:', fileId);
            return;
        }

        document.getElementById('modal-file-title').textContent = file.filename;
        document.getElementById('modal-filename').textContent = file.filename;
        document.getElementById('modal-filesize').textContent = formatBytes(file.size);
        document.getElementById('modal-filecategory').textContent = file.category || 'Other';
        document.getElementById('modal-filehash').textContent = file.hash || 'N/A';
        document.getElementById('modal-fileowner').textContent = file.owner || 'N/A';
        document.getElementById('modal-peeraddress').textContent = file.peer_address || 'N/A';
        document.getElementById('modal-uploadtime').textContent = file.upload_time ? new Date(file.upload_time).toLocaleString() : 'N/A';
        document.getElementById('modal-downloads').textContent = file.downloads || 0;
        document.getElementById('modal-islocal').textContent = isLocal ? 'Yes' : 'No';

        document.getElementById('file-info-modal').style.display = 'block';
    }

    // Close the file information modal
    function closeModal() {
        document.getElementById('file-info-modal').style.display = 'none';
    }

    // Close modal when clicking outside of it
    window.onclick = function (event) {
        const modal = document.getElementById('file-info-modal');
        if (event.target === modal) {
            modal.style.display = "none";
        }
    }


    // Initial fetches
    fetchPeerInfo();
    fetchSharedFiles();
    performSearch(); // Initial search to show all files on network

    // WebSocket for real-time updates (optional, if peer server supports it)
    // const ws = new WebSocket(`ws://localhost:${peerInfo.port}/ws`);
    // ws.onmessage = (event) => {
    //     const message = JSON.parse(event.data);
    //     console.log('WebSocket message:', message);
    //     if (message.type === 'file_added' || message.type === 'file_removed') {
    //         fetchSharedFiles();
    //     }
    //     // Handle other real-time updates like download progress if implemented
    // };
    // ws.onclose = () => console.log('WebSocket disconnected');
    // ws.onerror = (error) => console.error('WebSocket error:', error);
});