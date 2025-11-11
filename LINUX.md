# UCXSync для Linux AMD64/x86_64

Руководство по установке UCXSync на стандартных Linux серверах (Ubuntu, Debian, CentOS, RHEL).

## 📋 Системные требования

### Минимальные требования
- **OS**: Ubuntu 20.04+, Debian 11+, CentOS 8+, RHEL 8+
- **CPU**: 2+ ядра (рекомендуется 4+)
- **RAM**: 2GB (рекомендуется 4GB+)
- **Storage**: 500MB для системы + место для данных
- **Network**: Gigabit Ethernet

### Оптимальная конфигурация
- **CPU**: Intel Xeon / AMD EPYC, 4+ ядра
- **RAM**: 8GB+
- **Storage**: NVMe SSD или быстрый RAID
- **Network**: 10 Gigabit Ethernet

## 🚀 Быстрая установка

### Автоматическая установка (рекомендуется)

```bash
# Клонирование репозитория
git clone https://github.com/zangezia/UCXSync.git
cd UCXSync

# Запуск установщика
sudo chmod +x install.sh
sudo ./install.sh
```

Скрипт автоматически:
- ✅ Определит архитектуру (AMD64)
- ✅ Установит Go (если не установлен)
- ✅ Установит cifs-utils
- ✅ Скомпилирует приложение
- ✅ Создаст необходимые директории
- ✅ Настроит systemd service

### Ручная установка

```bash
# 1. Установка зависимостей
sudo apt update
sudo apt install -y git build-essential cifs-utils

# 2. Установка Go (если нужно)
wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# 3. Клонирование и сборка
git clone https://github.com/zangezia/UCXSync.git
cd UCXSync
make build

# 4. Установка
sudo mkdir -p /opt/ucxsync /etc/ucxsync /var/log/ucxsync /mnt/ucx
sudo cp ucxsync /opt/ucxsync/
sudo cp -r web /opt/ucxsync/
sudo cp config.example.yaml /etc/ucxsync/config.yaml

# 5. Настройка systemd
sudo cp ucxsync.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable ucxsync
```

## ⚙️ Конфигурация

### Базовая настройка

Отредактируйте `/etc/ucxsync/config.yaml`:

```yaml
# Учетные данные для доступа к сетевым узлам
credentials:
  username: Administrator
  password: YourPassword

# Настройки синхронизации
sync:
  max_parallelism: 8  # Для AMD64 можно использовать больше
  
# Веб-интерфейс
web:
  host: 0.0.0.0  # Доступ со всех интерфейсов
  port: 8080
```

### Оптимизация для AMD64

**Высокопроизводительный сервер:**
```yaml
sync:
  max_parallelism: 16  # Больше параллельных операций

monitoring:
  performance_update_interval: 1s  # Чаще обновления
  ui_update_interval: 1s
  max_disk_throughput_mbps: 500.0  # NVMe SSD
```

**Стандартный сервер:**
```yaml
sync:
  max_parallelism: 8

monitoring:
  max_disk_throughput_mbps: 200.0  # SATA SSD
```

**Офисный компьютер:**
```yaml
sync:
  max_parallelism: 4

monitoring:
  max_disk_throughput_mbps: 100.0  # HDD
```

## 📊 Производительность AMD64

### Типичная производительность

| Конфигурация | Скорость | CPU | RAM |
|--------------|----------|-----|-----|
| HDD (SATA) | 80-120 MB/s | 5-10% | 50 MB |
| SSD (SATA) | 300-500 MB/s | 10-20% | 60 MB |
| SSD (NVMe) | 800-2000 MB/s | 15-30% | 70 MB |
| RAID 0 NVMe | 2000-5000 MB/s | 30-50% | 100 MB |

### Факторы производительности

**Узкие места:**
1. **Сеть** (1 Gbps = ~120 MB/s max)
2. **Диск назначения**
3. **Параллелизм** (max_parallelism)
4. **Latency сети** до UCX узлов

**Оптимизация:**
```bash
# Проверка скорости сети
iperf3 -c WU01

# Проверка скорости диска
dd if=/dev/zero of=/mnt/storage/test.dat bs=1M count=1024 oflag=direct

# Мониторинг I/O
iostat -x 1
```

## 🔧 Использование

### Запуск сервиса

```bash
# Запуск
sudo systemctl start ucxsync

# Автозапуск при загрузке
sudo systemctl enable ucxsync

# Статус
sudo systemctl status ucxsync

# Логи
sudo journalctl -u ucxsync -f
```

### Доступ к веб-интерфейсу

```
http://localhost:8080
```

или с другого компьютера:
```
http://<server-ip>:8080
```

### Команды управления

```bash
# Монтирование сетевых дисков
sudo /opt/ucxsync/ucxsync mount

# Размонтирование
sudo /opt/ucxsync/ucxsync unmount

# Проверка требований
sudo /opt/ucxsync/ucxsync check

# Версия
/opt/ucxsync/ucxsync --version
```

## 🐧 Дистрибутивы Linux

### Ubuntu / Debian

```bash
# Установка зависимостей
sudo apt update
sudo apt install -y cifs-utils

# Автозапуск
sudo systemctl enable ucxsync
```

### CentOS / RHEL / Rocky Linux

```bash
# Установка зависимостей
sudo yum install -y cifs-utils
# или
sudo dnf install -y cifs-utils

# Firewall (если включен)
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --reload

# SELinux (если включен)
sudo setsebool -P httpd_can_network_connect 1
```

### Arch Linux

```bash
# Установка зависимостей
sudo pacman -S cifs-utils

# Systemd
sudo systemctl enable ucxsync
```

## 🔐 Безопасность

### Firewall

**UFW (Ubuntu/Debian):**
```bash
# Разрешить только из локальной сети
sudo ufw allow from 192.168.0.0/16 to any port 8080
sudo ufw enable
```

**FirewallD (CentOS/RHEL):**
```bash
sudo firewall-cmd --permanent --add-rich-rule='rule family="ipv4" source address="192.168.0.0/16" port port="8080" protocol="tcp" accept'
sudo firewall-cmd --reload
```

### SSL/TLS (опционально)

Для HTTPS используйте reverse proxy (nginx или Apache):

```nginx
# /etc/nginx/sites-available/ucxsync
server {
    listen 443 ssl;
    server_name ucxsync.example.com;

    ssl_certificate /etc/ssl/certs/ucxsync.crt;
    ssl_certificate_key /etc/ssl/private/ucxsync.key;

    location / {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
    }
}
```

## 📈 Мониторинг

### Системные метрики

```bash
# CPU и память
htop

# Дисковая активность
iotop -o

# Сетевая активность
nethogs

# Статистика диска
iostat -x 1

# Состояние монтирования
mount | grep ucx
```

### Логи

```bash
# Логи приложения (последние 100 строк)
sudo journalctl -u ucxsync -n 100

# Логи в реальном времени
sudo journalctl -u ucxsync -f

# Логи за сегодня
sudo journalctl -u ucxsync --since today

# Только ошибки
sudo journalctl -u ucxsync -p err
```

### Prometheus / Grafana (расширенный мониторинг)

UCXSync можно интегрировать с Prometheus для продвинутого мониторинга:

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'ucxsync'
    static_configs:
      - targets: ['localhost:8080']
```

## 🚨 Troubleshooting

### Высокая нагрузка на CPU

**Решение:**
```yaml
sync:
  max_parallelism: 4  # Снизить параллелизм
```

### Медленная синхронизация

**Диагностика:**
```bash
# Проверка скорости сети
ping -c 10 WU01
iperf3 -c WU01

# Проверка скорости диска
hdparm -Tt /dev/sda

# Проверка I/O wait
iostat -x 1
```

### Ошибки монтирования

**Проверка доступности узлов:**
```bash
# Проверка DNS
nslookup WU01

# Проверка SMB
smbclient -L //WU01 -U Administrator

# Ручное монтирование для теста
sudo mount -t cifs //WU01/E$ /mnt/test -o username=Administrator,password=pass,vers=1.0
```

### Проблемы с памятью

**Оптимизация:**
```bash
# Ограничение памяти systemd
sudo systemctl edit ucxsync

# Добавить:
[Service]
MemoryLimit=500M
```

## 💡 Советы по оптимизации

### 1. Использование SSD кэша (bcache)

```bash
# Установка bcache
sudo apt install bcache-tools

# Настройка (пример)
sudo make-bcache -B /dev/sda1 -C /dev/nvme0n1
```

### 2. Оптимизация сети

```bash
# Увеличение TCP буферов
sudo sysctl -w net.core.rmem_max=134217728
sudo sysctl -w net.core.wmem_max=134217728
```

### 3. Оптимизация файловой системы

```bash
# ext4 с большими блоками
sudo mkfs.ext4 -b 4096 -E stride=128,stripe-width=128 /dev/sda1
```

### 4. NUMA оптимизация (многопроцессорные серверы)

```bash
# Проверка NUMA
numactl --hardware

# Запуск на конкретном NUMA узле
sudo numactl --cpunodebind=0 --membind=0 /opt/ucxsync/ucxsync
```

## 📚 Дополнительные ресурсы

- [Основная документация](README.md)
- [Управление параллелизмом](PARALLELISM.md)
- [Архитектура проекта](ARCHITECTURE.md)
- [Сборка из исходников](BUILD.md)

## ✅ Checklist установки

- [ ] Ubuntu/Debian/CentOS установлен и обновлен
- [ ] cifs-utils установлен
- [ ] Go 1.21+ установлен
- [ ] Приложение собрано и установлено
- [ ] Конфигурация настроена
- [ ] Systemd service активирован
- [ ] Firewall настроен
- [ ] Доступ к UCX узлам проверен
- [ ] Веб-интерфейс доступен

**Готово к работе!** 🚀
