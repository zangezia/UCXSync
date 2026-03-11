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
        this.destinationSelect = document.getElementById('destination');
        this.destinationCustom = document.getElementById('destination-custom');
        this.parallelismInput = document.getElementById('parallelism');
        this.startBtn = document.getElementById('start-btn');
        this.stopBtn = document.getElementById('stop-btn');
        this.refreshBtn = document.getElementById('refresh-projects');
        this.manageDevicesBtn = document.getElementById('manage-devices-btn');
        this.mountSharesBtn = document.getElementById('mount-shares-btn');
        this.restartServiceBtn = document.getElementById('restart-service-btn');

        // Status
        this.completedCapturesEl = document.getElementById('completed-captures');
        this.lastCaptureEl = document.getElementById('last-capture');
        this.testCapturesEl = document.getElementById('test-captures');
        this.activeOpsCountEl = document.getElementById('active-ops-count');
        this.maxParallelismEl = document.getElementById('max-parallelism');

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

        // Device modal
        this.deviceModal = document.getElementById('device-modal');
        this.devicesBody = document.getElementById('devices-body');
    }

    initEventListeners() {
        this.startBtn.addEventListener('click', () => this.startSync());
        this.stopBtn.addEventListener('click', () => this.stopSync());
        this.refreshBtn.addEventListener('click', () => {
            this.loadProjects();
            this.loadDestinations();
        });
        this.manageDevicesBtn.addEventListener('click', () => this.openDeviceModal());
        this.mountSharesBtn.addEventListener('click', () => this.mountShares());
        this.restartServiceBtn.addEventListener('click', () => this.restartService());

        // Auto-save settings
        this.projectSelect.addEventListener('change', () => this.saveSettings());
        this.destinationSelect.addEventListener('change', () => this.saveSettings());
        this.destinationCustom.addEventListener('change', () => this.saveSettings());
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

    async loadDestinations() {
        this.log('Поиск дисков...', 'info');

        try {
            const response = await fetch('/api/destinations');
            if (!response.ok) throw new Error('Failed to load destinations');

            const destinations = await response.json();
            
            this.destinationSelect.innerHTML = '<option value="">-- Выберите диск --</option>';
            destinations.forEach(dest => {
                const option = document.createElement('option');
                option.value = dest.path;
                
                const icon = dest.type === 'usb' ? '💾' : '💿';
                const freeSpace = dest.free_space_gb.toFixed(1);
                const totalSpace = dest.total_gb.toFixed(1);
                
                option.textContent = `${icon} ${dest.label} - ${freeSpace}/${totalSpace} GB свободно`;
                
                if (dest.is_default) {
                    option.selected = true;
                }
                
                this.destinationSelect.appendChild(option);
            });

            this.log(`✓ Найдено дисков: ${destinations.length}`, 'success');
        } catch (error) {
            this.log(`✗ Ошибка загрузки дисков: ${error.message}`, 'error');
        }
    }

    async startSync() {
        const project = this.projectSelect.value;
        let destination = this.destinationSelect.value;
        
        // If custom path is entered, use it instead
        if (this.destinationCustom.value.trim()) {
            destination = this.destinationCustom.value.trim();
        }
        
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
                this.log(`✗ Ошибка запуска: ${error}`, 'error');
                return;
            }

            this.isRunning = true;
            this.updateControlsState();
            this.log(`✓ Синхронизация проекта '${project}' запущена`, 'success');
        } catch (error) {
            this.log(`✗ Ошибка запуска: ${error.message}`, 'error');
        }
    }

    async stopSync() {
        try {
            const response = await fetch('/api/sync/stop', { method: 'POST' });
            
            if (!response.ok) {
                const error = await response.text();
                this.log(`✗ Ошибка остановки: ${error}`, 'error');
                return;
            }

            this.isRunning = false;
            this.updateControlsState();
            this.log('✓ Синхронизация остановлена', 'info');
        } catch (error) {
            this.log(`✗ Ошибка остановки: ${error.message}`, 'error');
        }
    }

    updateControlsState() {
        this.startBtn.disabled = this.isRunning;
        this.stopBtn.disabled = !this.isRunning;
        this.projectSelect.disabled = this.isRunning;
        this.destinationSelect.disabled = this.isRunning;
        this.destinationCustom.disabled = this.isRunning;
        this.parallelismInput.disabled = this.isRunning;
    }

    async mountShares() {
        this.mountSharesBtn.disabled = true;

        try {
            const response = await fetch('/api/shares/mount', { method: 'POST' });
            if (!response.ok) {
                const error = await response.text();
                throw new Error(error);
            }

            this.log('✓ Повторная попытка монтирования шар выполнена', 'success');
            await this.loadProjects();
        } catch (error) {
            this.log(`✗ Ошибка монтирования шар: ${error.message}`, 'error');
        } finally {
            this.mountSharesBtn.disabled = false;
        }
    }

    async restartService() {
        if (!window.confirm('Перезапустить службу UCXSync? Соединение с веб-интерфейсом будет временно разорвано.')) {
            return;
        }

        this.restartServiceBtn.disabled = true;

        try {
            const response = await fetch('/api/service/restart', { method: 'POST' });
            if (!response.ok) {
                const error = await response.text();
                throw new Error(error);
            }

            this.log('↻ Команда на перезапуск службы отправлена', 'info');
        } catch (error) {
            this.log(`✗ Ошибка перезапуска службы: ${error.message}`, 'error');
            this.restartServiceBtn.disabled = false;
        }
    }

    updateStatus(status) {
        this.isRunning = status.is_running;
        this.updateControlsState();

        // Update status cards
        this.completedCapturesEl.textContent = status.completed_captures || 0;
        this.lastCaptureEl.textContent = status.last_capture_number || '-';
        this.testCapturesEl.textContent = status.completed_test_captures || 0;
        
        // Update parallelism info
        this.activeOpsCountEl.textContent = status.active_file_operations || 0;
        this.maxParallelismEl.textContent = status.max_parallelism || 8;
        
        // Color code based on usage
        const usage = status.max_parallelism > 0 ? (status.active_file_operations / status.max_parallelism) : 0;
        if (usage > 0.9) {
            this.activeOpsCountEl.style.color = 'var(--danger-color)';
        } else if (usage > 0.7) {
            this.activeOpsCountEl.style.color = 'var(--warning-color)';
        } else {
            this.activeOpsCountEl.style.color = 'var(--success-color)';
        }

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
            destination: this.destinationSelect.value,
            destinationCustom: this.destinationCustom.value,
            parallelism: this.parallelismInput.value
        };
        localStorage.setItem('ucxsync_settings', JSON.stringify(settings));
    }

    loadSavedSettings() {
        const saved = localStorage.getItem('ucxsync_settings');
        if (saved) {
            try {
                const settings = JSON.parse(saved);
                if (settings.destinationCustom) this.destinationCustom.value = settings.destinationCustom;
                if (settings.parallelism) this.parallelismInput.value = settings.parallelism;
            } catch (error) {
                console.error('Failed to load saved settings:', error);
            }
        }

        // Load projects and destinations on startup
        this.loadProjects();
        this.loadDestinations();
    }

    // Device management methods
    async openDeviceModal() {
        this.deviceModal.classList.add('active');
        await this.loadDevices();
    }

    async loadDevices() {
        this.devicesBody.innerHTML = '<tr><td colspan="6" class="no-data">Загрузка...</td></tr>';

        try {
            const response = await fetch('/api/devices');
            if (!response.ok) throw new Error('Failed to load devices');

            const devices = await response.json();

            if (devices.length === 0) {
                this.devicesBody.innerHTML = '<tr><td colspan="6" class="no-data">Устройства не найдены</td></tr>';
                return;
            }

            this.devicesBody.innerHTML = devices.map(device => {
                const mountStatus = device.is_mounted 
                    ? `<span class="device-mounted">✓ ${device.mount_point}</span>`
                    : `<span class="device-unmounted">Не смонтирован</span>`;

                const actionBtn = device.is_mounted
                    ? `<button class="btn-unmount" onclick="app.unmountDevice('${device.device_path}')">Размонтировать</button>`
                    : `<button class="btn-mount" onclick="app.mountDevice('${device.device_path}')">Монтировать</button>`;

                const removableIcon = device.is_removable ? '💾 ' : '💿 ';

                return `
                    <tr>
                        <td>${removableIcon}${device.device_name}</td>
                        <td>${device.size}</td>
                        <td>${device.fstype || '-'}</td>
                        <td>${device.label || '-'}</td>
                        <td>${mountStatus}</td>
                        <td>${actionBtn}</td>
                    </tr>
                `;
            }).join('');
        } catch (error) {
            this.log(`✗ Ошибка загрузки устройств: ${error.message}`, 'error');
            this.devicesBody.innerHTML = '<tr><td colspan="6" class="no-data">Ошибка загрузки устройств</td></tr>';
        }
    }

    async mountDevice(devicePath) {
        try {
            const response = await fetch('/api/devices/mount', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ device_path: devicePath, action: 'mount' })
            });

            if (!response.ok) {
                const error = await response.text();
                throw new Error(error);
            }

            this.log(`✓ Устройство ${devicePath} смонтировано в /ucdata`, 'success');
            
            // Reload devices and destinations
            await this.loadDevices();
            await this.loadDestinations();
        } catch (error) {
            this.log(`✗ Ошибка монтирования: ${error.message}`, 'error');
        }
    }

    async unmountDevice(devicePath) {
        try {
            const response = await fetch('/api/devices/mount', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ device_path: devicePath, action: 'unmount' })
            });

            if (!response.ok) {
                const error = await response.text();
                throw new Error(error);
            }

            this.log(`✓ Устройство ${devicePath} размонтировано`, 'success');
            
            // Reload devices and destinations
            await this.loadDevices();
            await this.loadDestinations();
        } catch (error) {
            this.log(`✗ Ошибка размонтирования: ${error.message}`, 'error');
        }
    }
}

// Global functions for modal
function closeDeviceModal() {
    document.getElementById('device-modal').classList.remove('active');
}

function refreshDevices() {
    window.app.loadDevices();
}

// Initialize app when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    window.app = new UCXSyncApp();
});
