# Руководство по Go (Часть 1): От синтаксиса до архитектуры HTTP-сервиса

Данный справочник охватывает фундаментальные концепции языка Go, стандартную библиотеку (`fmt`, `strconv`, `net/http`, `encoding/json`), семантику типов и памяти, обработку ошибок, роутинг (включая нововведения Go 1.22+), архитектурные паттерны инкапсуляции состояния и middleware.

---

## Оглавление

1. [Окружение, структура проекта и запуск](#1-окружение-структура-проекта-и-запуск)
2. [Переменные, константы и базовая система типов](#2-переменные-константы-и-базовая-система-типов)
3. [Функции, видимость идентификаторов и множественный возврат](#3-функции-видимость-идентификаторов-и-множественный-возврат)
4. [Структуры (struct) и методы](#4-структуры-struct-и-методы)
5. [Обработка ошибок и валидация](#5-обработка-ошибок-и-валидация)
6. [Создание HTTP-сервера (net/http)](#6-создание-http-сервера-nethttp)
7. [Сериализация в JSON (encoding/json) и Struct Tags](#7-сериализация-в-json-encodingjson-и-struct-tags)
8. [Слайсы (Slices) и итерация for range](#8-слайсы-slices-и-итерация-for-range)
9. [Прием данных: HTTP-методы и десериализация JSON](#9-прием-данных-http-методы-и-десериализация-json)
10. [Ассоциативные массивы (map) и маршрутизация Go 1.22+](#10-ассоциативные-массивы-map-и-маршрутизация-go-122)
11. [Архитектура: инкапсуляция состояния в Server и Pointer Receivers](#11-архитектура-инкапсуляция-состояния-в-server-и-pointer-receivers)
12. [Middleware (Промежуточное ПО)](#12-middleware-промежуточное-по)
13. [Полный сквозной пример микросервиса](#13-полный-сквозной-пример-микросервиса)

---

## 1. Окружение, структура проекта и запуск

### 1.1. Проверка установки и версионирование

Команда проверки версии компилятора Go:

```bash
go version
# Вывод: go version go1.22.x linux/amd64
```

### 1.2. Модульная система (`go.mod`)

Начиная с Go 1.11 (и безусловно с Go 1.16+), управление зависимостями строится на базе **Go Modules**. Проекты больше не привязаны к устаревшему `GOPATH/src`.

Инициализация модуля:

```bash
# Синтаксис: go mod init <имя_модуля>
go mod init example.com/myproject
# Либо локальное имя:
go mod init pilot
```

Файл `go.mod` определяет корень модуля, минимальную версию языка и дерево прямых/транзитивных внешних зависимостей:

```go
module pilot

go 1.22
```

### 1.3. Точка входа: `package main` и `func main()`

Каждая исполняемая программа Go обязана содержать:

1. Декларацию пакета `package main` на первой строке исполняемого файла.
2. Функцию `func main()` без параметров и без возвращаемых значений.

```go
package main

import "fmt"

func main() {
    fmt.Println("Application started")
}
```

> **Пакеты библиотек vs приложения:** Если пакет называется иначе, чем `main` (например, `package store`), компилятор Go трактует его как переиспользуемую библиотеку, и он не может быть скомпилирован в самостоятельный бинарный файл.

### 1.4. Сборка и выполнение: `go run` vs `go build`

| Команда                         | Что делает                                                                                                          | Когда применять                                          |
| ------------------------------- | ------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------- |
| `go run .` или `go run main.go` | Компилирует исходники во временную директорию ОС (`/tmp/...`), запускает процесс, а по завершении удаляет бинарник. | Разработка, локальные тесты, скрипты.                    |
| `go build -o app .`             | Создает статически скомпилированный исполняемый бинарный файл в текущей директории.                                 | Production, CI/CD, упаковка в Docker scratch-контейнеры. |

Бинарный файл Go по умолчанию самодостаточен: он включает в себя runtime (шедулер горутин, сборщик мусора GC, сетевой поллер) и слинкован статически (если не задействован cgo).

---

## 2. Переменные, константы и базовая система типов

Go — статически типизированный язык со строгой компиляторной дисциплиной. Неявное приведение типов (implicit type conversion/coercion) отсутствует: попытка сложить `int` и `float64` или присвоить `int32` переменной `int64` завершится ошибкой компиляции.

### 2.1. Объявление переменных: `var` против `:=`

```go
// 1. Полная форма с ключевым словом var (доступна на уровне пакета и внутри функций)
var appName string = "Pilot Service"
var retryCount int = 3

// 2. Декларация без инициализации (получает Zero Value)
var isReady bool // Значение по умолчанию: false

// 3. Блочное объявление var (...)
var (
    serverPort = 8080 // тип int выведен автоматически
    timeoutSec = 30.5 // тип float64
)

func run() {
    // 4. Краткое объявление через оператор := (ТОЛЬКО внутри функций)
    host := "127.0.0.1" // string выведен компилятором
    port := 9000        // int

    // Обычное присваивание существующей переменной
    port = 9001
}
```

### 2.2. Особенность оператора `:=` при переопределении

Оператор `:=` создает новые переменные. Однако он разрешает переиспользование уже существующей переменной в кортеже множественного присваивания при соблюдении правила: **хотя бы одна переменная слева от `:=` должна быть абсолютно новой**.

```go
func process() error {
    data, err := readConfig() // data и err объявлены впервые
    if err != nil {
        return err
    }

    // err уже объявлен, но file — новая переменная.
    // Это валидно: file создается, err переприсваивается!
    file, err := openFile("log.txt")
    if err != nil {
        return err
    }
    _ = file
    _ = data
    return nil
}
```

### 2.3. Скалярные типы данных

- Целые числа: `int` (размер зависит от архитектуры: 32 или 64 бита), `int8`, `int16`, `int32` (синоним `rune`), `int64`. Беззнаковые: `uint`, `uint8` (синоним `byte`), `uint16`, `uint32`, `uint64`, `uintptr`.
- С плавающей точкой: `float32`, `float64` (по умолчанию при литерале `1.0`).
- Булевы: `bool` (`true`, `false`).
- Строки: `string` (неизменяемый срез байтов UTF-8).

### 2.4. Концепция Zero Values (Нулевые значения)

В Go отсутствует понятие «мусора в памяти» или неинициализированных переменных (`undefined`, как в JS/C++). Выделенная память всегда заполняется нулями:

| Тип                                         | Zero Value                                                         |
| ------------------------------------------- | ------------------------------------------------------------------ |
| Числовые (`int`, `float64`, и т.д.)         | `0`, `0.0`                                                         |
| `bool`                                      | `false`                                                            |
| `string`                                    | `""` (пустая строка, длина 0)                                      |
| Указатели, слайсы, мапы, каналы, интерфейсы | `nil`                                                              |
| Структуры                                   | Структура, у которой каждое поле инициализировано своим Zero Value |

### 2.5. Константы (`const`)

Константы вычисляются на этапе компиляции и неизменяемы:

```go
const DefaultTimeout = 30 // бестиповая числовая константа (untyped)
const MaxRetries int = 5   // строго типизированная

const (
    StatusOK      = 200
    StatusCreated = 201
    StatusNotFound = 404
)
```

---

## 3. Функции, видимость идентификаторов и множественный возврат

### 3.1. Сигнатура функции

```go
// func <Имя>(<параметры>) (<возвращаемые значения>)
func Add(a int, b int) int {
    return a + b
}

// Сокращенная запись однотипных параметров подряд:
func FormatMessage(prefix, message, suffix string) string {
    return prefix + ": " + message + " [" + suffix + "]"
}
```

### 3.2. Правило видимости (Экспортируемость)

В Go нет ключевых слов `public`, `private` или `protected`. Доступность определяется **регистром первой буквы идентификатора**:

- **Заглавная буква** (`PascalCase`, например `Notification`, `CreateServer`): **экспортируемый** идентификатор (виден снаружи пакета).
- **Строчная буква** (`camelCase`, например `dbPool`, `validatePayload`): **неэкспортируемый** идентификатор (виден строго внутри своего пакета).

### 3.3. Форматирование строк через `fmt.Sprintf`

Пакет `fmt` реализует форматирование вывода. Функция `fmt.Sprintf` возвращает строку:

```go
import "fmt"

id := 42
name := "Server1"
res := fmt.Sprintf("ID: %d, Name: %s, Debug: %+v", id, name, someStruct)
```

Частые глаголы форматирования:

- `%s` — строка.
- `%d` — десятичное целое.
- `%f` / `%.2f` — число с плавающей точкой.
- `%t` — булево значение (`true`/`false`).
- `%v` — значение по умолчанию.
- `%+v` — поля структуры с их именами.
- `%T` — тип переменной.

### 3.4. Множественный возврат значений и Blank Identifier (`_`)

Go штатно возвращает несколько значений из функции. Это фундамент идиомы `(результат, ошибка)`:

```go
func Divide(a, b float64) (float64, error) {
    if b == 0 {
        return 0, fmt.Errorf("division by zero")
    }
    return a / b, nil
}

// Использование:
res, err := Divide(10, 2)
if err != nil {
    // обработка ошибки
}

// Игнорирование ненужного значения через blank identifier:
val, _ := Divide(100, 5) // ошибку осознанно отбросили
```

### 3.5. Именованные возвращаемые значения (Named Returns)

Параметры возврата могут быть именованы. Они объявляются в теле функции как обычные переменные со своими Zero Values:

```go
func CalculateBounds(values []int) (min int, max int) {
    if len(values) == 0 {
        return 0, 0
    }
    min = values[0]
    max = values[0]
    for _, v := range values {
        if v < min { min = v }
        if v > max { max = v }
    }
    return min, max // явный возврат предпочтительнее "голого" return
}
```

> **Совет по стилю:** Избегайте «голых» (naked) `return` в функциях длиной более 5 строк, так как это снижает читаемость кода.

---

## 4. Структуры (struct) и методы

Структура — пользовательский составной тип данных, группирующий поля под одним именем.

### 4.1. Определение и создание экземпляров

```go
// Определение структуры
type Notification struct {
    ID      int       // Экспортируемое поле
    Message string    // Экспортируемое поле
    retries int       // Приватное поле (неэкспортируемое)
}

func main() {
    // 1. Инициализация с именованными полями (Best Practice)
    n1 := Notification{
        ID:      1,
        Message: "Deploy successful",
    } // retries получит 0

    // 2. Инициализация Zero Value
    var n2 Notification // ID: 0, Message: "", retries: 0

    // Доступ через точку:
    n1.retries = 3
    fmt.Println(n1.Message)
}
```

### 4.2. Методы и ресиверы (Receivers)

Метод — это функция, привязанная к определенному типу через специальный аргумент: **receiver (получатель)**. В Go нет неявного указателя `this` или `self`. Экземпляр передается абсолютно явно:

```go
// Value Receiver: n передается как КОПИЯ
func (n Notification) Format() string {
    return fmt.Sprintf("[%d] %s", n.ID, n.Message)
}

// Pointer Receiver: n передается по УКАЗАТЕЛЮ (мутирует оригинал)
func (n *Notification) IncrementRetry() {
    n.retries++
}
```

Правило выбора ресивера:

- **Value Receiver (`n Notification`)**: метод только читает данные структуры и не мутирует их; размер структуры мал.
- **Pointer Receiver (`n *Notification`)**: метод изменяет поля структуры; структура содержит мьютексы/состояние; структура тяжелая (чтобы избежать накладных расходов на копирование памяти).

---

## 5. Обработка ошибок и валидация

В Go ошибки — это обычные значения первого класса (first-class citizens), а не исключения (`try/catch`).

### 5.1. Интерфейс `error`

Встроенный интерфейс `error` минималистичен:

```go
type error interface {
    Error() string
}
```

Значение `nil` для интерфейса `error` свидетельствует об **отсутствии** ошибки.

### 5.2. Создание ошибок

```go
import (
    "errors"
    "fmt"
)

// Простая ошибка без форматирования:
var ErrNotFound = errors.New("entity not found")

// Ошибка с форматированием контекста:
func parsePort(p int) error {
    if p < 1 || p > 65535 {
        return fmt.Errorf("invalid port %d: must be in range 1-65535", p)
    }
    return nil
}
```

### 5.3. Синтаксис `if` и ранний возврат (Guard Clauses)

В Go круглые скобки вокруг условия в `if` не ставятся, а открывающая фигурная скобка `{` обязана быть на той же строке:

```go
// Канонический паттерн Guard Clause (Ранний выход):
if err != nil {
    return err // немедленно возвращаем ошибку, не вкладывая основной код в else
}
```

### 5.4. Паттерн валидации структуры

```go
type CreateUserInput struct {
    Email string
    Age   int
}

// Метод валидации экземпляра
func (u CreateUserInput) Validate() error {
    if u.Email == "" {
        return errors.New("email is required")
    }
    if u.Age < 18 {
        return fmt.Errorf("invalid age %d: user must be an adult", u.Age)
    }
    return nil
}
```

---

## 6. Создание HTTP-сервера (`net/http`)

Стандартная библиотека Go предоставляет промышленный, высокопроизводительный HTTP-сервер, где каждое входящее соединение обслуживается отдельной легковесной горутиной.

### 6.1. Сигнатура хендлера и аргументы

Любой стандартный обработчик запросов имеет сигнатуру:

```go
func(w http.ResponseWriter, r *http.Request)
```

Разбор аргументов:

1. `w http.ResponseWriter` — интерфейс для формирования ответа клиенту:
   - `Header() http.Header`: карта заголовков для добавления/изменения.
   - `WriteHeader(statusCode int)`: отправка HTTP статус-кода.
   - `Write([]byte) (int, error)`: запись байтов в тело ответа.
2. `r *http.Request` — структура входящего запроса:
   - `r.Method`: HTTP-метод (`"GET"`, `"POST"` и т.д.).
   - `r.URL`: разобранный URL и query-параметры.
   - `r.Header`: заголовки входящего запроса.
   - `r.Body`: поток чтения тела запроса (`io.ReadCloser`).

### 6.2. Вывод: `fmt.Println` vs `fmt.Fprintln`

- `fmt.Println(...)` пишет в стандартный вывод процесса (`os.Stdout`). Клиент в браузере или curl этого **не увидит**.
- `fmt.Fprintln(w, ...)` принимает `io.Writer` (коим является `http.ResponseWriter`) и передает данные по TCP-сокету клиенту:

```go
func helloHandler(w http.ResponseWriter, r *http.Request) {
    fmt.Println("Log on server stdout")        // увидит разработчик в терминале
    fmt.Fprintln(w, "Hello, client from Go!") // увидит HTTP-клиент
}
```

### 6.3. Запуск сервера и регистрация путей

```go
package main

import (
    "fmt"
    "log"
    "net/http"
)

func main() {
    // Регистрация хендлера в DefaultServeMux
    http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        fmt.Fprintln(w, "OK")
    })

    fmt.Println("Server is listening on :8080...")
    // ListenAndServe блокирует поток и слушает порт.
    // При возникновении ошибки (например, порт занят) возвращает non-nil error.
    err := http.ListenAndServe(":8080", nil)
    if err != nil {
        log.Fatalf("Server failed to start: %v", err)
    }
}
```

---

## 7. Сериализация в JSON (`encoding/json`) и Struct Tags

Для построения REST API текстовые ответы не подходят — требуется обмен в JSON-формате.

### 7.1. Функция `json.Marshal`

```go
// Сигнатура функции маршалинга
func Marshal(v any) ([]byte, error)
```

`json.Marshal` принимает любое значение, инспектирует его через рефлексию и возвращает срез байтов `[]byte` в кодировке UTF-8.

### 7.2. Struct Tags (Теги структур) и требование экспортируемости

> **Критическое правило:** Пакет `encoding/json` сериализует **только публичные (экспортируемые) поля** структуры, начинающиеся с заглавной буквы. Поля со строчной буквы будут проигнорированы!

Для контроля имен ключей в JSON используются теги структуры:

```go
type NotificationResponse struct {
    ID        int       `json:"id"`                  // В JSON ключ будет "id"
    Message   string    `json:"message"`             // Ключ "message"
    IsRead    bool      `json:"is_read"`             // snake_case
    CreatedAt string    `json:"created_at,omitempty"`// omitempty скрывает поле, если оно равно zero value
    SecretKey string    `json:"-"`                   // "-" полностью исключает поле из сериализации
    internal  string    // не сериализуется, так как не экспортируется
}
```

### 7.3. Строгий порядок отправки HTTP-ответа

При отправке ответа необходимо соблюдать строгую последовательность:

1. `w.Header().Set("Content-Type", "application/json")` — установка заголовков.
2. `w.WriteHeader(statusCode)` — установка статуса ответа (например `http.StatusOK` или `http.StatusBadRequest`).
3. `w.Write(bytes)` — запись байтов тела.

```go
func sendJSON(w http.ResponseWriter, status int, payload any) {
    bytes, err := json.Marshal(payload)
    if err != nil {
        w.WriteHeader(http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    w.Write(bytes)
}
```

> **Внимание:** Если вызвать `w.Write()` до вызова `w.WriteHeader()`, Go автоматически отправит статус `200 OK`. Последующий вызов `w.WriteHeader(400)` будет проигнорирован с предупреждением в логах: `http: superfluous response.WriteHeader call`.

### 7.4. Анонимные структуры для разовых ответов

Если ответ специфичен для одного хендлера (например, ошибка валидации), не обязательно засорять глобальную область новым типом:

```go
func errorHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusBadRequest)

    // Анонимная структура inline
    errResponse := struct {
        Error   string `json:"error"`
        Code    int    `json:"code"`
    }{
        Error: "Invalid request payload",
        Code:  40001,
    }

    bytes, _ := json.Marshal(errResponse)
    w.Write(bytes)
}
```

---

## 8. Слайсы (Slices) и итерация `for range`

Массив в Go имеет фиксированную длину на этапе компиляции (`[5]int`). Слайс (`[]int`) — это динамическая обертка (fat pointer) над базовым массивом.

### 8.1. Внутреннее устройство слайса

Под капотом срез представлен структурой из трех машинных слов:

1. `Data uintptr` — указатель на область памяти базового массива.
2. `Len int` — текущее количество элементов (`len(s)`).
3. `Cap int` — емкость (вместимость) базового массива от указателя `Data` (`cap(s)`).

### 8.2. Функция `append` и перевыделение памяти

```go
var list []string // len: 0, cap: 0, list == nil

// Функция append принимает слайс и элементы, возвращая НОВЫЙ заголовок слайса:
list = append(list, "Item 1")
list = append(list, "Item 2", "Item 3")
```

> **Почему результат `append` всегда переприсваивают?**
> Если емкости `cap` недостаточно, `append` выделяет новый массив большего размера в куче (обычно x2), копирует туда старые элементы и возвращает слайс с новым указателем `Data`. Если не присвоить результат обратно переменной (`list = append(list, ...)`), вы продолжите ссылаться на старый заголовок с прежним `len` и потеряете добавленные данные!

### 8.3. Итерация через `for range` и главная ловушка мутации

```go
type Item struct {
    ID    int
    Title string
}

items := []Item{
    {ID: 1, Title: "A"},
    {ID: 2, Title: "B"},
}

// ОШИБКА: item — это локальная КОПИЯ элемента!
for _, item := range items {
    item.Title = "Updated" // Исходный срез НЕ изменится!
}

// ПРАВИЛЬНО: мутация по индексу
for i := range items {
    items[i].Title = "Updated" // Модифицируется память в базовом массиве
}
```

### 8.4. Фильтрация и маршалинг слайсов в JSON-массив

При передаче слайса `[]Item` в `json.Marshal` формируется стандартный JSON-массив `[{...}, {...}]`:

```go
var activeItems []Item
for _, item := range items {
    if item.ID > 0 {
        activeItems = append(activeItems, item)
    }
}

// Если activeItems пуст, но инициализирован make([]Item, 0), сериализуется как []
// Если activeItems был nil (var activeItems []Item), сериализуется как null
bytes, _ := json.Marshal(activeItems)
```

---

## 9. Прием данных: HTTP-методы и десериализация JSON

### 9.1. Семантика HTTP-методов

- `GET`: Безопасное получение ресурса (без тела запроса).
- `POST`: Создание новой сущности.
- `PUT`: Полная замена существующей сущности.
- `PATCH`: Частичное обновление сущности.
- `DELETE`: Удаление сущности.

В стандартном пакете `net/http` определены типобезопасные константы:
`http.MethodGet`, `http.MethodPost`, `http.MethodPut`, `http.MethodDelete` и др.

### 9.2. Потоковая десериализация через `json.NewDecoder`

В отличие от `json.Unmarshal(data, &v)`, который требует вычитки всего тела в память `[]byte`, `json.NewDecoder` читает данные непосредственно из потока ввода `io.Reader` (`r.Body`):

```go
// Сигнатура конструктора декодера:
func NewDecoder(r io.Reader) *Decoder

// Метод Decode:
func (dec *Decoder) Decode(v any) error
```

### 9.3. Роль оператора взятия адреса `&`

```go
var payload CreateNotificationInput

// ОШИБКА: передача по значению (Decode не сможет записать данные в payload):
// err := json.NewDecoder(r.Body).Decode(payload)

// ПРАВИЛЬНО: передаем указатель &payload
err := json.NewDecoder(r.Body).Decode(&payload)
if err != nil {
    http.Error(w, "Malformed JSON", http.StatusBadRequest)
    return
}
```

Декодер должен изменить поля существующей переменной в памяти. Для этого ему необходим адрес этой ячейки памяти (`&`).

### 9.4. Паттерн обработки POST-запроса с валидацией

```go
type CreatePayload struct {
    Message string `json:"message"`
}

func handleCreate(w http.ResponseWriter, r *http.Request) {
    // 1. Десериализация
    var body CreatePayload
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusBadRequest) // 400
        fmt.Fprint(w, `{"error":"invalid json body"}`)
        return
    }

    // 2. Валидация бизнес-правил
    if body.Message == "" {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusBadRequest)
        fmt.Fprint(w, `{"error":"message cannot be empty"}`)
        return
    }

    // 3. Успешный ответ (201 Created)
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusCreated)
    fmt.Fprint(w, `{"status":"created"}`)
}
```

---

## 10. Ассоциативные массивы (`map`) и маршрутизация Go 1.22+

### 10.1. Структура `map[K]V`

Поиск в слайсе требует линейного перебора $O(N)$. Хэш-таблица (`map`) обеспечивает среднее время выборки, вставки и удаления за константное время $O(1)$. Ключом `K` может выступать любой сравнимый оператором `==` тип (числа, строки, булевы значения, но не слайсы и не функции).

```go
// 1. Инициализация через make:
store := make(map[int]string)

// 2. Литеральная инициализация:
roles := map[string]int{
    "admin": 1,
    "user":  2,
}

// 3. ОПАСНОСТЬ: nil-мапа
var badMap map[int]string
// badMap[1] = "val" // ПАНИКА! Запись в nil map приводит к runtime panic!
```

### 10.2. Операции с мапой и паттерн Comma-ok

```go
m := make(map[string]int)

// Вставка / Обновление
m["requests"] = 42

// Удаление по ключу
delete(m, "requests")

// Чтение: ловушка Zero Value
val := m["unknown"] // вернет 0, но было ли там 0 или ключа нет?

// Паттерн безопасного чтения comma-ok:
val, exists := m["unknown"]
if !exists {
    // ключ реально отсутствует в мапе
}
```

> **Важно:** Порядок итерации `for k, v := range m` в Go намеренно рандомизируется runtime-системой при каждом запуске программы. Полагаться на порядок элементов в мапе категорически запрещено.

### 10.3. Революция маршрутизации в Go 1.22

До Go 1.22 стандартный `http.ServeMux` поддерживал только префиксные совпадения без методов и без извлечения URL-параметров (приходилось использовать внешние роутеры вроде `chi` или `gorilla/mux`).

Начиная с **Go 1.22**, стандартный роутер получил полноценную поддержку методов и шаблонов путей:

```go
mux := http.NewServeMux()

// 1. Метод + путь с именованным параметром {id}
mux.HandleFunc("GET /notifications/{id}", handleGetByID)
mux.HandleFunc("POST /notifications", handleCreate)
mux.HandleFunc("DELETE /notifications/{id}", handleDelete)

// 2. Wildcard для всех вложенных путей ({path...})
mux.HandleFunc("GET /static/{path...}", handleStatic)
```

### 10.4. Извлечение параметров пути: `r.PathValue` и парсинг чисел

Метод `r.PathValue(paramName)` возвращает строковое значение сегмента пути:

```go
import (
    "net/http"
    "strconv"
)

func handleGetByID(w http.ResponseWriter, r *http.Request) {
    // Получение строкового параметра из URL /notifications/{id}
    idStr := r.PathValue("id")

    // Преобразование строки в целое число
    id, err := strconv.Atoi(idStr)
    if err != nil || id <= 0 {
        http.Error(w, "Invalid ID parameter: must be a positive integer", http.StatusBadRequest)
        return
    }

    // дальнейшая работа с числовым id...
}
```

### 10.5. Сбор значений мапы в слайс для JSON-отдачи

JSON не поддерживает числовые ключи в словарях так же, как Go-мапа `map[int]Entity`. Поэтому при отдаче списка сущностей значения мапы собирают в слайс:

```go
func (s *Server) listHandler(w http.ResponseWriter, r *http.Request) {
    // Аллоцируем слайс с предвыделенной емкостью для экономии памяти
    list := make([]Notification, 0, len(s.storage))

    for _, item := range s.storage {
        list = append(list, item)
    }

    bytes, _ := json.Marshal(list)
    w.Header().Set("Content-Type", "application/json")
    w.Write(bytes)
}
```

---

## 11. Архитектура: Инкапсуляция состояния в `Server` и Pointer Receivers

### 11.1. Проблемы глобальных переменных уровня пакета

Использование `var db = make(...)` или `var counter = 0` на уровне файла/пакета приводит к фундаментальным дефектам:

1. Невозможно запустить изолированные параллельные unit-тесты (`t.Parallel()`).
2. Утечка состояния между тестами.
3. Состояние размазано по файлам проекта, нарушается инкапсуляция.

### 11.2. Создание структуры `Server`

Идиоматичное решение в Go — инкапсуляция всех зависимостей, кэшей, хранилищ и счетчиков внутри управляющей структуры:

```go
type Server struct {
    storage map[int]Notification
    nextID  int
}

// Конструктор экземпляра сервера:
func NewServer() *Server {
    return &Server{
        storage: make(map[int]Notification),
        nextID:  1,
    }
}
```

### 11.3. Value Receiver vs Pointer Receiver: потеря состояния

```go
// ОШИБКА: Value Receiver
func (s Server) AddBad(n Notification) {
    s.storage[n.ID] = n
    s.nextID++ // Изменяется копия! В исходном сервере nextID останется прежним!
}

// ПРАВИЛЬНО: Pointer Receiver
func (s *Server) AddGood(n Notification) {
    s.storage[n.ID] = n
    s.nextID++ // Мутирует настоящий экземпляр по указателю в куче
}
```

> **Правило:** Все методы HTTP-хендлеров, изменяющие внутреннее состояние структуры или имеющие дело с сервисами, **всегда должны объявляться с Pointer Receiver (`*Server`)**.

### 11.4. Регистрация методов структуры как хендлеров (Method Values)

Go поддерживает передачу методов объекта как обычных функций:

```go
func (s *Server) ListNotifications(w http.ResponseWriter, r *http.Request) {
    // реализация
}

func main() {
    srv := NewServer()
    mux := http.NewServeMux()

    // srv.ListNotifications является "Method Value" —
    // Go связывает экземпляр srv с методом, получая идеальную сигнатуру:
    // func(http.ResponseWriter, *http.Request)
    mux.HandleFunc("GET /notifications", srv.ListNotifications)
}
```

---

## 12. Middleware (Промежуточное ПО)

Middleware решает проблему дублирования сквозной функциональности: логирование, аутентификация, замеры времени выполнения (тайминги), восстановление после паник (`recover`), сжатие ответов.

### 12.1. Интерфейс `http.Handler` и тип `http.HandlerFunc`

В Go концепция обработчика держится на интерфейсе:

```go
type Handler interface {
    ServeHTTP(ResponseWriter, *Request)
}
```

Любая функция с сигнатурой `func(ResponseWriter, *Request)` может быть приведена к интерфейсу через адаптер `http.HandlerFunc`:

```go
type HandlerFunc func(ResponseWriter, *Request)

func (f HandlerFunc) ServeHTTP(w ResponseWriter, r *Request) {
    f(w, r)
}
```

### 12.2. Канонический паттерн Middleware

Middleware в Go — это функция, которая принимает следующий по цепочке `http.Handler` и возвращает новый обернутый `http.Handler`:

```go
func LoggingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()

        // Логика ДО обработки запроса
        log.Printf("Started %s %s", r.Method, r.URL.Path)

        // Передача управления следующему звену цепочки
        next.ServeHTTP(w, r)

        // Логика ПОСЛЕ обработки запроса
        duration := time.Since(start)
        log.Printf("Completed %s %s in %v", r.Method, r.URL.Path, duration)
    })
}
```

### 12.3. Подключение цепочки к `http.ServeMux`

Поскольку `http.ServeMux` сам реализует интерфейс `http.Handler`, Middleware можно обернуть вокруг всего мультиплексора:

```go
func main() {
    mux := http.NewServeMux()
    // регистрация маршрутов...

    // Оборачиваем весь роутер в middleware
    wrappedMux := LoggingMiddleware(mux)

    // Запускаем сервер с обернутым корневым хендлером
    http.ListenAndServe(":8080", wrappedMux)
}
```

---

## 13. Полный сквозной пример микросервиса

Ниже приведен эталонный, самодостаточный пример сервиса управления уведомлениями (`Notification API`), объединяющий все изученные концепции в единую чистую архитектуру Go 1.22+.

```go
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// --- Доменные модели ---

// Notification представляет запись в системе.
type Notification struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateNotificationDTO описывает контракт входящих данных.
type CreateNotificationDTO struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}

// Validate проверяет валидность входящего DTO.
func (dto CreateNotificationDTO) Validate() error {
	if dto.Title == "" {
		return errors.New("field 'title' is required")
	}
	if dto.Message == "" {
		return errors.New("field 'message' is required")
	}
	return nil
}

// --- Инкапсуляция сервера и состояния ---

// Server хранит состояние приложения и зависимости.
type Server struct {
	mu      sync.RWMutex // Защита от race conditions при конкурентных HTTP-запросах
	storage map[int]Notification
	nextID  int
}

// NewServer инициализирует новый экземпляр сервера.
func NewServer() *Server {
	return &Server{
		storage: make(map[int]Notification),
		nextID:  1,
	}
}

// --- HTTP Хендлеры (Pointer Receivers) ---

// HandleList возвращает список всех уведомлений в виде JSON-массива.
// Маршрут: GET /api/notifications
func (s *Server) HandleList(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	list := make([]Notification, 0, len(s.storage))
	for _, item := range s.storage {
		list = append(list, item)
	}
	s.mu.RUnlock()

	s.respondJSON(w, http.StatusOK, list)
}

// HandleGetByID возвращает конкретное уведомление по ID.
// Маршрут: GET /api/notifications/{id}
func (s *Server) HandleGetByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		s.respondError(w, http.StatusBadRequest, "Invalid ID parameter")
		return
	}

	s.mu.RLock()
	item, found := s.storage[id]
	s.mu.RUnlock()

	if !found {
		s.respondError(w, http.StatusNotFound, fmt.Sprintf("Notification with ID %d not found", id))
		return
	}

	s.respondJSON(w, http.StatusOK, item)
}

// HandleCreate создает новое уведомление.
// Маршрут: POST /api/notifications
func (s *Server) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var dto CreateNotificationDTO

	// 1. Десериализация JSON из потока тела запроса
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		s.respondError(w, http.StatusBadRequest, "Malformed JSON request body")
		return
	}

	// 2. Валидация входных данных
	if err := dto.Validate(); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 3. Сохранение в состояние
	s.mu.Lock()
	newNotification := Notification{
		ID:        s.nextID,
		Title:     dto.Title,
		Message:   dto.Message,
		CreatedAt: time.Now().UTC(),
	}
	s.storage[s.nextID] = newNotification
	s.nextID++
	s.mu.Unlock()

	// 4. Возврат созданного объекта
	s.respondJSON(w, http.StatusCreated, newNotification)
}

// HandleDelete удаляет уведомление по ID.
// Маршрут: DELETE /api/notifications/{id}
func (s *Server) HandleDelete(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		s.respondError(w, http.StatusBadRequest, "Invalid ID parameter")
		return
	}

	s.mu.Lock()
	_, found := s.storage[id]
	if !found {
		s.mu.Unlock()
		s.respondError(w, http.StatusNotFound, "Notification not found")
		return
	}
	delete(s.storage, id)
	s.mu.Unlock()

	w.WriteHeader(http.StatusNoContent) // 204 No Content
}

// --- Вспомогательные методы ответов (Helpers) ---

func (s *Server) respondJSON(w http.ResponseWriter, status int, data any) {
	bytes, err := json.Marshal(data)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(bytes)
}

func (s *Server) respondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	// Анонимная структура для ошибки
	_ = json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{
		Error: message,
	})
}

// --- Middleware ---

// LoggingMiddleware логирует время выполнения и параметры каждого запроса.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("[%s] %s %s - %v", r.Method, r.URL.Path, r.RemoteAddr, time.Since(start))
	})
}

// --- Точка входа ---

func main() {
	server := NewServer()

	// Создание собственного экземпляра маршрутизатора
	mux := http.NewServeMux()

	// Регистрация маршрутов с методами (Go 1.22+)
	mux.HandleFunc("GET /api/notifications", server.HandleList)
	mux.HandleFunc("GET /api/notifications/{id}", server.HandleGetByID)
	mux.HandleFunc("POST /api/notifications", server.HandleCreate)
	mux.HandleFunc("DELETE /api/notifications/{id}", server.HandleDelete)

	// Оборачивание всего роутера в middleware
	appHandler := LoggingMiddleware(mux)

	const addr = ":8080"
	log.Printf("HTTP Server is listening on http://localhost%s", addr)

	if err := http.ListenAndServe(addr, appHandler); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Fatal server error: %v", err)
	}
}
```

---

## 14. Шпаргалка по частым ошибкам (Gotchas)

1. **Неэкспортированные поля в JSON**: если поле структуры начинается со строчной буквы (например, `title string`), `json.Marshal` и `json.NewDecoder` молча проигнорируют его. Всегда используйте заглавную букву: `Title string \`json:"title"\``.
2. **Мутация в `for range`**: переменная значения в цикле — локальная копия. Изменение `item.Field = "x"` не затрагивает слайс. Используйте `slice[i].Field = "x"`.
3. **Запись в `nil`-мапу**: простое объявление `var m map[string]int` оставляет мапу равной `nil`. Попытка `m["k"] = 1` вызовет немедленную панику. Всегда делайте `make(map[string]int)` или `{}`.
4. **Порядок вызовов заголовков в `http.ResponseWriter`**: `w.Header().Set(...)` вызывается **до** `w.WriteHeader(...)`, а `w.WriteHeader(...)` — **до** `w.Write(...)`.
5. **Потеря состояния сервера**: если объявить метод хендлера как `func (s Server) MyHandler(...)` вместо указателя `func (s *Server) MyHandler(...)`, метод получит копию структуры `Server`, и любые изменения мапы/счетчиков исчезнут после завершения запроса.
6. **Возврат `append`**: результат вызова `append(slice, val)` всегда должен быть переприсвоен: `slice = append(slice, val)`.
