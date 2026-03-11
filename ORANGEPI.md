# UCXSync на Orange Pi RV2

Руководство по установке и настройке UCXSync на Orange Pi RV2 (RISC-V) с Ubuntu Server 24.04.

## 🍊 Характеристики Orange Pi RV2

- **CPU**: Spacemit K1 (8-core RISC-V @ 2.0GHz)
- **Architecture**: RISC-V 64-bit (RV64GC)
- **RAM**: 4GB / 8GB / 16GB LPDDR4X
- **Storage**: eMMC + microSD + M.2 NVMe
- **Network**: Gigabit Ethernet
- **USB**: 2× USB 3.0, 2× USB 2.0

## 📋 Требования

### Системные требования

- Ubuntu Server 24.04 LTS (ARM64)
- Минимум 2GB RAM (рекомендуется 4GB+)
- Минимум 500MB свободного места для системы
- Внешний USB 3.0 диск для хранения данных (рекомендуется)

### Сетевые требования

- Gigabit Ethernet подключение (обязательно проводное!)
- Доступ к сети UCX узлов (WU01-WU13, CU)
- Статический IP или DHCP резервация (рекомендуется)

## 🚀 Быстрая установка

### 1. Подготовка Orange Pi

```bash
# Обновление системы
sudo apt update
sudo apt upgrade -y

# Установка необходимых пакетов
sudo apt install -y git build-essential cifs-utils

# Установка Go для RISC-V (если нужна сборка)
# Примечание: Официальный Go для RISC-V может быть недоступен,
# используйте репозиторий Ubuntu или соберите из исходников
sudo apt install -y golang-go

# Или скачайте последнюю версию (если доступна):
# wget https://go.dev/dl/go1.21.5.linux-riscv64.tar.gz
# sudo tar -C /usr/local -xzf go1.21.5.linux-riscv64.tar.gz
# echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
# source ~/.bashrc
```

### 2. Клонирование репозитория

```bash
git clone https://github.com/zangezia/UCXSync.git
cd UCXSync
```

### 3. Автоматическая установка

```bash
sudo chmod +x install-orangepi.sh
sudo ./install-orangepi.sh
```

### 4. Настройка конфигурации

```bash
sudo nano /etc/ucxsync/config.yaml
```

Измените учетные данные:
```yaml
credentials:
  username: ВашЛогин
  password: ВашПароль
```

### 5. Запуск

```bash
# Запуск сервиса
sudo systemctl start ucxsync

# Автозапуск при загрузке
sudo systemctl enable ucxsync

# Проверка статуса
sudo systemctl status ucxsync
```

### 6. Доступ к веб-интерфейсу

Откройте браузер и перейдите по адресу:
```
http://<IP-адрес-Orange-Pi>:8080
```

Узнать IP-адрес:
```bash
hostname -I
```

## ⚙️ Оптимизация для Orange Pi

### Рекомендуемые настройки (`/etc/ucxsync/config.yaml`)

```yaml
sync:
  max_parallelism: 4  # Оптимально для ARM и USB

monitoring:
  performance_update_interval: 2s  # Экономия CPU
  ui_update_interval: 3s
  cpu_smoothing_samples: 5
  max_disk_throughput_mbps: 40.0  # USB 3.0 realistic
```

### Производительность

**Типичная производительность на Orange Pi RV2 (RISC-V):**

| Сценарий | Скорость | CPU | Температура |
|----------|----------|-----|-------------|
| USB 3.0 HDD | 25-35 MB/s | 25-35% | 40-50°C |
| USB 3.0 SSD | 60-100 MB/s | 35-45% | 45-55°C |
| NVMe SSD | 150-300 MB/s | 45-65% | 50-60°C |

**Примечание:** RISC-V производительность может отличаться от ARM из-за разной зрелости компилятора и оптимизаций.

### Мониторинг температуры

```bash
# Текущая температура CPU
cat /sys/class/thermal/thermal_zone0/temp | awk '{print $1/1000 "°C"}'

# Непрерывный мониторинг
watch -n 1 'cat /sys/class/thermal/thermal_zone0/temp | awk "{print \$1/1000 \"°C\"}"'
```

**Критические температуры:**
- < 65°C: Нормально
- 65-75°C: Повышенная нагрузка
- \> 75°C: Риск троттлинга, добавьте охлаждение!

### Охлаждение

**Обязательно используйте:**
- Радиатор на процессоре
- Активный вентилятор для 24/7 работы
- Корпус с вентиляцией

## 💾 Настройка хранилища

### Вариант 1: USB 3.0 диск (рекомендуется для начала)

```bash
# Автомонтирование USB диска
sudo nano /etc/fstab

# Добавьте строку:
UUID=ваш-uuid /ucdata ext4 defaults,nofail 0 2

# Узнать UUID:
sudo blkid
```

### Вариант 2: NVMe SSD (максимальная производительность)

Orange Pi RV2 поддерживает M.2 NVMe через переходник:

```bash
# Форматирование NVMe
sudo mkfs.ext4 /dev/nvme0n1

# Монтирование
sudo mkdir -p /mnt/nvme
sudo mount /dev/nvme0n1 /mnt/nvme

# Автомонтирование
echo "UUID=$(sudo blkid -s UUID -o value /dev/nvme0n1) /mnt/nvme ext4 defaults 0 2" | sudo tee -a /etc/fstab
```

## 🔧 Типичные проблемы

### Проблема: Низкая скорость копирования

**Решение:**
1. Проверьте USB порт (используйте USB 3.0, синий порт)
2. Снизьте `max_parallelism` до 2-4
3. Убедитесь в стабильности сети

```yaml
sync:
  max_parallelism: 2  # Попробуйте снизить
```

### Проблема: Высокая температура CPU

**Решение:**
1. Установите радиатор и вентилятор
2. Снизьте частоту обновления метрик:

```yaml
monitoring:
  performance_update_interval: 5s
  ui_update_interval: 10s
```

3. Ограничьте параллелизм:

```yaml
sync:
  max_parallelism: 2
```

### Проблема: Сбой монтирования сетевых дисков

**Решение:**
1. Проверьте доступность узлов:

```bash
ping WU01
ping WU02
# и т.д.
```

2. Проверьте учетные данные в `/etc/ucxsync/config.yaml`

3. Вручную смонтируйте для теста:

```bash
sudo mount -t cifs //WU01/E$ /mnt/test -o username=Administrator,password=ultracam,vers=1.0
```

### Проблема: Приложение вылетает / зависает

**Решение:**
1. Проверьте логи:

```bash
sudo journalctl -u ucxsync -f
```

2. Проверьте ресурсы:

```bash
htop
df -h
free -h
```

3. Перезапустите сервис:

```bash
sudo systemctl restart ucxsync
```

## 📊 Мониторинг

### Системные метрики

```bash
# CPU, память, диск
htop

# Сетевые подключения
sudo netstat -tupln | grep ucxsync

# Использование диска
df -h

# I/O активность
iostat -x 1
```

### Логи UCXSync

```bash
# Реальное время
sudo journalctl -u ucxsync -f

# Последние 100 строк
sudo journalctl -u ucxsync -n 100

# Только ошибки
sudo journalctl -u ucxsync -p err

# За сегодня
sudo journalctl -u ucxsync --since today
```

### Веб-интерфейс

Доступен по адресу: `http://<IP>:8080`

**Метрики в реальном времени:**
- CPU загрузка
- Использование памяти
- Скорость диска (MB/s)
- Сетевой трафик
- Активные операции копирования
- Завершенные captures

## 🔐 Безопасность

### Файрвол

```bash
# Разрешить доступ к веб-интерфейсу только из локальной сети
sudo ufw allow from 192.168.1.0/24 to any port 8080
sudo ufw enable
```

### Смена пароля администратора

```bash
sudo nano /etc/ucxsync/config.yaml
# Измените credentials.password
sudo systemctl restart ucxsync
```

### Права доступа

```bash
# Ограничить доступ к конфигурации
sudo chmod 600 /etc/ucxsync/config.yaml
sudo chown root:root /etc/ucxsync/config.yaml
```

## 🛠 Обслуживание

### Обновление UCXSync

```bash
cd UCXSync
git pull
sudo make build-arm64
sudo cp ucxsync-arm64 /opt/ucxsync/ucxsync
sudo systemctl restart ucxsync
```

### Очистка логов

```bash
# Ограничить размер журнала systemd
sudo journalctl --vacuum-size=100M
sudo journalctl --vacuum-time=7d
```

### Резервное копирование конфигурации

```bash
# Создать backup
sudo cp /etc/ucxsync/config.yaml ~/ucxsync-config-backup.yaml

# Восстановить
sudo cp ~/ucxsync-config-backup.yaml /etc/ucxsync/config.yaml
sudo systemctl restart ucxsync
```

## 📈 Советы по производительности

### 1. Используйте проводное соединение
Wi-Fi нестабилен для непрерывной синхронизации больших файлов.

### 2. Статический IP
Упростит доступ к веб-интерфейсу:

```bash
# /etc/netplan/01-netcfg.yaml
network:
  version: 2
  renderer: networkd
  ethernets:
    eth0:
      addresses: [192.168.1.100/24]
      gateway4: 192.168.1.1
      nameservers:
        addresses: [8.8.8.8, 8.8.4.4]
```

### 3. Оптимизация I/O для ext4

```bash
# В /etc/fstab добавьте noatime
UUID=xxx /ucdata ext4 defaults,noatime,nofail 0 2
```

### 4. Мониторинг производительности

Установите дополнительные инструменты:

```bash
sudo apt install -y iotop nethogs sysstat
```

## 🆘 Поддержка

**Логи для отчета об ошибке:**

```bash
# Собрать диагностическую информацию
sudo journalctl -u ucxsync -n 500 > ucxsync-logs.txt
uname -a >> ucxsync-logs.txt
free -h >> ucxsync-logs.txt
df -h >> ucxsync-logs.txt
```

**Полезные команды:**

```bash
# Полный статус
sudo systemctl status ucxsync -l --no-pager

# Проверка монтирования
mount | grep ucx

# Температура и частота CPU
cat /sys/devices/system/cpu/cpu*/cpufreq/scaling_cur_freq
```

## ✅ Checklist перед запуском

- [ ] Ubuntu Server 24.04 установлена и обновлена
- [ ] Проводное Ethernet подключение работает
- [ ] cifs-utils установлен
- [ ] Учетные данные в config.yaml настроены
- [ ] Внешний диск подключен и смонтирован
- [ ] Есть доступ к UCX узлам (ping работает)
- [ ] Радиатор/вентилятор установлен
- [ ] Веб-интерфейс доступен

Готово к работе! 🚀
