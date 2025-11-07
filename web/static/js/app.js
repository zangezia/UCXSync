// UCXSync Web Application
class UCXSyncApp {
    constructor() {
        this.ws = null;
        this.reconnectInterval = 5000;
        this.isRunning = false;

        this.initElements();
        this.initEventListeners();
        this.connectWebSocket();
        this.loadSavedSettings();
    }

    initElements() {
        // Controls
        this.projectSelect = document.getElementById('project');
        this.destinationInput = document.getElementById('destination');
        this.parallelismInput = document.getElementById('parallelism');
        this.startBtn = document.getElementById('start-btn');
        this.stopBtn = document.getElementById('stop-btn');
        this.refreshBtn = document.getElementById('refresh-projects');

        // Status
        this.completedCapturesEl = document.getElementById('completed-captures');
        this.lastCaptureEl = document.getElementById('last-capture');
        this.testCapturesEl = document.getElementById('test-captures');
        this.syncStatusEl = document.getElementById('sync-status');

        // Metrics
        this.cpuProgress = document.getElementById('cpu-progress');
        this.cpuValue = document.getElementById('cpu-value');
        this.memoryProgress = document.getElementById('memory-progress');
        this.memoryValue = document.getElementById('memory-value');
        this.diskProgress = document.getElementById('disk-progress');
        this.diskValue = document.getElementById('disk-value');
        this.networkProgress = document.getElementById('network-progress');
        this.networkValue = document.getElementById('network-value');
        this.freeDiskEl = document.getElementById('free-disk');

        // Activity table
        this.activityBody = document.getElementById('activity-body');

        // Log
        this.logContainer = document.getElementById('log-container');

        // Connection status
        this.connectionStatus = document.getElementById('connection-status');
    }

    initEventListeners() {
        this.startBtn.addEventListener('click', () => this.startSync());
        this.stopBtn.addEventListener('click', () => this.stopSync());
        this.refreshBtn.addEventListener('click', () => this.loadProjects());

        // Auto-save settings
        this.projectSelect.addEventListener('change', () => this.saveSettings());
        this.destinationInput.addEventListener('change', () => this.saveSettings());
        this.parallelismInput.addEventListener('change', () => this.saveSettings());
    }

    connectWebSocket() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws`;

        this.log('Подключение к серверу...', 'info');

        this.ws = new WebSocket(wsUrl);

        this.ws.onopen = () => {
            this.log('✓ Подключено к серверу', 'success');
            this.updateConnectionStatus(true);
        };

        this.ws.onclose = () => {
            this.log('✗ Соединение закрыто', 'error');
            this.updateConnectionStatus(false);
            setTimeout(() => this.connectWebSocket(), this.reconnectInterval);
        };

        this.ws.onerror = (error) => {
            this.log('Ошибка WebSocket', 'error');
            console.error('WebSocket error:', error);
        };

        this.ws.onmessage = (event) => {
            try {
                const message = JSON.parse(event.data);
                this.handleWebSocketMessage(message);
            } catch (error) {
                console.error('Failed to parse WebSocket message:', error);
            }
        };
    }

    handleWebSocketMessage(message) {
        switch (message.type) {
            case 'status':
                this.updateStatus(message.payload);
                break;
            case 'metrics':
                this.updateMetrics(message.payload);
                break;
            case 'log':
                const log = message.payload;
                this.log(log.message, log.level);
                break;
            default:
                console.log('Unknown message type:', message.type);
        }
    }

    updateConnectionStatus(connected) {
        if (connected) {
            this.connectionStatus.textContent = '🟢 Подключено';
            this.connectionStatus.style.color = 'var(--success-color)';
        } else {
            this.connectionStatus.textContent = '🔴 Отключено';
            this.connectionStatus.style.color = 'var(--danger-color)';
        }
    }

    async loadProjects() {
        this.refreshBtn.disabled = true;
        this.log('Поиск проектов...', 'info');

        try {
            const response = await fetch('/api/projects');
            if (!response.ok) throw new Error('Failed to load projects');

            const projects = await response.json();
            
            this.projectSelect.innerHTML = '<option value="">-- Выберите проект --</option>';
            projects.forEach(project => {
                const option = document.createElement('option');
                option.value = project.name;
                option.textContent = `${project.name} (${project.source})`;
                this.projectSelect.appendChild(option);
            });

            this.log(`✓ Найдено проектов: ${projects.length}`, 'success');
        } catch (error) {
            this.log(`✗ Ошибка загрузки проектов: ${error.message}`, 'error');
        } finally {
            this.refreshBtn.disabled = false;
        }
    }

    async startSync() {
        const project = this.projectSelect.value;
        const destination = this.destinationInput.value;
        const parallelism = parseInt(this.parallelismInput.value);

        if (!project || !destination) {
            this.log('✗ Укажите проект и папку назначения', 'error');
            return;
        }

        try {
            const response = await fetch('/api/sync/start', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ project, destination, max_parallelism: parallelism })
            });

            if (!response.ok) {
                const error = await response.text();
                throw new Error(error);
            }

            this.isRunning = true;
            this.updateControlsState();
        } catch (error) {
            this.log(`✗ Ошибка запуска: ${error.message}`, 'error');
        }
    }

    async stopSync() {
        try {
            const response = await fetch('/api/sync/stop', { method: 'POST' });
            if (!response.ok) throw new Error('Failed to stop sync');

            this.isRunning = false;
            this.updateControlsState();
        } catch (error) {
            this.log(`✗ Ошибка остановки: ${error.message}`, 'error');
        }
    }

    updateControlsState() {
        this.startBtn.disabled = this.isRunning;
        this.stopBtn.disabled = !this.isRunning;
        this.projectSelect.disabled = this.isRunning;
        this.destinationInput.disabled = this.isRunning;
        this.parallelismInput.disabled = this.isRunning;
    }

    updateStatus(status) {
        this.isRunning = status.is_running;
        this.updateControlsState();

        // Update status cards
        this.completedCapturesEl.textContent = status.completed_captures || 0;
        this.lastCaptureEl.textContent = status.last_capture_number || '-';
        this.testCapturesEl.textContent = status.completed_test_captures || 0;
        this.syncStatusEl.textContent = status.is_running ? 'Работает' : 'Остановлено';
        this.syncStatusEl.style.color = status.is_running ? 'var(--success-color)' : 'var(--text-secondary)';

        // Update activity table
        this.updateActivityTable(status.active_tasks || []);
    }

    updateMetrics(metrics) {
        // CPU
        const cpuPercent = Math.round(metrics.cpu_percent || 0);
        this.cpuProgress.style.width = `${cpuPercent}%`;
        this.cpuValue.textContent = `${cpuPercent}%`;

        // Memory
        const memPercent = Math.round(metrics.memory_percent || 0);
        const memUsedGB = (metrics.memory_used_bytes / 1024 / 1024 / 1024).toFixed(1);
        const memTotalGB = (metrics.memory_total_bytes / 1024 / 1024 / 1024).toFixed(1);
        this.memoryProgress.style.width = `${memPercent}%`;
        this.memoryValue.textContent = `${memUsedGB} GB / ${memTotalGB} GB`;

        // Disk
        const diskPercent = Math.min(100, Math.round(metrics.disk_percent || 0));
        const diskMBps = (metrics.disk_mbps || 0).toFixed(2);
        this.diskProgress.style.width = `${diskPercent}%`;
        this.diskValue.textContent = `${diskMBps} MB/s`;

        // Network
        const netPercent = Math.min(100, Math.round(metrics.network_percent || 0));
        const netMBps = (metrics.network_mbps || 0).toFixed(2);
        this.networkProgress.style.width = `${netPercent}%`;
        this.networkValue.textContent = `${netMBps} MB/s`;

        // Free disk
        const freeDiskGB = (metrics.free_disk_gb || 0).toFixed(1);
        this.freeDiskEl.textContent = `${freeDiskGB} GB`;
    }

    updateActivityTable(tasks) {
        if (tasks.length === 0) {
            this.activityBody.innerHTML = '<tr><td colspan="6" class="no-data">Нет активных задач</td></tr>';
            return;
        }

        this.activityBody.innerHTML = tasks.map(task => {
            const progress = task.progress ? `${Math.round(task.progress)}%` : '-';
            const lastActivity = task.last_activity ? new Date(task.last_activity).toLocaleTimeString() : '-';

            return `
                <tr>
                    <td>${task.node}</td>
                    <td>${task.share}</td>
                    <td>${task.status}</td>
                    <td>${task.copied_files}</td>
                    <td>${progress}</td>
                    <td>${lastActivity}</td>
                </tr>
            `;
        }).join('');
    }

    log(message, level = 'info') {
        const timestamp = new Date().toLocaleTimeString();
        const entry = document.createElement('div');
        entry.className = 'log-entry';
        entry.innerHTML = `
            <span class="log-timestamp">[${timestamp}]</span>
            <span class="log-level ${level}">${level.toUpperCase()}</span>
            <span class="log-message">${message}</span>
        `;

        this.logContainer.appendChild(entry);
        this.logContainer.scrollTop = this.logContainer.scrollHeight;

        // Keep only last 100 entries
        while (this.logContainer.children.length > 100) {
            this.logContainer.removeChild(this.logContainer.firstChild);
        }
    }

    saveSettings() {
        const settings = {
            project: this.projectSelect.value,
            destination: this.destinationInput.value,
            parallelism: this.parallelismInput.value
        };
        localStorage.setItem('ucxsync_settings', JSON.stringify(settings));
    }

    loadSavedSettings() {
        const saved = localStorage.getItem('ucxsync_settings');
        if (saved) {
            try {
                const settings = JSON.parse(saved);
                if (settings.destination) this.destinationInput.value = settings.destination;
                if (settings.parallelism) this.parallelismInput.value = settings.parallelism;
            } catch (error) {
                console.error('Failed to load saved settings:', error);
            }
        }

        // Load projects on startup
        this.loadProjects();
    }
}

// Initialize app when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    window.app = new UCXSyncApp();
});
