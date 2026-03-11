# 🚀 Инструкция для тестирования UCXSync на ноутбуке с Ubuntu Server

## ✅ Всё запушено в GitHub!

**Репозиторий:** https://github.com/zangezia/UCXSync

**Последние коммиты:**
- `863287d` - docs: Add quick reference cheatsheet for Ubuntu testing
- `000035c` - docs: Add testing guide and quick test script for Ubuntu Server
- `a890137` - feat: Add multi-architecture support (AMD64, ARM64, RISC-V)

---

## 📋 План действий на ноутбуке

### ⚠️ Шаг 0: Удаление предыдущей установки (если была)

Если UCXSync уже был установлен ранее, сначала удалите старую версию:

```bash
# Остановить и отключить сервис
sudo systemctl stop ucxsync 2>/dev/null
sudo systemctl disable ucxsync 2>/dev/null

# Удалить файлы
sudo rm -f /usr/local/bin/ucxsync
sudo rm -rf /etc/ucxsync
sudo rm -f /etc/systemd/system/ucxsync.service

# Обновить systemd
sudo systemctl daemon-reload

# Опционально: удалить данные (если нужно начать с чистого листа)
# sudo rm -rf /ucdata/ucx/*
# sudo rm -rf /ucmount/*
```

Теперь можно устанавливать новую версию:

---

### Вариант 1: Автоматическая установка (рекомендуется)

```bash
# На ноутбуке с Ubuntu Server выполните:

# 1. Клонируем репозиторий
git clone https://github.com/zangezia/UCXSync.git
cd UCXSync

# 2. Запускаем скрипт автоматической установки
chmod +x QUICK-TEST.sh
./QUICK-TEST.sh
```

**Скрипт автоматически:**
- ✅ Определит архитектуру (x86_64)
- ✅ Установит Go 1.21+ (если нужно)
- ✅ Соберёт приложение
- ✅ Установит в `/usr/local/bin/ucxsync`
- ✅ Создаст конфиг в `/etc/ucxsync/config.yaml`
- ✅ Создаст директории `/ucdata/ucx` и `/ucmount`
- ✅ Установит systemd сервис

### Вариант 2: Ручная установка (если хочешь контроль)

```bash
# 1. Клонируем
git clone https://github.com/zangezia/UCXSync.git
cd UCXSync

# 2. Устанавливаем через install.sh
chmod +x install.sh
sudo ./install.sh

# 3. Настраиваем конфигурацию
sudo mkdir -p /etc/ucxsync
sudo cp config.example.yaml /etc/ucxsync/config.yaml
sudo nano /etc/ucxsync/config.yaml  # ← РЕДАКТИРУЕМ!

# 4. Создаём директории
sudo mkdir -p /ucdata/ucx
sudo mkdir -p /ucmount
sudo chown $USER:$USER /ucdata/ucx
```

---

## ⚙️ Что нужно изменить в config.yaml

После установки **ОБЯЗАТЕЛЬНО** отредактируйте конфиг:

```bash
sudo nano /etc/ucxsync/config.yaml
```

**Измените следующие параметры:**

```yaml
# 1. Параллелизм (для AMD64 ноутбука)
sync:
  project: "Arh2k_mezen_200725"    # Название проекта
  destination: "/ucdata/ucx"   # USB-SSD диск!
  max_parallelism: 6                # 4-8 для ноутбука
  min_free_disk_space: 52428800     # 50 MB

# 2. Список UCX узлов
nodes:
  - WU01
  - WU02
  - WU03
  # ... добавьте все 14 узлов
  - WU13
  - CU

# 3. Сетевые шары на узлах
shares:
  - E$
  - F$

# 4. Учётные данные для доступа к UCX узлам
credentials:
  username: "Administrator"  # ← ИЗМЕНИТЕ!
  password: "ultracam"       # ← ИЗМЕНИТЕ!
```

**Важно:** 
- `/ucdata/ucx` - это путь на USB-SSD диске!
- Узлы WU01-WU13, CU должны быть доступны по сети
- См. `STORAGE-ARCHITECTURE.md` для деталей

---

## 🧪 Первый тестовый запуск

### 1. Запуск БЕЗ systemd (для тестирования)

```bash
ucxsync serve --config /etc/ucxsync/config.yaml
```

**Должен увидеть:**
```
INFO Starting UCXSync
INFO Web server listening on :8080
INFO Monitoring service started
INFO Sync service started
```

Если всё OK, жми `Ctrl+C` чтобы остановить.

### 2. Открой веб-интерфейс

**На самом ноутбуке:**
```
http://localhost:8080
```

**С другого компьютера в той же сети:**
```
http://<IP-ноутбука>:8080
```

Узнать IP ноутбука:
```bash
hostname -I | awk '{print $1}'
# или
ip a | grep "inet " | grep -v 127.0.0.1
```

### 3. Тест синхронизации одного узла

В веб-интерфейсе:
1. Выбери узел (например, ucx01)
2. Нажми "Start Sync"
3. Смотри процесс в реальном времени

Или через CLI:
```bash
ucxsync sync --node ucx01 --share share1 --config /etc/ucxsync/config.yaml
```

---

## 🎯 Запуск как постоянного сервиса

После успешного теста, запусти как сервис:

```bash
# Включить автозапуск при загрузке
sudo systemctl enable ucxsync

# Запустить сервис
sudo systemctl start ucxsync

# Проверить статус
sudo systemctl status ucxsync

# Смотреть логи в реальном времени
sudo journalctl -u ucxsync -f
```

---

## 📊 Мониторинг во время теста

Открой несколько терминалов и запусти:

### Терминал 1: Логи UCXSync
```bash
sudo journalctl -u ucxsync -f
```

### Терминал 2: Системные ресурсы
```bash
htop
```

### Терминал 3: Дисковый I/O
```bash
sudo apt install sysstat  # если не установлен
iostat -x 2
```

### Терминал 4: Сетевая активность
```bash
sudo apt install iftop  # если не установлен
sudo iftop -i eth0  # замени eth0 на свой интерфейс (ip a)
```

### Терминал 5: Использование диска
```bash
watch -n 1 'df -h | grep "/ucdata"'
```

---

## 🔍 Что проверять при тесте

### ✅ Базовая функциональность:
- [ ] Приложение компилируется без ошибок
- [ ] Веб-сервер запускается на порту 8080
- [ ] Веб-интерфейс доступен из браузера
- [ ] WebSocket соединение работает (обновления в реальном времени)
- [ ] Мониторинг показывает CPU, RAM, Disk, Network
- [ ] Подключение к UCX узлу успешно
- [ ] Монтирование сетевого диска работает
- [ ] Синхронизация файлов работает
- [ ] Прогресс отображается корректно
- [ ] После завершения файлы на месте

### ✅ Производительность (для AMD64 ноутбука):
- **CPU**: 30-60% при max_parallelism=6
- **RAM**: ~500MB-1GB
- **Network**: Зависит от сети (100Mbps - 1Gbps)
- **Disk I/O**: 100-300 MB/s (зависит от диска)
- **Скорость копирования**: ~50-100 MB/s (зависит от сети)

### ✅ Стресс-тесты:
- [ ] Синхронизация большого количества файлов (>5000)
- [ ] Синхронизация больших файлов (>1GB)
- [ ] Параллельная синхронизация 2-3 узлов одновременно
- [ ] Длительная работа (>30 минут)
- [ ] Перезапуск сервиса во время синхронизации

### ✅ Граничные случаи:
- [ ] Недоступный UCX узел (отключи сеть)
- [ ] Неправильный пароль
- [ ] Недостаточно места на диске
- [ ] Порт 8080 уже занят
- [ ] Нет прав на запись в /ucdata/ucx

---

## 🐛 Типичные проблемы и решения

### Проблема: "connection refused" к UCX узлу

**Решение:**
```bash
# Проверь доступность узла
ping 192.168.1.101

# Проверь SMB
smbclient -L //192.168.1.101 -U admin

# Попробуй смонтировать вручную
sudo mount -t cifs //192.168.1.101/share1 /mnt/test \
  -o username=admin,password=yourpass
```

### Проблема: "port 8080 already in use"

**Решение:**
```bash
# Узнай кто занял порт
sudo ss -tulpn | grep 8080

# Измени порт в конфиге
sudo nano /etc/ucxsync/config.yaml
# Измени: web.port: 8081
```

### Проблема: "permission denied" на /ucdata/ucx

**Решение:**
```bash
sudo chown -R $USER:$USER /ucdata/ucx
sudo chmod -R 755 /ucdata/ucx
```

### Проблема: "out of memory"

**Решение:**
```yaml
# В /etc/ucxsync/config.yaml уменьши параллелизм
sync:
  max_parallelism: 3  # было 6
```

### Проблема: cifs-utils не установлен

**Решение:**
```bash
sudo apt update
sudo apt install cifs-utils
```

---

## 📝 После успешного теста

### 1. Проверь логи
```bash
sudo journalctl -u ucxsync --since "1 hour ago"
```

### 2. Проверь статистику
Открой веб-интерфейс и посмотри:
- Количество синхронизированных файлов
- Объём скопированных данных
- Использование системных ресурсов
- Время выполнения задач

### 3. Проверь файлы на диске
```bash
# Список синхронизированных файлов
ls -lah /ucdata/ucx/ucx01/share1/

# Размер
du -sh /ucdata/ucx/*
```

### 4. Если всё OK - сообщи результат!

**Что сообщить:**
- ✅ Версия Ubuntu: `lsb_release -a`
- ✅ Архитектура: `uname -m`
- ✅ Количество синхронизированных файлов
- ✅ Общий объём данных
- ✅ Время выполнения
- ✅ Использование CPU/RAM
- ✅ Скриншот веб-интерфейса (опционально)
- ✅ Любые проблемы или баги

---

## 🆘 Если что-то пошло не так

### Соберём отладочную информацию:

```bash
# Создай файл с диагностикой
cat > debug-info.txt << 'EOF'
=== System Info ===
$(lsb_release -a)
$(uname -a)

=== UCXSync Version ===
$(ucxsync version)

=== Go Version ===
$(go version)

=== Disk Space ===
$(df -h)

=== Network Interfaces ===
$(ip a)

=== UCXSync Config ===
$(cat /etc/ucxsync/config.yaml | grep -v password)

=== UCXSync Logs (last 100 lines) ===
$(sudo journalctl -u ucxsync -n 100 --no-pager)

=== System Resources ===
$(free -h)
$(lscpu | grep -E "Model name|CPU\(s\)")
EOF

# Отправь мне этот файл
cat debug-info.txt
```

---

## 🎓 Полезные команды для повседневного использования

```bash
# Просмотр статуса
systemctl status ucxsync

# Перезапуск
sudo systemctl restart ucxsync

# Логи
sudo journalctl -u ucxsync -f

# Проверка веб-API
curl http://localhost:8080/api/status | jq

# Ручная синхронизация конкретного узла
ucxsync sync --node ucx01 --share share1

# Список всех доступных узлов
ucxsync nodes --list

# Проверка конфигурации
ucxsync config --validate
```

---

## 📚 Документация

В репозитории есть подробная документация:

- **CHEATSHEET.md** - эта шпаргалка
- **TEST.md** - подробное руководство по тестированию
- **LINUX.md** - полное руководство для Linux AMD64
- **QUICKSTART.md** - быстрый старт
- **README.md** - общая информация о проекте
- **ARCHITECTURE.md** - архитектура приложения

---

## ✨ Быстрый старт одной командой

```bash
git clone https://github.com/zangezia/UCXSync.git && \
cd UCXSync && \
chmod +x QUICK-TEST.sh && \
./QUICK-TEST.sh
```

После этого:
1. Отредактируй `/etc/ucxsync/config.yaml` (IP, логины, пароли)
2. Запусти `ucxsync serve --config /etc/ucxsync/config.yaml`
3. Открой `http://localhost:8080`

---

**Удачи с тестированием! 🚀**

При любых вопросах или проблемах - пиши!
