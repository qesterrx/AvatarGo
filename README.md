Для запуска сервиса необходимо:

1. Получить лицензию на minio https://www.min.io/pricing, полученный файл лицензии сохранить в docker/minio/minio.license
2. Установить Docker
3. Собрать образ приложения
```bash
docker build -t avatargo:latest .
```
4. Запустить приложение
```bash
docker-compose -f ./docker/docker-compose.yml up -d
```

Остановить приложение
```bash
docker-compose -f ./docker/docker-compose.yml down
```

Запускаются следующие сервисы:

Приложение на порту 8080, адрес для загрузки фотографии http://localhost:8080/web/upload
RabbitMq: http://localhost:15672
Minio (S3 совместимое хранилище): http://localhost:3000
Loki: http://localhost:3100
Grafana: http://localhost:3200
Jaeger: http://localhost:3300
Prometeus: http://localhost:3400


Обмен данными с PosgreSQL на порту 5432
Обмен данными с RabbitMq на порту 5672
Обмен данными с Minio на порту 9000
Обмен данными с Opentlemety Collector на порту 4317 / 4318 / 8889
Обмен данными с Prometeus на порту 9000


Тестирование нагрузки:

```bash
go run .tests/load/main.go -count 10 -format webp
```



Прошу оценить промежуточный вариант, т.к. по заданию совершенно не понятно что конкретно нужно сделать
Про несоответствие материала заданию я вообще молчу