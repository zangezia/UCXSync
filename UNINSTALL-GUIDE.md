# 🗑️ Быстрое удаление UCXSync

## Автоматическое удаление

```bash
cd UCXSync
chmod +x uninstall.sh
sudo ./uninstall.sh
```

Скрипт спросит что удалять (конфигурацию, данные, зависимости).

---

## Ручное быстрое удаление

### Полное удаление (всё!)

```bash
# Остановить сервис
sudo systemctl stop ucxsync 2>/dev/null
sudo systemctl disable ucxsync 2>/dev/null

# Удалить файлы
sudo rm -f /usr/local/bin/ucxsync
sudo rm -rf /etc/ucxsync
sudo rm -f /etc/systemd/system/ucxsync.service
sudo rm -rf /mnt/storage/ucx
sudo rm -rf /mnt/ucx

# Обновить systemd
sudo systemctl daemon-reload

echo "✓ UCXSync полностью удалён"
```

### Частичное удаление (сохранить конфиг и данные)

```bash
# Остановить сервис
sudo systemctl stop ucxsync 2>/dev/null
sudo systemctl disable ucxsync 2>/dev/null

# Удалить только программу
sudo rm -f /usr/local/bin/ucxsync
sudo rm -f /etc/systemd/system/ucxsync.service

# Обновить systemd
sudo systemctl daemon-reload

# Конфиг остаётся в /etc/ucxsync
# Данные остаются в /mnt/storage/ucx

echo "✓ UCXSync удалён (конфиг и данные сохранены)"
```

### Только сервис (для переустановки)

```bash
# Остановить сервис
sudo systemctl stop ucxsync
sudo systemctl disable ucxsync

echo "✓ Сервис остановлен, файлы сохранены"
```

---

## Удаление только старой сборки перед обновлением

```bash
# Остановить сервис (но не отключать autostart)
sudo systemctl stop ucxsync

# Удалить старый бинарник
sudo rm -f /usr/local/bin/ucxsync

# Конфиг и данные остаются нетронутыми
# Теперь можно делать новую сборку

echo "✓ Готово к обновлению"
```

После этого можно установить новую версию:
```bash
cd UCXSync
git pull
make build
sudo cp ucxsync /usr/local/bin/
sudo systemctl start ucxsync
```

---

## Проверка что осталось

```bash
# Проверить бинарник
ls -l /usr/local/bin/ucxsync

# Проверить конфигурацию
ls -l /etc/ucxsync/

# Проверить данные
du -sh /mnt/storage/ucx

# Проверить сервис
systemctl status ucxsync

# Проверить процесс
ps aux | grep ucxsync
```

---

## Очистка всего (полный reset)

```bash
# Остановить всё
sudo systemctl stop ucxsync 2>/dev/null
sudo systemctl disable ucxsync 2>/dev/null

# Размонтировать сетевые диски
sudo umount /mnt/ucx/* 2>/dev/null || true

# Удалить всё
sudo rm -f /usr/local/bin/ucxsync
sudo rm -rf /etc/ucxsync
sudo rm -f /etc/systemd/system/ucxsync.service
sudo rm -rf /mnt/storage/ucx
sudo rm -rf /mnt/ucx
sudo rm -rf /opt/ucxsync
sudo rm -rf /var/log/ucxsync

# Удалить исходники (если не нужны)
cd ~ && rm -rf UCXSync

# Обновить systemd
sudo systemctl daemon-reload

echo "✓ Полная очистка завершена"
```

---

## Быстрая переустановка

```bash
# 1. Удалить старую версию
sudo systemctl stop ucxsync
sudo rm -f /usr/local/bin/ucxsync

# 2. Обновить код
cd UCXSync
git pull

# 3. Пересобрать
make build

# 4. Установить
sudo cp ucxsync /usr/local/bin/
sudo systemctl start ucxsync

# 5. Проверить
systemctl status ucxsync
```

---

## Одна команда - всё удалить

```bash
sudo systemctl stop ucxsync 2>/dev/null; sudo systemctl disable ucxsync 2>/dev/null; sudo rm -f /usr/local/bin/ucxsync; sudo rm -rf /etc/ucxsync; sudo rm -f /etc/systemd/system/ucxsync.service; sudo rm -rf /mnt/storage/ucx; sudo rm -rf /mnt/ucx; sudo systemctl daemon-reload; echo "✓ UCXSync удалён"
```

---

## Переустановка из нуля

```bash
# Полная очистка
sudo systemctl stop ucxsync 2>/dev/null; sudo systemctl disable ucxsync 2>/dev/null; sudo rm -f /usr/local/bin/ucxsync; sudo rm -rf /etc/ucxsync; sudo rm -f /etc/systemd/system/ucxsync.service; sudo systemctl daemon-reload

# Установка заново
cd ~ && rm -rf UCXSync
git clone https://github.com/zangezia/UCXSync.git
cd UCXSync
chmod +x QUICK-TEST.sh
./QUICK-TEST.sh
```

---

**Важно:** Скрипт `QUICK-TEST.sh` теперь автоматически удаляет предыдущую установку перед новой!
