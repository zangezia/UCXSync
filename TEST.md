# Инструкция по тестированию UCXSync на Ubuntu Server

## Быстрая установка и тест

### 0. Удаление предыдущей установки (если была)

```bash
# Остановить и удалить старую версию
sudo systemctl stop ucxsync 2>/dev/null
sudo systemctl disable ucxsync 2>/dev/null
sudo rm -f /usr/local/bin/ucxsync
sudo rm -rf /etc/ucxsync
sudo rm -f /etc/systemd/system/ucxsync.service
sudo systemctl daemon-reload

# Опционально: очистить старые данные
# sudo rm -rf /ucdata/ucx/*
# sudo rm -rf /ucmount/*
```

### 1. Клонирование репозитория

```bash
git clone https://github.com/zangezia/UCXSync.git
cd UCXSync
```

### 2. Автоматическая установка

```bash
chmod +x install.sh
sudo ./install.sh
```

**Что делает установщик:**
- Определяет архитектуру (AMD64/ARM64/RISC-V)
- Устанавливает Go 1.21+ (если не установлен)
- Устанавливает cifs-utils для монтирования сетевых дисков
- Собирает приложение для вашей архитектуры
- Устанавливает бинарник в `/usr/local/bin/ucxsync`
- Создает systemd сервис

### 3. Настройка конфигурации

```bash
# Копируем пример конфигурации
sudo cp config.example.yaml /etc/ucxsync/config.yaml

# Редактируем под свои нужды
sudo nano /etc/ucxsync/config.yaml
```

**Обязательные настройки:**
```yaml
sync:
  max_parallelism: 8  # Для AMD64: 6-12, для RISC-V: 2-4
  
destinations:
  - path: "/ucdata/ucx"  # Путь должен существовать!
    min_free_gb: 500

nodes:
  ucx01:
    ip: "192.168.1.101"  # IP вашего UCX узла
    username: "ucxuser"
    password: "yourpassword"
    shares: ["share1", "share2"]
```

### 4. Создание директорий

```bash
# Создаем директорию для хранения данных
sudo mkdir -p /ucdata/ucx
sudo chown $USER:$USER /ucdata/ucx

# Создаем точки монтирования
sudo mkdir -p /ucmount
```

### 5. Тестовый запуск (БЕЗ systemd)

```bash
# Запуск в режиме тестирования
ucxsync serve --config /etc/ucxsync/config.yaml
```

**Ожидаемый вывод:**
```
INFO Starting UCXSync
INFO Web server listening on :8080
INFO Monitoring service started
INFO Sync service started
```

Откройте в браузере: `http://<IP_ноутбука>:8080`

### 6. Тест синхронизации

В веб-интерфейсе или через CLI:

```bash
# Запуск одной задачи синхронизации
ucxsync sync --node ucx01 --share share1 --config /etc/ucxsync/config.yaml
```

### 7. Запуск как сервис (после успешного теста)

```bash
# Включаем автозапуск
sudo systemctl enable ucxsync

# Запускаем сервис
sudo systemctl start ucxsync

# Проверяем статус
sudo systemctl status ucxsync

# Просмотр логов
sudo journalctl -u ucxsync -f
```

## Проверочный список

- [ ] Go установлен (версия >= 1.21)
- [ ] cifs-utils установлен
- [ ] Приложение скомпилировано без ошибок
- [ ] Конфигурация настроена
- [ ] Директория назначения создана
- [ ] Веб-интерфейс доступен на порту 8080
- [ ] Тестовая синхронизация работает
- [ ] Сервис стартует через systemd

## Мониторинг системы во время теста

### В отдельном терминале:

```bash
# Мониторинг ресурсов
htop

# Мониторинг дискового I/O
iostat -x 2

# Мониторинг сети
iftop -i eth0  # замените eth0 на ваш интерфейс

# Мониторинг использования диска
watch -n 1 df -h
```

## Ожидаемые показатели для AMD64

### Intel i5/i7, 8GB RAM, SSD:
- **CPU**: 30-60% при max_parallelism=8
- **RAM**: ~500MB-1GB
- **Network**: до 1Gb/s (зависит от сети)
- **Disk I/O**: 200-500 MB/s (зависит от диска)

### Старт задачи синхронизации:
```
[14:23:45] Mounting ucx01://share1 to /ucmount/ucx01_share1
[14:23:46] Connected successfully
[14:23:46] Scanning files...
[14:23:50] Found 15234 files (42.3 GB)
[14:23:50] Starting sync (8 parallel operations)
[14:24:15] Progress: 2156/15234 files (14%), 6.1 GB transferred
[14:24:30] Progress: 4512/15234 files (29%), 12.8 GB transferred
...
```

## Типичные проблемы и решения

### 1. Ошибка монтирования CIFS

```bash
# Проверьте доступность узла
ping 192.168.1.101

# Проверьте доступность SMB
smbclient -L //192.168.1.101 -U ucxuser

# Ручное монтирование для теста
sudo mount -t cifs //192.168.1.101/share1 /mnt/test -o username=ucxuser,password=yourpassword
```

### 2. Порт 8080 занят

Измените в `config.yaml`:
```yaml
web:
  port: 8081  # или другой свободный порт
```

### 3. Недостаточно прав

```bash
# Проверьте права на директорию
ls -la /ucdata/

# Исправьте владельца
sudo chown -R $USER:$USER /ucdata/ucx
```

### 4. Out of Memory при большом max_parallelism

Уменьшите в `config.yaml`:
```yaml
sync:
  max_parallelism: 4  # уменьшите вдвое
```

## Тест производительности

### Создание тестовых файлов (на UCX узле):

```bash
# 1000 файлов по 1MB
for i in {1..1000}; do 
    dd if=/dev/urandom of=/share/test/file$i.dat bs=1M count=1
done
```

### Запуск теста с разными настройками:

```bash
# Test 1: max_parallelism=4
ucxsync sync --node ucx01 --share test --config config1.yaml

# Test 2: max_parallelism=8
ucxsync sync --node ucx01 --share test --config config2.yaml

# Test 3: max_parallelism=12
ucxsync sync --node ucx01 --share test --config config3.yaml
```

**Замеряйте:**
- Время выполнения
- Использование CPU
- Использование RAM
- Скорость сети
- Disk I/O

## Финальная проверка

```bash
# Версия приложения
ucxsync version

# Проверка конфигурации
ucxsync config --validate

# Список доступных узлов
ucxsync nodes --list

# Статус сервиса
systemctl status ucxsync

# Доступность веб-интерфейса
curl http://localhost:8080/api/status
```

## Удаление (если нужно)

```bash
sudo systemctl stop ucxsync
sudo systemctl disable ucxsync
sudo rm /usr/local/bin/ucxsync
sudo rm -rf /etc/ucxsync
sudo rm /etc/systemd/system/ucxsync.service
sudo systemctl daemon-reload
```

## Что тестировать

### Базовые функции:
1. ✅ Установка и сборка
2. ✅ Запуск веб-сервера
3. ✅ Подключение к UCX узлу
4. ✅ Монтирование сетевых дисков
5. ✅ Синхронизация файлов
6. ✅ Веб-интерфейс и мониторинг
7. ✅ WebSocket обновления в реальном времени

### Стресс-тесты:
1. ✅ Синхронизация большого количества файлов (>10000)
2. ✅ Синхронизация больших файлов (>1GB)
3. ✅ Параллельная синхронизация нескольких узлов
4. ✅ Длительная работа (>1 час)
5. ✅ Перезапуск сервиса под нагрузкой

### Граничные случаи:
1. ✅ Недоступный UCX узел
2. ✅ Недостаточно места на диске
3. ✅ Сетевые разрывы во время синхронизации
4. ✅ Неправильные учетные данные
5. ✅ Отсутствие прав на запись

## Репортинг проблем

При обнаружении проблем создайте issue на GitHub с:
- Версией Ubuntu
- Архитектурой (uname -m)
- Версией Go (go version)
- Конфигурационным файлом (без паролей!)
- Логами (journalctl -u ucxsync)
- Описанием проблемы

---

**Удачного тестирования!** 🚀
