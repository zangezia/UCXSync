# UCXSync — краткое руководство на русском

UCXSync — Linux-сервис для синхронизации файлов с UCX-узлов по CIFS/SMB на локальное хранилище с веб-интерфейсом мониторинга.

## Что умеет

- подключать CIFS-шары UCX-узлов;
- искать новые и изменённые файлы;
- копировать данные параллельно в локальное хранилище;
- показывать статус, логи и метрики в браузере.

## Поддерживаемая среда

- **ОС:** Linux
- **Архитектуры:** AMD64, ARM64, RISC-V 64
- **Права:** root / `sudo` для операций монтирования
- **Зависимости:** `cifs-utils`, `mount`, `umount`, `lsblk`

## Быстрый запуск

```bash
git clone https://github.com/zangezia/UCXSync.git
cd UCXSync
sudo ./install.sh
sudo nano /etc/ucxsync/config.yaml
sudo /opt/ucxsync/ucxsync check --config /etc/ucxsync/config.yaml
sudo systemctl enable --now ucxsync
```

После запуска откройте:

```text
http://localhost:8080
```

## Где лежат основные файлы

- бинарник: `/opt/ucxsync/ucxsync`
- web assets: `/opt/ucxsync/web`
- конфигурация: `/etc/ucxsync/config.yaml`
- каталог монтирования шар: `/ucmount`
- локальное хранилище: `/ucdata`

## Часто используемые команды

```bash
sudo /opt/ucxsync/ucxsync check --config /etc/ucxsync/config.yaml
sudo /opt/ucxsync/ucxsync mount --config /etc/ucxsync/config.yaml
sudo /opt/ucxsync/ucxsync unmount --config /etc/ucxsync/config.yaml
sudo systemctl restart ucxsync
sudo journalctl -u ucxsync -f
```

## Карта документации

- [`QUICKSTART.md`](QUICKSTART.md) — самый короткий путь до первого запуска
- [`INSTALL.md`](INSTALL.md) — полная установка и варианты single / dual deployment
- [`ORANGEPI.md`](ORANGEPI.md) — особенности Orange Pi RV2 и RISC-V
- [`BUILD.md`](BUILD.md) — сборка для поддерживаемых архитектур
- [`CHEATSHEET.md`](CHEATSHEET.md) — краткая шпаргалка по командам
- [`ARCHITECTURE.md`](ARCHITECTURE.md) — устройство системы
- [`PARALLELISM.md`](PARALLELISM.md) — настройки параллелизма
- [`STORAGE-ARCHITECTURE.md`](STORAGE-ARCHITECTURE.md) — схема `/ucmount` и `/ucdata`
- [`USB-SSD-GUIDE.md`](USB-SSD-GUIDE.md) — подготовка внешнего диска
- [`UNINSTALL-GUIDE.md`](UNINSTALL-GUIDE.md) — корректное удаление

## Замечания

- Полная функциональность зависит от Linux-специфичных механизмов монтирования и доступа к `/proc`.
- Для установленной копии используйте `/opt/ucxsync/ucxsync`, а не просто `ucxsync`, если вы не добавляли бинарник в `PATH` вручную.
- Каталог `cpp/` остаётся экспериментальным и не участвует в продакшн-рантайме.
