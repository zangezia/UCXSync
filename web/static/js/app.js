// UCXSync Web Application
class UCXSyncApp {
    constructor() {
        this.ws = null;
        this.reconnectInterval = 5000;
        this.dashboardPollInterval = 2000;
        this.dashboardTimer = null;
        this.isRunning = false;
        this.mode = 'single';
        this.dashboardConfig = { enabled: false, instances: [] };
        this.lastOverview = null;

        this.initElements();
        this.initEventListeners();
        this.initialize();
    }

    async initialize() {
        await this.detectMode();
        this.loadSavedSettings();

        if (this.mode === 'dashboard') {
            this.enableDashboardMode();
            await Promise.all([
                this.loadDashboardProjects(),
                this.loadDashboardDestinations(),
                this.refreshDashboardOverview()
            ]);
            this.dashboardTimer = setInterval(() => this.refreshDashboardOverview(), this.dashboardPollInterval);
        } else {
            this.connectWebSocket();
            this.loadProjects();
            this.loadDestinations();
        }
    }

    initElements() {
        // Header / mode-specific elements
        this.subtitleEl = document.querySelector('.subtitle');
        this.metricsTitle = document.getElementById('metrics-title');
        this.instancesPanel = document.getElementById('instances-panel');
        this.instancesGrid = document.getElementById('instances-grid');

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
        this.networkPrimaryLabel = document.getElementById('network-primary-label');
        this.networkPrimaryProgress = document.getElementById('network-primary-progress');
        this.networkPrimaryValue = document.getElementById('network-primary-value');
        this.networkSecondaryLabel = document.getElementById('network-secondary-label');
        this.networkSecondaryProgress = document.getElementById('network-secondary-progress');
        this.networkSecondaryValue = document.getElementById('network-secondary-value');
        this.cpuTemperatureValue = document.getElementById('cpu-temperature-value');
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
        this.startBtn.addEventListener('click', () => {
            if (this.mode === 'dashboard') {
                this.startDashboardSync();
            } else {
                this.startSync();
            }
        });

        this.stopBtn.addEventListener('click', () => {
            if (this.mode === 'dashboard') {
                this.stopDashboardSync();
            } else {
                this.stopSync();
            }
        });

        this.refreshBtn.addEventListener('click', () => {
            if (this.mode === 'dashboard') {
                this.loadDashboardProjects();
                this.loadDashboardDestinations();
                this.refreshDashboardOverview();
            } else {
                this.loadProjects();
                this.loadDestinations();
            }
        });

        this.manageDevicesBtn.addEventListener('click', () => this.openDeviceModal());
        this.mountSharesBtn.addEventListener('click', () => {
            if (this.mode === 'dashboard') {
                this.mountDashboardShares();
            } else {
                this.mountShares();
            }
        });

        this.restartServiceBtn.addEventListener('click', () => {
            if (this.mode === 'dashboard') {
                this.restartDashboardServices();
            } else {
                this.restartService();
            }
        });

        // Auto-save settings
        this.projectSelect.addEventListener('change', () => this.saveSettings());
        this.destinationSelect.addEventListener('change', () => this.saveSettings());
        this.destinationCustom.addEventListener('change', () => this.saveSettings());
        this.parallelismInput.addEventListener('change', () => this.saveSettings());
    }

    async detectMode() {
        try {
            const response = await fetch('/api/dashboard/config');
            if (!response.ok) {
                return;
            }

            const config = await response.json();
            this.dashboardConfig = config;
            if (config.enabled && Array.isArray(config.instances) && config.instances.length > 0) {
                this.mode = 'dashboard';
            }
        } catch (error) {
            console.debug('Dashboard config unavailable, using single mode:', error);
        }
    }

    enableDashboardMode() {
        this.subtitleEl.textContent = 'Общий дашборд для двух инстансов UCXSync';
        this.metricsTitle.textContent = 'Производительность хоста';
        this.startBtn.textContent = '▶️ Запустить оба';
        this.stopBtn.textContent = '⏹️ Остановить оба';
        this.mountSharesBtn.textContent = '🔁 Смонтировать шары на обоих';
        this.restartServiceBtn.textContent = '♻️ Перезапустить оба сервиса';
        this.instancesPanel.hidden = false;
        this.log('Общий дашборд включен', 'info');
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
                this.updateSingleStatus(message.payload);
                break;
            case 'metrics':
                this.updateMetrics(message.payload);
                break;
            case 'log':
                this.log(message.payload.message, message.payload.level);
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

    async fetchJSON(url, options = {}) {
        const response = await fetch(url, options);
        if (!response.ok) {
            const text = await response.text();
            throw new Error(text || `HTTP ${response.status}`);
        }
        return response.json();
    }

    getCurrentDestination() {
        return this.destinationCustom.value.trim() || this.destinationSelect.value;
    }

    async loadProjects() {
        this.refreshBtn.disabled = true;
        this.log('Поиск проектов...', 'info');

        try {
            const projects = await this.fetchJSON('/api/projects');
            this.populateProjects(projects);
            this.log(`✓ Найдено проектов: ${projects.length}`, 'success');
        } catch (error) {
            this.log(`✗ Ошибка загрузки проектов: ${error.message}`, 'error');
        } finally {
            this.refreshBtn.disabled = false;
        }
    }

    async loadDashboardProjects() {
        this.refreshBtn.disabled = true;
        this.log('Поиск проектов на обоих инстансах...', 'info');

        try {
            const projects = await this.fetchJSON('/api/dashboard/projects');
            this.populateProjects(projects);
            this.log(`✓ Найдено общих проектов: ${projects.length}`, 'success');
        } catch (error) {
            this.log(`✗ Ошибка загрузки проектов общего дашборда: ${error.message}`, 'error');
        } finally {
            this.refreshBtn.disabled = false;
        }
    }

    populateProjects(projects) {
        const previousValue = this.projectSelect.value;
        this.projectSelect.innerHTML = '<option value="">-- Выберите проект --</option>';
        projects.forEach(project => {
            const option = document.createElement('option');
            option.value = project.name;
            option.textContent = project.source ? `${project.name} (${project.source})` : project.name;
            this.projectSelect.appendChild(option);
        });

        if (previousValue) {
            this.projectSelect.value = previousValue;
        }
    }

    async loadDestinations() {
        this.log('Поиск дисков...', 'info');

        try {
            const destinations = await this.fetchJSON('/api/destinations');
            this.populateDestinations(destinations);
            this.log(`✓ Найдено дисков: ${destinations.length}`, 'success');
        } catch (error) {
            this.log(`✗ Ошибка загрузки дисков: ${error.message}`, 'error');
        }
    }

    async loadDashboardDestinations() {
        this.log('Поиск дисков для общего дашборда...', 'info');

        try {
            const destinations = await this.fetchJSON('/api/dashboard/destinations');
            this.populateDestinations(destinations);
            this.log(`✓ Найдено дисков: ${destinations.length}`, 'success');
        } catch (error) {
            this.log(`✗ Ошибка загрузки дисков: ${error.message}`, 'error');
        }
    }

    populateDestinations(destinations) {
        const previousValue = this.destinationSelect.value;
        this.destinationSelect.innerHTML = '<option value="">-- Выберите диск --</option>';

        destinations.forEach(dest => {
            const option = document.createElement('option');
            option.value = dest.path;
            const icon = dest.type === 'usb' ? '💾' : '💿';
            const freeSpace = Number(dest.free_space_gb || 0).toFixed(1);
            const totalSpace = Number(dest.total_gb || 0).toFixed(1);
            option.textContent = `${icon} ${dest.label} - ${freeSpace}/${totalSpace} GB свободно`;
            if (dest.is_default) {
                option.selected = true;
            }
            this.destinationSelect.appendChild(option);
        });

        if (previousValue) {
            this.destinationSelect.value = previousValue;
        }
    }

    async startSync() {
        const project = this.projectSelect.value;
        const destination = this.getCurrentDestination();
        const parallelism = parseInt(this.parallelismInput.value, 10);

        if (!project || !destination) {
            this.log('✗ Укажите проект и папку назначения', 'error');
            return;
        }

        try {
            await this.fetchJSON('/api/sync/start', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ project, destination, max_parallelism: parallelism })
            });

            this.isRunning = true;
            this.updateControlsState();
            this.log(`✓ Синхронизация проекта '${project}' запущена`, 'success');
        } catch (error) {
            this.log(`✗ Ошибка запуска: ${error.message}`, 'error');
        }
    }

    async startDashboardSync(targets = []) {
        const project = this.projectSelect.value;
        const destination = this.getCurrentDestination();
        const parallelism = parseInt(this.parallelismInput.value, 10);

        if (!project || !destination) {
            this.log('✗ Укажите проект и папку назначения', 'error');
            return;
        }

        try {
            const response = await this.fetchJSON('/api/dashboard/sync/start', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    project,
                    destination,
                    max_parallelism: parallelism,
                    targets
                })
            });

            this.logDashboardActionResults('Запуск синхронизации', response.results);
            await this.refreshDashboardOverview();
        } catch (error) {
            this.log(`✗ Ошибка запуска общего дашборда: ${error.message}`, 'error');
        }
    }

    async stopSync() {
        try {
            await this.fetchJSON('/api/sync/stop', { method: 'POST' });
            this.isRunning = false;
            this.updateControlsState();
            this.log('✓ Синхронизация остановлена', 'info');
        } catch (error) {
            this.log(`✗ Ошибка остановки: ${error.message}`, 'error');
        }
    }

    async stopDashboardSync(targets = []) {
        try {
            const response = await this.fetchJSON('/api/dashboard/sync/stop', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ targets })
            });

            this.logDashboardActionResults('Остановка синхронизации', response.results);
            await this.refreshDashboardOverview();
        } catch (error) {
            this.log(`✗ Ошибка остановки общего дашборда: ${error.message}`, 'error');
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
            await this.fetchJSON('/api/shares/mount', { method: 'POST' });
            this.log('✓ Повторная попытка монтирования шар выполнена', 'success');
            await this.loadProjects();
        } catch (error) {
            this.log(`✗ Ошибка монтирования шар: ${error.message}`, 'error');
        } finally {
            this.mountSharesBtn.disabled = false;
        }
    }

    async mountDashboardShares(targets = []) {
        this.mountSharesBtn.disabled = true;
        try {
            const response = await this.fetchJSON('/api/dashboard/shares/mount', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ targets })
            });

            this.logDashboardActionResults('Повторное монтирование шар', response.results);
            await this.loadDashboardProjects();
            await this.refreshDashboardOverview();
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
            await this.fetchJSON('/api/service/restart', { method: 'POST' });
            this.log('↻ Команда на перезапуск службы отправлена', 'info');
        } catch (error) {
            this.log(`✗ Ошибка перезапуска службы: ${error.message}`, 'error');
            this.restartServiceBtn.disabled = false;
        }
    }

    async restartDashboardServices(targets = []) {
        if (!window.confirm('Перезапустить выбранные службы UCXSync? Общий дашборд может кратковременно стать недоступным.')) {
            return;
        }

        this.restartServiceBtn.disabled = true;
        try {
            const response = await this.fetchJSON('/api/dashboard/service/restart', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ targets })
            });

            this.logDashboardActionResults('Перезапуск служб', response.results);
        } catch (error) {
            this.log(`✗ Ошибка перезапуска служб: ${error.message}`, 'error');
        } finally {
            setTimeout(() => {
                this.restartServiceBtn.disabled = false;
                this.refreshDashboardOverview();
            }, 2000);
        }
    }

    updateSingleStatus(status) {
        this.isRunning = status.is_running;
        this.updateControlsState();
        this.completedCapturesEl.textContent = status.completed_captures || 0;
        this.lastCaptureEl.textContent = status.last_capture_number || '-';
        this.testCapturesEl.textContent = status.completed_test_captures || 0;
        this.activeOpsCountEl.textContent = status.active_file_operations || 0;
        this.maxParallelismEl.textContent = status.max_parallelism || 8;
        this.updateActiveOpsColor(status.active_file_operations || 0, status.max_parallelism || 0);
        this.updateActivityTable((status.active_tasks || []).map(task => ({ ...task, instance: '—' })));
    }

    async refreshDashboardOverview() {
        try {
            const overview = await this.fetchJSON('/api/dashboard/overview');
            this.lastOverview = overview;
            this.updateDashboardOverview(overview);
            this.updateConnectionStatus(true);
        } catch (error) {
            this.updateConnectionStatus(false);
            this.log(`✗ Ошибка обновления общего дашборда: ${error.message}`, 'error');
        }
    }

    updateDashboardOverview(overview) {
        this.updateDashboardSummary(overview.summary, overview.instances || []);
        this.updateMetrics(overview.host_metrics || {});
        this.renderInstanceCards(overview.instances || []);
        this.updateDashboardControlsState(overview.instances || []);
        this.updateDashboardActivity(overview.instances || []);
    }

    updateDashboardSummary(summary, instances) {
        this.completedCapturesEl.textContent = summary.total_completed_captures || 0;
        this.lastCaptureEl.textContent = this.formatDashboardLastCapture(instances);
        this.testCapturesEl.textContent = summary.total_completed_test_captures || 0;
        this.activeOpsCountEl.textContent = summary.total_active_file_operations || 0;
        this.maxParallelismEl.textContent = summary.total_max_parallelism || 0;
        this.updateActiveOpsColor(summary.total_active_file_operations || 0, summary.total_max_parallelism || 0);
    }

    formatDashboardLastCapture(instances) {
        const parts = instances
            .filter(instance => instance.available && instance.status.last_capture_number)
            .map(instance => `${instance.name}: ${instance.status.last_capture_number}`);
        return parts.length > 0 ? parts.join(' · ') : '-';
    }

    updateDashboardControlsState(instances) {
        const anyRunning = instances.some(instance => instance.available && instance.status.is_running);
        this.isRunning = anyRunning;
        this.updateControlsState();
    }

    renderInstanceCards(instances) {
        if (!instances.length) {
            this.instancesGrid.innerHTML = '<div class="no-data">Инстансы не настроены</div>';
            return;
        }

        this.instancesGrid.innerHTML = instances.map(instance => {
            const badgeClass = !instance.available
                ? 'error'
                : instance.status.is_running
                    ? 'running'
                    : 'stopped';
            const badgeText = !instance.available
                ? 'Недоступен'
                : instance.status.is_running
                    ? 'Работает'
                    : 'Остановлен';
            const cardClass = !instance.available
                ? 'instance-card unavailable'
                : instance.status.is_running
                    ? 'instance-card running'
                    : 'instance-card';
            const activeOps = instance.status.active_file_operations || 0;
            const maxParallelism = instance.status.max_parallelism || 0;

            return `
                <div class="${cardClass}">
                    <div class="instance-header">
                        <div>
                            <div class="instance-title">${this.escapeHtml(instance.name)}</div>
                            <div class="instance-url">${this.escapeHtml(instance.url)}</div>
                        </div>
                        <span class="instance-badge ${badgeClass}">${badgeText}</span>
                    </div>

                    ${instance.error ? `<div class="instance-error">${this.escapeHtml(instance.error)}</div>` : ''}

                    <div class="instance-grid">
                        <div class="instance-stat">
                            <div class="instance-stat-label">Проект</div>
                            <div class="instance-stat-value">${this.escapeHtml(instance.status.project || '—')}</div>
                        </div>
                        <div class="instance-stat">
                            <div class="instance-stat-label">Назначение</div>
                            <div class="instance-stat-value">${this.escapeHtml(instance.status.destination || '—')}</div>
                        </div>
                        <div class="instance-stat">
                            <div class="instance-stat-label">Снимков</div>
                            <div class="instance-stat-value">${instance.status.completed_captures || 0}</div>
                        </div>
                        <div class="instance-stat">
                            <div class="instance-stat-label">Тестовых</div>
                            <div class="instance-stat-value">${instance.status.completed_test_captures || 0}</div>
                        </div>
                        <div class="instance-stat">
                            <div class="instance-stat-label">Операций</div>
                            <div class="instance-stat-value">${activeOps} / ${maxParallelism}</div>
                        </div>
                        <div class="instance-stat">
                            <div class="instance-stat-label">Последний снимок</div>
                            <div class="instance-stat-value">${this.escapeHtml(instance.status.last_capture_number || '—')}</div>
                        </div>
                    </div>

                    <div class="instance-actions">
                        <button class="btn btn-primary" data-action="start-instance" data-instance-id="${this.escapeHtml(instance.id)}">▶️ Запустить</button>
                        <button class="btn btn-danger" data-action="stop-instance" data-instance-id="${this.escapeHtml(instance.id)}">⏹️ Остановить</button>
                        <button class="btn btn-secondary" data-action="mount-instance" data-instance-id="${this.escapeHtml(instance.id)}">🔁 Шары</button>
                        <button class="btn btn-secondary" data-action="restart-instance" data-instance-id="${this.escapeHtml(instance.id)}">♻️ Сервис</button>
                    </div>
                </div>
            `;
        }).join('');

        this.instancesGrid.querySelectorAll('[data-action]').forEach(button => {
            button.addEventListener('click', () => this.handleInstanceAction(button.dataset.action, button.dataset.instanceId));
        });
    }

    async handleInstanceAction(action, instanceId) {
        switch (action) {
            case 'start-instance':
                await this.startDashboardSync([instanceId]);
                break;
            case 'stop-instance':
                await this.stopDashboardSync([instanceId]);
                break;
            case 'mount-instance':
                await this.mountDashboardShares([instanceId]);
                break;
            case 'restart-instance':
                await this.restartDashboardServices([instanceId]);
                break;
            default:
                console.warn('Unknown instance action', action);
        }
    }

    updateMetrics(metrics) {
        const cpuPercent = Math.round(metrics.cpu_percent || 0);
        this.cpuProgress.style.width = `${cpuPercent}%`;
        this.cpuValue.textContent = `${cpuPercent}%`;

        if (metrics.cpu_temperature_available) {
            const cpuTemp = Number(metrics.cpu_temperature_celsius || 0).toFixed(1);
            this.cpuTemperatureValue.textContent = `${cpuTemp} °C`;
            if ((metrics.cpu_temperature_celsius || 0) >= 85) {
                this.cpuTemperatureValue.style.color = 'var(--danger-color)';
            } else if ((metrics.cpu_temperature_celsius || 0) >= 70) {
                this.cpuTemperatureValue.style.color = 'var(--warning-color)';
            } else {
                this.cpuTemperatureValue.style.color = 'var(--primary-color)';
            }
        } else {
            this.cpuTemperatureValue.textContent = 'N/A';
            this.cpuTemperatureValue.style.color = 'var(--text-secondary)';
        }

        const memPercent = Math.round(metrics.memory_percent || 0);
        const memUsedGB = ((metrics.memory_used_bytes || 0) / 1024 / 1024 / 1024).toFixed(1);
        const memTotalGB = ((metrics.memory_total_bytes || 0) / 1024 / 1024 / 1024).toFixed(1);
        this.memoryProgress.style.width = `${memPercent}%`;
        this.memoryValue.textContent = `${memUsedGB} GB / ${memTotalGB} GB`;

        const diskPercent = Math.min(100, Math.round(metrics.disk_percent || 0));
        const diskMBps = Number(metrics.disk_mbps || 0).toFixed(2);
        this.diskProgress.style.width = `${diskPercent}%`;
        this.diskValue.textContent = `${diskMBps} MB/s`;

        const netPercent = Math.min(100, Math.round(metrics.network_percent || 0));
        const netMBps = Number(metrics.network_mbps || 0).toFixed(2);
        this.networkProgress.style.width = `${netPercent}%`;
        this.networkValue.textContent = `${netMBps} MB/s`;

        const interfaceMetrics = this.selectNetworkInterfaces(metrics.network_interfaces || []);
        this.updateNetworkInterfaceCard(this.networkPrimaryLabel, this.networkPrimaryProgress, this.networkPrimaryValue, interfaceMetrics[0], 'Сеть #1');
        this.updateNetworkInterfaceCard(this.networkSecondaryLabel, this.networkSecondaryProgress, this.networkSecondaryValue, interfaceMetrics[1], 'Сеть #2');

        const freeDiskGB = Number(metrics.free_disk_gb || 0).toFixed(1);
        this.freeDiskEl.textContent = `${freeDiskGB} GB`;
    }

    selectNetworkInterfaces(interfaces) {
        const preferred = ['end0', 'end1'];
        const selected = [];
        const remaining = [...interfaces];

        preferred.forEach(name => {
            const index = remaining.findIndex(item => item.name === name);
            if (index >= 0) {
                selected.push(remaining[index]);
                remaining.splice(index, 1);
            }
        });

        remaining
            .sort((a, b) => (b.bytes_per_sec || 0) - (a.bytes_per_sec || 0))
            .forEach(item => {
                if (selected.length < 2) {
                    selected.push(item);
                }
            });

        return selected.slice(0, 2);
    }

    updateNetworkInterfaceCard(labelEl, progressEl, valueEl, metric, fallbackLabel) {
        if (!metric) {
            labelEl.textContent = fallbackLabel;
            progressEl.style.width = '0%';
            valueEl.textContent = 'Нет данных';
            return;
        }

        const percent = Math.min(100, Math.round(metric.percent || 0));
        const mbps = Number(metric.mbps || 0).toFixed(2);
        labelEl.textContent = `Сеть ${metric.name}`;
        progressEl.style.width = `${percent}%`;
        valueEl.textContent = `${mbps} MB/s`;
    }

    updateDashboardActivity(instances) {
        const rows = [];
        instances.forEach(instance => {
            (instance.status.active_tasks || []).forEach(task => {
                rows.push({ ...task, instance: instance.name });
            });
        });
        this.updateActivityTable(rows);
    }

    updateActivityTable(tasks) {
        if (tasks.length === 0) {
            this.activityBody.innerHTML = '<tr><td colspan="7" class="no-data">Нет активных задач</td></tr>';
            return;
        }

        this.activityBody.innerHTML = tasks.map(task => {
            const progress = task.progress ? `${Math.round(task.progress)}%` : '-';
            const lastActivity = task.last_activity ? new Date(task.last_activity).toLocaleTimeString() : '-';
            return `
                <tr>
                    <td>${this.escapeHtml(task.instance || '—')}</td>
                    <td>${this.escapeHtml(task.node || '')}</td>
                    <td>${this.escapeHtml(task.share || '')}</td>
                    <td>${this.escapeHtml(task.status || '')}</td>
                    <td>${task.copied_files || 0}</td>
                    <td>${progress}</td>
                    <td>${lastActivity}</td>
                </tr>
            `;
        }).join('');
    }

    updateActiveOpsColor(activeOps, maxParallelism) {
        const usage = maxParallelism > 0 ? (activeOps / maxParallelism) : 0;
        if (usage > 0.9) {
            this.activeOpsCountEl.style.color = 'var(--danger-color)';
        } else if (usage > 0.7) {
            this.activeOpsCountEl.style.color = 'var(--warning-color)';
        } else {
            this.activeOpsCountEl.style.color = 'var(--success-color)';
        }
    }

    logDashboardActionResults(prefix, results = []) {
        if (!results.length) {
            this.log(`✗ ${prefix}: нет результатов`, 'error');
            return;
        }

        results.forEach(result => {
            const text = result.success
                ? `✓ ${prefix}: ${result.name}`
                : `✗ ${prefix}: ${result.name} — ${result.error || 'ошибка'}`;
            this.log(text, result.success ? 'success' : 'error');
        });
    }

    log(message, level = 'info') {
        const timestamp = new Date().toLocaleTimeString();
        const entry = document.createElement('div');
        entry.className = 'log-entry';
        entry.innerHTML = `
            <span class="log-timestamp">[${timestamp}]</span>
            <span class="log-level ${level}">${level.toUpperCase()}</span>
            <span class="log-message">${this.escapeHtml(message)}</span>
        `;

        this.logContainer.appendChild(entry);
        this.logContainer.scrollTop = this.logContainer.scrollHeight;

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
                if (settings.project) this.projectSelect.value = settings.project;
                if (settings.destination) this.destinationSelect.value = settings.destination;
            } catch (error) {
                console.error('Failed to load saved settings:', error);
            }
        }
    }

    async openDeviceModal() {
        this.deviceModal.classList.add('active');
        await this.loadDevices();
    }

    async loadDevices() {
        this.devicesBody.innerHTML = '<tr><td colspan="6" class="no-data">Загрузка...</td></tr>';

        try {
            const devices = await this.fetchJSON('/api/devices');

            if (devices.length === 0) {
                this.devicesBody.innerHTML = '<tr><td colspan="6" class="no-data">Устройства не найдены</td></tr>';
                return;
            }

            this.devicesBody.innerHTML = devices.map(device => {
                const mountStatus = device.is_mounted
                    ? `<span class="device-mounted">✓ ${this.escapeHtml(device.mount_point)}</span>`
                    : '<span class="device-unmounted">Не смонтирован</span>';

                const actionBtn = device.is_mounted
                    ? `<button class="btn-unmount" onclick="app.unmountDevice('${this.escapeJs(device.device_path)}')">Размонтировать</button>`
                    : `<button class="btn-mount" onclick="app.mountDevice('${this.escapeJs(device.device_path)}')">Монтировать</button>`;

                const removableIcon = device.is_removable ? '💾 ' : '💿 ';

                return `
                    <tr>
                        <td>${removableIcon}${this.escapeHtml(device.device_name)}</td>
                        <td>${this.escapeHtml(device.size)}</td>
                        <td>${this.escapeHtml(device.fstype || '-')}</td>
                        <td>${this.escapeHtml(device.label || '-')}</td>
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
            await this.fetchJSON('/api/devices/mount', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ device_path: devicePath, action: 'mount' })
            });

            this.log(`✓ Устройство ${devicePath} смонтировано в /ucdata`, 'success');
            await this.loadDevices();
            if (this.mode === 'dashboard') {
                await this.loadDashboardDestinations();
            } else {
                await this.loadDestinations();
            }
        } catch (error) {
            this.log(`✗ Ошибка монтирования: ${error.message}`, 'error');
        }
    }

    async unmountDevice(devicePath) {
        try {
            await this.fetchJSON('/api/devices/mount', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ device_path: devicePath, action: 'unmount' })
            });

            this.log(`✓ Устройство ${devicePath} размонтировано`, 'success');
            await this.loadDevices();
            if (this.mode === 'dashboard') {
                await this.loadDashboardDestinations();
            } else {
                await this.loadDestinations();
            }
        } catch (error) {
            this.log(`✗ Ошибка размонтирования: ${error.message}`, 'error');
        }
    }

    escapeHtml(value) {
        return String(value ?? '')
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#39;');
    }

    escapeJs(value) {
        return String(value ?? '').replace(/\\/g, '\\\\').replace(/'/g, "\\'");
    }
}

function closeDeviceModal() {
    document.getElementById('device-modal').classList.remove('active');
}

function refreshDevices() {
    window.app.loadDevices();
}

document.addEventListener('DOMContentLoaded', () => {
    window.app = new UCXSyncApp();
});
