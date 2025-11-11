# Шпаргалка для тестирования на ноутбуке с Ubuntu Server

## Один файл - вся установка

```bash
# Клонируем и запускаем автоустановку
git clone https://github.com/zangezia/UCXSync.git
cd UCXSync
chmod +x QUICK-TEST.sh
./QUICK-TEST.sh
```

## Или поэтапно

### 0️⃣ Удаление старой версии (если была)
```bash
# Если UCXSync уже установлен - удаляем
sudo systemctl stop ucxsync 2>/dev/null
sudo systemctl disable ucxsync 2>/dev/null
sudo rm -f /usr/local/bin/ucxsync
sudo rm -rf /etc/ucxsync
sudo rm -f /etc/systemd/system/ucxsync.service
sudo systemctl daemon-reload
```

### 1️⃣ Клонирование
```bash
git clone https://github.com/zangezia/UCXSync.git
cd UCXSync
```

### 2️⃣ Установка (автоматом)
```bash
chmod +x install.sh
sudo ./install.sh
```

### 3️⃣ Настройка
```bash
# Копируем конфиг
sudo mkdir -p /etc/ucxsync
sudo cp config.example.yaml /etc/ucxsync/config.yaml

# Редактируем (ОБЯЗАТЕЛЬНО!)
sudo nano /etc/ucxsync/config.yaml
```

**Что менять в config.yaml:**
```yaml
# 1. Параллелизм (для AMD64 ноутбука)
sync:
  max_parallelism: 6  # 4-8 для ноутбука

# 2. Путь к хранилищу
destinations:
  - path: "/mnt/storage/ucx"
    min_free_gb: 100

# 3. IP адреса UCX узлов
nodes:
  ucx01:
    ip: "192.168.1.101"  # ← ИЗМЕНИТЬ!
    username: "admin"     # ← ИЗМЕНИТЬ!
    password: "password"  # ← ИЗМЕНИТЬ!
    shares: ["share1"]
```

### 4️⃣ Создание директорий
```bash
sudo mkdir -p /mnt/storage/ucx
sudo mkdir -p /mnt/ucx
sudo chown $USER:$USER /mnt/storage/ucx
```

### 5️⃣ Первый запуск (тест БЕЗ сервиса)
```bash
ucxsync serve --config /etc/ucxsync/config.yaml
```

**Должно быть:**
```
INFO Starting UCXSync
INFO Web server listening on :8080
INFO Monitoring service started
```

### 6️⃣ Открыть веб-интерфейс
```
http://localhost:8080
```
Или с другого компьютера:
```
http://<IP-ноутбука>:8080
```

Узнать IP:
```bash
ip a | grep inet
# или
hostname -I
```

### 7️⃣ Тест синхронизации
В веб-интерфейсе нажмите кнопку синхронизации или через CLI:
```bash
ucxsync sync --node ucx01 --share share1
```

### 8️⃣ Запуск как сервис (после успешного теста)
```bash
sudo systemctl enable ucxsync
sudo systemctl start ucxsync
sudo systemctl status ucxsync
```

## Полезные команды

### Мониторинг

```bash
# Логи в реальном времени
sudo journalctl -u ucxsync -f

# Статус сервиса
systemctl status ucxsync

# Проверка веб-API
curl http://localhost:8080/api/status | jq

# Системные ресурсы
htop
```

### Управление сервисом

```bash
# Запуск
sudo systemctl start ucxsync

# Остановка
sudo systemctl stop ucxsync

# Перезапуск
sudo systemctl restart ucxsync

# Отключить автозапуск
sudo systemctl disable ucxsync

# Включить автозапуск
sudo systemctl enable ucxsync
```

### Отладка

```bash
# Проверка конфига
cat /etc/ucxsync/config.yaml

# Проверка доступности UCX узла
ping 192.168.1.101
smbclient -L //192.168.1.101 -U admin

# Ручное монтирование для теста
sudo mount -t cifs //192.168.1.101/share1 /mnt/test \
  -o username=admin,password=yourpass

# Размонтировать
sudo umount /mnt/test

# Проверка дискового пространства
df -h /mnt/storage

# Проверка портов
sudo ss -tulpn | grep 8080
```

### Быстрая переустановка

```bash
# Остановить и удалить
sudo systemctl stop ucxsync
sudo systemctl disable ucxsync
sudo rm /usr/local/bin/ucxsync
sudo rm -rf /etc/ucxsync
sudo rm /etc/systemd/system/ucxsync.service
sudo systemctl daemon-reload

# Заново собрать и установить
cd UCXSync
git pull
make build
sudo cp ucxsync /usr/local/bin/
sudo cp ucxsync.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl start ucxsync
```

## Типичные проблемы

### 🔴 Порт 8080 занят
```bash
# Узнать кто занял
sudo ss -tulpn | grep 8080

# Изменить порт в конфиге
sudo nano /etc/ucxsync/config.yaml
# web.port: 8081
```

### 🔴 Ошибка монтирования
```bash
# Установить cifs-utils
sudo apt install cifs-utils

# Проверить доступность
ping 192.168.1.101
smbclient -L //192.168.1.101 -U admin
```

### 🔴 Go не найден
```bash
# Установить Go вручную
wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

### 🔴 Нет прав на директорию
```bash
sudo chown -R $USER:$USER /mnt/storage/ucx
sudo chmod -R 755 /mnt/storage/ucx
```

### 🔴 Out of Memory
```yaml
# В config.yaml уменьшить параллелизм
sync:
  max_parallelism: 3  # было 8
```

## Быстрый полный тест

```bash
# 1. Клонируем
git clone https://github.com/zangezia/UCXSync.git && cd UCXSync

# 2. Устанавливаем
chmod +x install.sh && sudo ./install.sh

# 3. Настраиваем
sudo cp config.example.yaml /etc/ucxsync/config.yaml
sudo nano /etc/ucxsync/config.yaml

# 4. Директории
sudo mkdir -p /mnt/storage/ucx /mnt/ucx
sudo chown $USER:$USER /mnt/storage/ucx

# 5. Тест
ucxsync serve --config /etc/ucxsync/config.yaml

# В другом терминале:
curl http://localhost:8080/api/status

# 6. Сервис
sudo systemctl enable ucxsync
sudo systemctl start ucxsync
sudo journalctl -u ucxsync -f
```

## Узнать версию всего

```bash
# ОС
lsb_release -a

# Kernel
uname -r

# Архитектура
uname -m

# Go
go version

# UCXSync
ucxsync version

# Git commit
cd UCXSync && git log -1 --oneline
```

## Ссылки на документацию

- **TEST.md** - подробная инструкция по тестированию
- **LINUX.md** - полное руководство для AMD64 Linux
- **QUICKSTART.md** - быстрый старт
- **README.md** - общая информация

---

**⚡ Быстрый старт:**
```bash
git clone https://github.com/zangezia/UCXSync.git && \
cd UCXSync && \
chmod +x QUICK-TEST.sh && \
./QUICK-TEST.sh
```

Потом открыть: `http://localhost:8080` 🚀
