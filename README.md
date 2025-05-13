# transport-bus

Пакет для обмена сообщениями ставок фандинга и настроек через RabbitMQ с использованием protobuf (json-формат).

## Установка зависимостей для генерации кода

### Ubuntu
```sh
sudo apt update
sudo apt install -y protobuf-compiler
GO111MODULE=on go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
export PATH="$PATH:$(go env GOPATH)/bin"
```

### macOS
```sh
brew install protobuf
GO111MODULE=on go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
export PATH="$PATH:$(go env GOPATH)/bin"
```

## Генерация Go-кода из proto

```sh
protoc --go_out=internal --go_opt=paths=source_relative proto/funding.proto
```

## Сериализация/десериализация в JSON

Для передачи сообщений через RabbitMQ используйте:
```go
import "google.golang.org/protobuf/encoding/protojson"

// сериализация
jsonBytes, err := protojson.Marshal(msg)
// десериализация
err := protojson.Unmarshal(jsonBytes, msg)
```

## Использование

- Импортируйте пакет как go-модуль в свой проект.
- Для работы с protobuf в рантайме используйте зависимости, указанные в `go.mod`.
- Для генерации новых версий proto-файлов используйте команды выше. 