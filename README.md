# Финальный проект 1 семестра

REST API сервис для загрузки и выгрузки данных о ценах.

## Требования к системе

Ubuntu 22.04 или выше

Go 1.23.3 или выше

## Установка и запуск

1. Установка go:
    ```
    https://go.dev/doc/install
    ```

2. Установка PostgreSQL:
    ```bash
    sudo apt update
    sudo apt install -y postgresql
    ```

3. Настройка базы данных:
    ```bash
    psql -U postgres
    CREATE USER validator WITH PASSWORD 'val1dat0r';
    CREATE DATABASE "project-sem-1";
    \c "project-sem-1"
    CREATE TABLE IF NOT EXISTS prices (
        id SERIAL PRIMARY KEY,
        product_id INT NOT NULL,
        created_at DATE NOT NULL,
        name TEXT NOT NULL,
        category TEXT NOT NULL,
        price NUMERIC(10, 2) NOT NULL
    );
    GRANT ALL PRIVILEGES ON DATABASE "project-sem-1" TO validator;
    ```

4. Установка зависимостей
```
./scripts/prepare.sh
```

5. Запуск сервера
```
./scripts/run.sh
```

6. Запуск тестов
```
./scripts/tests.sh
```

## Тестирование

Директория `sample_data` - это пример директории, которая является разархивированной версией файла `sample_data.zip`

Отправка POST-запроса на запись в базу данных
```
curl -X POST -F "file=@sample_data.zip" http://localhost:8080/api/v0/prices
```
Отправка GET-запроса на скачивание записей из базы данных
```
curl -X GET -o response.zip http://localhost:8080/api/v0/prices

## Контакт

Никита Кудухов
@kykitos
