Для запуска сервиса необходимо:

1. Получить лицензию на minio https://www.min.io/pricing, полученный файл лицензии сохранить в docker/minio/minio.license
2. Установить Docker
3. Собрать образ приложения
```bash
docker build -t avatargo:latest .
```
4. Запустить приложение
```bash
docker-compose up -d
```

Остановить приложение
```bash
docker-compose down
```
