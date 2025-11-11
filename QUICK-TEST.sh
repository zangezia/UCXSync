#!/bin/bash
# Быстрый тест UCXSync на Ubuntu Server
# Использование: chmod +x QUICK-TEST.sh && ./QUICK-TEST.sh

set -e

echo "=== UCXSync Quick Test Script ==="
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
echo "=== Создание директорий ==="
sudo mkdir -p /mnt/storage/ucx
sudo mkdir -p /mnt/ucx
sudo chown -R $USER:$USER /mnt/storage/ucx
echo "✓ Директории созданы:"
echo "   /mnt/storage/ucx - хранилище"
echo "   /mnt/ucx - точки монтирования"

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
echo "✓ Установка завершена успешно!"
echo "================================================================"
echo ""
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
