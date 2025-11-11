#!/bin/bash
# Быстрый тест UCXSync на Ubuntu Server
# Использование: chmod +x QUICK-TEST.sh && ./QUICK-TEST.sh

set -e

echo "=== UCXSync Quick Test Script ==="
echo ""

# Удаляем предыдущую установку если была
echo "=== Checking for previous installation ==="
if [ -f "/usr/local/bin/ucxsync" ] || [ -d "/etc/ucxsync" ]; then
    echo "⚠ Found previous installation. Removing..."
    sudo systemctl stop ucxsync 2>/dev/null || true
    sudo systemctl disable ucxsync 2>/dev/null || true
    sudo rm -f /usr/local/bin/ucxsync
    sudo rm -rf /etc/ucxsync
    sudo rm -f /etc/systemd/system/ucxsync.service
    sudo systemctl daemon-reload
    echo "✓ Previous installation removed"
else
    echo "✓ No previous installation found"
fi
echo ""

# Определяем архитектуру
ARCH=$(uname -m)
echo "✓ Архитектура: $ARCH"

# Проверяем наличие Git
if ! command -v git &> /dev/null; then
    echo "✗ Git не установлен. Устанавливаем..."
    sudo apt-get update && sudo apt-get install -y git
fi
echo "✓ Git установлен"

# Клонируем репозиторий (если еще не клонирован)
if [ ! -d "UCXSync" ]; then
    echo "Клонируем репозиторий..."
    git clone https://github.com/zangezia/UCXSync.git
    cd UCXSync
else
    echo "✓ Репозиторий уже клонирован"
    cd UCXSync
    git pull
fi

# Проверяем Go
if ! command -v go &> /dev/null; then
    echo "✗ Go не установлен. Запускаем установщик..."
    chmod +x install.sh
    sudo ./install.sh
else
    GO_VERSION=$(go version | awk '{print $3}')
    echo "✓ Go установлен: $GO_VERSION"
fi

# Собираем приложение
echo ""
echo "=== Сборка приложения ==="
case $ARCH in
    x86_64)
        make build
        echo "✓ Собрано для AMD64"
        ;;
    aarch64)
        make build-arm64
        echo "✓ Собрано для ARM64"
        ;;
    riscv64)
        make build-riscv64
        echo "✓ Собрано для RISC-V64"
        ;;
    *)
        echo "✗ Неподдерживаемая архитектура: $ARCH"
        exit 1
        ;;
esac

# Устанавливаем бинарник
echo ""
echo "=== Установка ==="
sudo cp ucxsync /usr/local/bin/
sudo chmod +x /usr/local/bin/ucxsync
echo "✓ Бинарник установлен в /usr/local/bin/ucxsync"

# Создаем конфигурацию
echo ""
echo "=== Настройка конфигурации ==="
sudo mkdir -p /etc/ucxsync

if [ ! -f /etc/ucxsync/config.yaml ]; then
    sudo cp config.example.yaml /etc/ucxsync/config.yaml
    echo "✓ Конфигурация создана: /etc/ucxsync/config.yaml"
    echo ""
    echo "⚠ ВАЖНО: Отредактируйте конфигурацию:"
    echo "   sudo nano /etc/ucxsync/config.yaml"
    echo ""
    echo "Измените:"
    echo "  - IP адреса UCX узлов"
    echo "  - Имена пользователей и пароли"
    echo "  - Пути к директориям назначения"
else
    echo "✓ Конфигурация уже существует"
fi

# Создаем директории
echo ""
echo "=== Creating directories ==="
sudo mkdir -p /mnt/storage /mnt/ucx
echo "✓ Created /mnt/storage (USB-SSD mount point)"
echo "✓ Created /mnt/ucx (UCX network mount points)"

# Проверяем USB-SSD
if mountpoint -q /mnt/storage 2>/dev/null; then
    echo "✓ /mnt/storage is already mounted"
    sudo mkdir -p /mnt/storage/ucx
    sudo chown -R $USER:$USER /mnt/storage/ucx
    echo "✓ Created /mnt/storage/ucx for data"
    
    # Показываем информацию о диске
    STORAGE_INFO=$(df -h /mnt/storage 2>/dev/null | tail -1 | awk '{print $2 " total, " $4 " free"}')
    echo "Storage: $STORAGE_INFO"
else
    echo "⚠ /mnt/storage is NOT mounted"
    echo ""
    echo "USB-SSD is required for UCXSync!"
    echo ""
    echo "Quick mount:"
    echo "  1. lsblk                                    # find your USB-SSD (e.g., sda1)"
    echo "  2. sudo mount /dev/sda1 /mnt/storage        # mount it"
    echo "  3. sudo mkdir -p /mnt/storage/ucx           # create data dir"
    echo "  4. sudo chown -R \$USER:\$USER /mnt/storage/ucx  # set permissions"
    echo ""
    echo "See USB-SSD-GUIDE.md for details"
    echo ""
    read -p "Continue without USB-SSD? (yes/no): " -r
    if [[ ! $REPLY =~ ^[Yy][Ee][Ss]$ ]]; then
        echo "Installation cancelled. Please mount USB-SSD first."
        exit 1
    fi
fi

# Проверяем версию
echo ""
echo "=== Проверка установки ==="
ucxsync version

# Проверяем конфигурацию (если команда существует)
echo ""
echo "=== Информация о системе ==="
echo "OS: $(lsb_release -ds 2>/dev/null || cat /etc/os-release | grep PRETTY_NAME | cut -d'"' -f2)"
echo "Kernel: $(uname -r)"
echo "CPU: $(lscpu | grep "Model name" | cut -d':' -f2 | xargs)"
echo "RAM: $(free -h | awk '/^Mem:/ {print $2}')"
echo "Disk: $(df -h /mnt/storage | awk 'NR==2 {print $2 " total, " $4 " free"}')"

# Устанавливаем cifs-utils если нужно
echo ""
echo "=== Проверка зависимостей ==="
if ! dpkg -l | grep -q cifs-utils; then
    echo "Устанавливаем cifs-utils..."
    sudo apt-get update && sudo apt-get install -y cifs-utils
fi
echo "✓ cifs-utils установлен"

# Устанавливаем systemd сервис
echo ""
echo "=== Настройка systemd сервиса ==="
sudo cp ucxsync.service /etc/systemd/system/
sudo systemctl daemon-reload
echo "✓ Systemd сервис установлен"

echo ""
echo "================================================================"
echo "✓ UCXSync has been installed successfully!"
echo "================================================================"
echo ""

# Проверка USB-SSD
if ! mountpoint -q /mnt/storage 2>/dev/null; then
    echo "⚠ WARNING: USB-SSD is NOT mounted!"
    echo ""
    echo "UCXSync requires USB-SSD at /mnt/storage"
    echo ""
    echo "Mount your USB-SSD:"
    echo "  1. lsblk"
    echo "  2. sudo mount /dev/sdX1 /mnt/storage"
    echo "  3. sudo mkdir -p /mnt/storage/ucx"
    echo "  4. sudo chown -R \$USER:\$USER /mnt/storage/ucx"
    echo ""
    echo "See: USB-SSD-GUIDE.md"
    echo ""
fi

echo "Следующие шаги:"
echo ""
echo "1. Отредактируйте конфигурацию:"
echo "   sudo nano /etc/ucxsync/config.yaml"
echo ""
echo "2. Тестовый запуск (НЕ как сервис):"
echo "   ucxsync serve --config /etc/ucxsync/config.yaml"
echo ""
echo "3. Откройте веб-интерфейс:"
echo "   http://$(hostname -I | awk '{print $1}'):8080"
echo ""
echo "4. После успешного теста, запустите как сервис:"
echo "   sudo systemctl enable ucxsync"
echo "   sudo systemctl start ucxsync"
echo "   sudo systemctl status ucxsync"
echo ""
echo "5. Просмотр логов:"
echo "   sudo journalctl -u ucxsync -f"
echo ""
echo "Подробная инструкция: cat TEST.md"
echo "================================================================"
